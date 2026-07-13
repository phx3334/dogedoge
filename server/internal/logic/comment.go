package logic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"fake_tiktok/internal/breaker"
	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/dto/response"
	"fake_tiktok/internal/repository/interfaces"

	"go.uber.org/zap"
)

// CommentLogic 多级评论业务逻辑层
//
// 统一处理视频/文章/动态三类目标的评论创建、查询、点赞、删除。
// 通过 target_type 路由到对应 repo，三套表结构一致但独立存储。
//
// 依赖注入：VideoCommentRepo / ArticleCommentRepo / DynamicCommentRepo /
// VideoCommentLikeRepo / ArticleCommentLikeRepo / DynamicCommentLikeRepo /
// NotificationRepo 将在 Phase 5（Task 12）注入到 LogicDeps。
type CommentLogic struct {
	deps *LogicDeps
}

// NewCommentLogic 创建 CommentLogic 实例
func NewCommentLogic(deps *LogicDeps) *CommentLogic {
	return &CommentLogic{deps: deps}
}


// truncateCommentPreview 截断评论预览，用于通知展示
func truncateCommentPreview(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// CreateComment 创建评论或回复
//
// 流程：
//  1. 校验 target_type（video/article/dynamic）和 content 长度
//  2. 查询目标元信息，校验 comments_closed
//  3. 若 parent_id != 0：查询父评论，新 Level = parent.Level + 1（支持无限层级）
//  4. 根据 comments_curated 决定 approved（仅 article 有精选模式）
//  5. 插入评论记录；若 approved=true 则 IncrementCommentCount(+1)
//  6. 若为回复且父评论作者 != 当前用户：创建 reply_received 通知
func (l *CommentLogic) CreateComment(ctx context.Context, userID string, req request.CreateCommentReq) (uint64, error) {
	// ---- 校验 target_type ----
	var targetType interfaces.CommentTargetType
	switch req.TargetType {
	case string(interfaces.CommentTargetVideo):
		targetType = interfaces.CommentTargetVideo
	case string(interfaces.CommentTargetArticle):
		targetType = interfaces.CommentTargetArticle
	case string(interfaces.CommentTargetDynamic):
		targetType = interfaces.CommentTargetDynamic
	default:
		return 0, fmt.Errorf("不支持的目标类型: %s", req.TargetType)
	}

	// ---- 查询目标元信息，校验评论区是否关闭 ----
	targetInfo, parentAuthorID, err := l.fetchTargetInfo(ctx, targetType, req.TargetID)
	if err != nil {
		return 0, fmt.Errorf("目标不存在")
	}
	if targetInfo.CommentsClosed {
		return 0, fmt.Errorf("评论区已关闭")
	}

	// ---- 计算评论层级 ----
	level := 1
	var parentID uint64
	if req.ParentID != 0 {
		parentLevel, pAuthorID, pErr := l.fetchParentComment(ctx, targetType, req.ParentID)
		if pErr != nil {
			return 0, fmt.Errorf("父评论不存在")
		}
		// 评论层级不再限制为最大 3 层，支持无限嵌套回复
		level = parentLevel + 1
		parentID = req.ParentID
		parentAuthorID = pAuthorID
	}

	// ---- 决定 approved（仅 article 有精选模式） ----
	approved := true
	if targetType == interfaces.CommentTargetArticle && targetInfo.CommentsCurated {
		approved = false
	}

	// ---- 写入评论 + 更新计数 ----
	newCommentID, err := l.createCommentByType(ctx, targetType, userID, req, parentID, level, approved)
	if err != nil {
		return 0, err
	}

	// 仅 approved=true 时增加评论计数（精选模式下待作者精选后才计数）
	if approved {
		if cerr := l.incrementCommentCount(ctx, targetType, req.TargetID, 1); cerr != nil {
			l.deps.Logger.Warn("IncrementCommentCount 失败",
				zap.String("target_type", req.TargetType), zap.Uint64("target_id", req.TargetID), zap.Error(cerr))
		}
	}

	// ---- 回复通知 ----
	if req.ParentID != 0 && parentAuthorID != "" && parentAuthorID != userID {
		l.createReplyNotification(ctx, targetType, userID, parentAuthorID, newCommentID, req.TargetID, req.Content)
	}

	// ---- 评论经验奖励（+5，不限制每日次数）----
	if l.deps.DailyTaskLogic != nil {
		l.deps.DailyTaskLogic.TriggerCommentReward(ctx, userID)
	}

	return newCommentID, nil
}

// fetchTargetInfo 查询目标的评论相关元信息
func (l *CommentLogic) fetchTargetInfo(ctx context.Context, targetType interfaces.CommentTargetType, targetID uint64) (*interfaces.CommentTargetInfo, string, error) {
	var info *interfaces.CommentTargetInfo
	var err error

	if semErr := l.deps.Breakers.MySQLReadSem.Acquire(ctx, 1); semErr != nil {
		l.deps.Logger.Warn("MySQL read semaphore acquire failed", zap.Error(semErr))
	} else {
		defer l.deps.Breakers.MySQLReadSem.Release(1)
	}

	breakerErr := l.deps.Breakers.MySQL.Execute(func() error {
		switch targetType {
		case interfaces.CommentTargetVideo:
			info, err = l.deps.VideoCommentRepo.FindVideoTargetInfo(ctx, uint(targetID))
		case interfaces.CommentTargetArticle:
			info, err = l.deps.ArticleCommentRepo.FindArticleTargetInfo(ctx, targetID)
		case interfaces.CommentTargetDynamic:
			info, err = l.deps.DynamicCommentRepo.FindDynamicTargetInfo(ctx, targetID)
		}
		return err
	})
	if breakerErr != nil {
		return nil, "", breakerErr
	}
	return info, info.AuthorID, nil
}

// fetchParentComment 查询父评论，返回 (level, authorID, error)
func (l *CommentLogic) fetchParentComment(ctx context.Context, targetType interfaces.CommentTargetType, parentID uint64) (int, string, error) {
	if semErr := l.deps.Breakers.MySQLReadSem.Acquire(ctx, 1); semErr != nil {
		l.deps.Logger.Warn("MySQL read semaphore acquire failed", zap.Error(semErr))
	} else {
		defer l.deps.Breakers.MySQLReadSem.Release(1)
	}

	var level int
	var authorID string

	breakerErr := l.deps.Breakers.MySQL.Execute(func() error {
		switch targetType {
		case interfaces.CommentTargetVideo:
			c, err := l.deps.VideoCommentRepo.FindByID(ctx, parentID)
			if err != nil {
				return err
			}
			level = c.Level
			authorID = c.UserID
		case interfaces.CommentTargetArticle:
			c, err := l.deps.ArticleCommentRepo.FindByID(ctx, parentID)
			if err != nil {
				return err
			}
			level = c.Level
			authorID = c.UserID
		case interfaces.CommentTargetDynamic:
			c, err := l.deps.DynamicCommentRepo.FindByID(ctx, parentID)
			if err != nil {
				return err
			}
			level = c.Level
			authorID = strconv.FormatUint(c.UserID, 10)
		}
		return nil
	})
	if breakerErr != nil {
		return 0, "", breakerErr
	}
	return level, authorID, nil
}

// createCommentByType 按目标类型构造实体并写入
func (l *CommentLogic) createCommentByType(ctx context.Context, targetType interfaces.CommentTargetType, userID string, req request.CreateCommentReq, parentID uint64, level int, approved bool) (uint64, error) {
	if err := l.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); err != nil {
		return 0, fmt.Errorf("评论服务繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLWriteSem.Release(1)

	var newID uint64
	breakerErr := l.deps.Breakers.MySQL.Execute(func() error {
		switch targetType {
		case interfaces.CommentTargetVideo:
			c := &database.Comment{
				VideoID:   req.TargetID,
				UserID:    userID,
				ParentID:  parentID,
				Level:     level,
				Content:   req.Content,
				Approved:  approved,
				CreatedAt: time.Now(),
			}
			if err := l.deps.VideoCommentRepo.Create(ctx, c); err != nil {
				return err
			}
			newID = c.ID
		case interfaces.CommentTargetArticle:
			c := &database.ArticleComment{
				ArticleID: req.TargetID,
				UserID:    userID,
				ParentID:  parentID,
				Level:     level,
				Content:   req.Content,
				Approved:  approved,
				CreatedAt: time.Now(),
			}
			if err := l.deps.ArticleCommentRepo.Create(ctx, c); err != nil {
				return err
			}
			newID = c.ID
		case interfaces.CommentTargetDynamic:
			uid, err := strconv.ParseUint(userID, 10, 64)
			if err != nil {
				return fmt.Errorf("用户 ID 格式错误")
			}
			c := &database.DynamicComment{
				DynamicID: req.TargetID,
				UserID:    uid,
				ParentID:  parentID,
				Level:     level,
				Content:   req.Content,
				CreatedAt: time.Now(),
			}
			if err := l.deps.DynamicCommentRepo.Create(ctx, c); err != nil {
				return err
			}
			newID = c.ID
		}
		return nil
	})
	if breakerErr != nil {
		return 0, breakerErr
	}
	return newID, nil
}

// incrementCommentCount 更新目标的评论计数
func (l *CommentLogic) incrementCommentCount(ctx context.Context, targetType interfaces.CommentTargetType, targetID uint64, delta int) error {
	if err := l.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); err != nil {
		return fmt.Errorf("评论服务繁忙")
	}
	defer l.deps.Breakers.MySQLWriteSem.Release(1)

	if err := l.deps.Breakers.MySQL.Execute(func() error {
		switch targetType {
		case interfaces.CommentTargetVideo:
			return l.deps.VideoCommentRepo.IncrementVideoCommentCount(ctx, uint(targetID), delta)
		case interfaces.CommentTargetArticle:
			return l.deps.ArticleCommentRepo.IncrementArticleCommentCount(ctx, targetID, delta)
		case interfaces.CommentTargetDynamic:
			return l.deps.DynamicCommentRepo.IncrementDynamicCommentCount(ctx, targetID, delta)
		}
		return nil
	}); err != nil {
		return err
	}

	// 同步更新 Redis 视频动态缓存中的 comment_count（仅视频类型）
	if targetType == interfaces.CommentTargetVideo {
		_ = l.deps.Breakers.Redis.Execute(func() error {
			return l.deps.VideoCacheRepo.IncrementCommentCount(ctx, uint(targetID), delta)
		})
	}
	return nil
}

// createReplyNotification 创建回复通知
func (l *CommentLogic) createReplyNotification(ctx context.Context, targetType interfaces.CommentTargetType, senderID, recipientID string, commentID, targetID uint64, content string) {
	senderName := ""
	senderAvatar := ""
	cardMap := l.deps.BackfillRepo.LookupAuthorCards(ctx, []string{senderID})
	if card, ok := cardMap[senderID]; ok {
		senderName = card.Username
		senderAvatar = card.AvatarURL
	}
	namesJSON, _ := json.Marshal([]string{senderName})
	payload, _ := json.Marshal(map[string]any{
		"sender_id":    senderID,
		"sender_name":  senderName,
		"sender_avatar": senderAvatar,
		"target_type":  string(targetType),
		"target_id":    strconv.FormatUint(targetID, 10),
		"comment_id":   strconv.FormatUint(commentID, 10),
		"content":      content,
	})
	now := time.Now()
	notif := &database.Notification{
		RecipientID:     recipientID,
		Type:            "reply_received",
		RelatedID:       strconv.FormatUint(commentID, 10),
		SenderNamesJSON: string(namesJSON),
		CommentPreview:  truncateCommentPreview(content, 32),
		PayloadJSON:     string(payload),
		IsRead:          false,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := l.deps.Breakers.MySQL.Execute(func() error {
		return l.deps.NotificationRepo.Create(ctx, notif)
	}); err != nil {
		l.deps.Logger.Warn("创建回复通知失败",
			zap.String("recipient", recipientID), zap.Uint64("comment_id", commentID), zap.Error(err))
	}
}

// createCommentLikeNotif 在「评论被点赞」时给评论作者发一条 comment_like 通知（best-effort）。
//
// 跳过条件：
//   - 取不到评论（已删除）或作者为空
//   - 点赞人是评论作者本人（自己赞自己不通知）
//   - 作者已对该评论「不再通知点赞」（LikeNotifMute 表）
//
// PayloadJSON 携带 target_type / target_id / comment_id，供前端点击通知跳转到对应评论。
func (l *CommentLogic) createCommentLikeNotif(ctx context.Context, likerID string, targetType interfaces.CommentTargetType, commentID uint64) {
	ownerID, content, targetID, err := l.fetchCommentMeta(ctx, targetType, commentID)
	if err != nil || ownerID == "" {
		return
	}
	if ownerID == likerID {
		return // 自己点赞自己的评论不通知
	}
	if muted, _ := l.deps.NotificationRepo.IsMutedLike(ctx, ownerID, commentID); muted {
		return
	}

	senderName := ""
	senderAvatar := ""
	cardMap := l.deps.BackfillRepo.LookupAuthorCards(ctx, []string{likerID})
	if card, ok := cardMap[likerID]; ok {
		senderName = card.Username
		senderAvatar = card.AvatarURL
	}
	namesJSON, _ := json.Marshal([]string{senderName})
	payload, _ := json.Marshal(map[string]any{
		"sender_id":    likerID,
		"sender_name":  senderName,
		"sender_avatar": senderAvatar,
		"target_type":  string(targetType),
		"target_id":    targetID,
		"comment_id":   strconv.FormatUint(commentID, 10),
		"content":      content,
	})
	now := time.Now()
	notif := &database.Notification{
		RecipientID:     ownerID,
		Type:            "comment_like",
		RelatedID:       strconv.FormatUint(commentID, 10),
		SenderNamesJSON: string(namesJSON),
		CommentPreview:  truncateCommentPreview(content, 32),
		PayloadJSON:     string(payload),
		IsRead:          false,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if cerr := l.deps.Breakers.MySQL.Execute(func() error {
		return l.deps.NotificationRepo.Create(ctx, notif)
	}); cerr != nil {
		l.deps.Logger.Warn("创建评论点赞通知失败",
			zap.Uint64("comment_id", commentID), zap.Error(cerr))
	}
}

// fetchCommentMeta 按目标类型取出评论的作者、正文与所属目标 ID（用于通知跳转）。
func (l *CommentLogic) fetchCommentMeta(ctx context.Context, targetType interfaces.CommentTargetType, commentID uint64) (ownerID, content, targetID string, err error) {
	switch targetType {
	case interfaces.CommentTargetVideo:
		c, e := l.deps.VideoCommentRepo.FindByID(ctx, commentID)
		if e != nil || c == nil {
			return "", "", "", e
		}
		return c.UserID, c.Content, strconv.FormatUint(c.VideoID, 10), nil
	case interfaces.CommentTargetArticle:
		c, e := l.deps.ArticleCommentRepo.FindByID(ctx, commentID)
		if e != nil || c == nil {
			return "", "", "", e
		}
		return c.UserID, c.Content, strconv.FormatUint(c.ArticleID, 10), nil
	case interfaces.CommentTargetDynamic:
		c, e := l.deps.DynamicCommentRepo.FindByID(ctx, commentID)
		if e != nil || c == nil {
			return "", "", "", e
		}
		return strconv.FormatUint(c.UserID, 10), c.Content, strconv.FormatUint(c.DynamicID, 10), nil
	default:
		return "", "", "", nil
	}
}

// ListComments 分页查询顶级评论 + 每条前 3 条回复
func (l *CommentLogic) ListComments(ctx context.Context, req request.ListCommentsReq) ([]response.CommentItem, int64, error) {
	// 分页默认值
	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 50 {
		pageSize = 50
	}

	var targetType interfaces.CommentTargetType
	switch req.TargetType {
	case string(interfaces.CommentTargetVideo):
		targetType = interfaces.CommentTargetVideo
	case string(interfaces.CommentTargetArticle):
		targetType = interfaces.CommentTargetArticle
	case string(interfaces.CommentTargetDynamic):
		targetType = interfaces.CommentTargetDynamic
	default:
		return nil, 0, fmt.Errorf("不支持的目标类型: %s", req.TargetType)
	}

	if semErr := l.deps.Breakers.MySQLReadSem.Acquire(ctx, 1); semErr != nil {
		l.deps.Logger.Warn("MySQL read semaphore acquire failed", zap.Error(semErr))
	} else {
		defer l.deps.Breakers.MySQLReadSem.Release(1)
	}

	// ---- 拉取顶级评论 ----
	var total int64
	var err error
	list := make([]response.CommentItem, 0)

	breakerErr := l.deps.Breakers.MySQL.Execute(func() error {
		switch targetType {
		case interfaces.CommentTargetVideo:
			comments, t, e := l.deps.VideoCommentRepo.FindByVideoID(ctx, uint(req.TargetID), page, pageSize)
			total = t
			err = e
			list = l.assembleVideoCommentItems(ctx, comments)
		case interfaces.CommentTargetArticle:
			comments, t, e := l.deps.ArticleCommentRepo.FindByArticleID(ctx, req.TargetID, page, pageSize)
			total = t
			err = e
			list = l.assembleArticleCommentItems(ctx, comments)
		case interfaces.CommentTargetDynamic:
			comments, t, e := l.deps.DynamicCommentRepo.FindByDynamicID(ctx, req.TargetID, page, pageSize)
			total = t
			err = e
			list = l.assembleDynamicCommentItems(ctx, comments)
		}
		return err
	})
	if breakerErr != nil {
		if errors.Is(breakerErr, breaker.ErrCircuitOpen) {
			l.deps.Logger.Warn("MySQL circuit open during ListComments")
		}
		return nil, 0, breakerErr
	}
	return list, total, nil
}

// assembleVideoCommentItems 组装视频评论列表（含每条前 3 条回复 + reply_count）
func (l *CommentLogic) assembleVideoCommentItems(ctx context.Context, comments []database.Comment) []response.CommentItem {
	// 收集所有用户 ID 用于批量查询
	authorIDs := make([]string, 0, len(comments))
	seen := make(map[string]bool)
	for _, c := range comments {
		if c.UserID != "" && !seen[c.UserID] {
			authorIDs = append(authorIDs, c.UserID)
			seen[c.UserID] = true
		}
	}
	// 也收集回复的作者 ID
	for _, c := range comments {
		replies, _, _ := l.deps.VideoCommentRepo.FindReplies(ctx, c.ID, 1, 3)
		for _, r := range replies {
			if r.UserID != "" && !seen[r.UserID] {
				authorIDs = append(authorIDs, r.UserID)
				seen[r.UserID] = true
			}
		}
	}
	// 一次性批量取用户名+头像，避免 N+1 查询
	cardMap := l.deps.BackfillRepo.LookupAuthorCards(ctx, authorIDs)

	items := make([]response.CommentItem, 0, len(comments))
	for _, c := range comments {
		card := cardMap[c.UserID]
		item := response.CommentItem{
			ID:         c.ID,
			User:       response.UserCard{ID: c.UserID, Username: card.Username, AvatarURL: card.AvatarURL, Level: interfaces.CalcLevel(card.Experience)},
			Content:    c.Content,
			LikeCount:  c.LikeCount,
			Pinned:     c.Pinned,
			CreatedAt:  c.CreatedAt,
			IpLocation: c.IpLocation,
		}

		// 获取回复数和前 3 条回复
		replyCount, _ := l.deps.VideoCommentRepo.CountReplies(ctx, c.ID)
		item.ReplyCount = replyCount

		replies, _, _ := l.deps.VideoCommentRepo.FindReplies(ctx, c.ID, 1, 3)
		item.Replies = make([]response.CommentItem, 0, len(replies))
		for _, r := range replies {
			rcard := cardMap[r.UserID]
			item.Replies = append(item.Replies, response.CommentItem{
				ID:         r.ID,
				User:       response.UserCard{ID: r.UserID, Username: rcard.Username, AvatarURL: rcard.AvatarURL, Level: interfaces.CalcLevel(rcard.Experience)},
				Content:    r.Content,
				LikeCount:  r.LikeCount,
				Pinned:     r.Pinned,
				CreatedAt:  r.CreatedAt,
				IpLocation: r.IpLocation,
			})
		}
		items = append(items, item)
	}
	return items
}

// assembleArticleCommentItems 组装文章评论列表
func (l *CommentLogic) assembleArticleCommentItems(ctx context.Context, comments []database.ArticleComment) []response.CommentItem {
	authorIDs := make([]string, 0, len(comments))
	seen := make(map[string]bool)
	for _, c := range comments {
		if c.UserID != "" && !seen[c.UserID] {
			authorIDs = append(authorIDs, c.UserID)
			seen[c.UserID] = true
		}
	}
	for _, c := range comments {
		replies, _, _ := l.deps.ArticleCommentRepo.FindReplies(ctx, c.ID, 1, 3)
		for _, r := range replies {
			if r.UserID != "" && !seen[r.UserID] {
				authorIDs = append(authorIDs, r.UserID)
				seen[r.UserID] = true
			}
		}
	}
	// 一次性批量取用户名+头像，避免 N+1 查询
	cardMap := l.deps.BackfillRepo.LookupAuthorCards(ctx, authorIDs)

	items := make([]response.CommentItem, 0, len(comments))
	for _, c := range comments {
		card := cardMap[c.UserID]
		item := response.CommentItem{
			ID:         c.ID,
			User:       response.UserCard{ID: c.UserID, Username: card.Username, AvatarURL: card.AvatarURL, Level: interfaces.CalcLevel(card.Experience)},
			Content:    c.Content,
			LikeCount:  c.LikeCount,
			Pinned:     c.Pinned,
			CreatedAt:  c.CreatedAt,
			IpLocation: c.IpLocation,
		}

		replyCount, _ := l.deps.ArticleCommentRepo.CountReplies(ctx, c.ID)
		item.ReplyCount = replyCount

		replies, _, _ := l.deps.ArticleCommentRepo.FindReplies(ctx, c.ID, 1, 3)
		item.Replies = make([]response.CommentItem, 0, len(replies))
		for _, r := range replies {
			rcard := cardMap[r.UserID]
			item.Replies = append(item.Replies, response.CommentItem{
				ID:         r.ID,
				User:       response.UserCard{ID: r.UserID, Username: rcard.Username, AvatarURL: rcard.AvatarURL, Level: interfaces.CalcLevel(rcard.Experience)},
				Content:    r.Content,
				LikeCount:  r.LikeCount,
				Pinned:     r.Pinned,
				CreatedAt:  r.CreatedAt,
				IpLocation: r.IpLocation,
			})
		}
		items = append(items, item)
	}
	return items
}

// assembleDynamicCommentItems 组装动态评论列表
func (l *CommentLogic) assembleDynamicCommentItems(ctx context.Context, comments []database.DynamicComment) []response.CommentItem {
	authorIDs := make([]string, 0, len(comments))
	seen := make(map[string]bool)
	for _, c := range comments {
		uidStr := strconv.FormatUint(c.UserID, 10)
		if !seen[uidStr] {
			authorIDs = append(authorIDs, uidStr)
			seen[uidStr] = true
		}
	}
	for _, c := range comments {
		replies, _, _ := l.deps.DynamicCommentRepo.FindReplies(ctx, c.ID, 1, 3)
		for _, r := range replies {
			uidStr := strconv.FormatUint(r.UserID, 10)
			if !seen[uidStr] {
				authorIDs = append(authorIDs, uidStr)
				seen[uidStr] = true
			}
		}
	}
	// 一次性批量取用户名+头像，避免 N+1 查询
	cardMap := l.deps.BackfillRepo.LookupAuthorCards(ctx, authorIDs)

	items := make([]response.CommentItem, 0, len(comments))
	for _, c := range comments {
		uidStr := strconv.FormatUint(c.UserID, 10)
		card := cardMap[uidStr]
		item := response.CommentItem{
			ID:         c.ID,
			User:       response.UserCard{ID: uidStr, Username: card.Username, AvatarURL: card.AvatarURL, Level: interfaces.CalcLevel(card.Experience)},
			Content:    c.Content,
			LikeCount:  c.LikeCount,
			Pinned:     c.Pinned,
			CreatedAt:  c.CreatedAt,
			IpLocation: c.IpLocation,
		}

		replyCount, _ := l.deps.DynamicCommentRepo.CountReplies(ctx, c.ID)
		item.ReplyCount = replyCount

		replies, _, _ := l.deps.DynamicCommentRepo.FindReplies(ctx, c.ID, 1, 3)
		item.Replies = make([]response.CommentItem, 0, len(replies))
		for _, r := range replies {
			ruidStr := strconv.FormatUint(r.UserID, 10)
			rcard := cardMap[ruidStr]
			item.Replies = append(item.Replies, response.CommentItem{
				ID:         r.ID,
				User:       response.UserCard{ID: ruidStr, Username: rcard.Username, AvatarURL: rcard.AvatarURL, Level: interfaces.CalcLevel(rcard.Experience)},
				Content:    r.Content,
				LikeCount:  r.LikeCount,
				Pinned:     r.Pinned,
				CreatedAt:  r.CreatedAt,
				IpLocation: r.IpLocation,
			})
		}
		items = append(items, item)
	}
	return items
}

// ListReplies 分页查询指定评论的回复
func (l *CommentLogic) ListReplies(ctx context.Context, req request.ListRepliesReq) ([]response.CommentItem, int64, error) {
	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 50 {
		pageSize = 50
	}

	var targetType interfaces.CommentTargetType
	switch req.TargetType {
	case string(interfaces.CommentTargetVideo):
		targetType = interfaces.CommentTargetVideo
	case string(interfaces.CommentTargetArticle):
		targetType = interfaces.CommentTargetArticle
	case string(interfaces.CommentTargetDynamic):
		targetType = interfaces.CommentTargetDynamic
	default:
		return nil, 0, fmt.Errorf("不支持的目标类型: %s", req.TargetType)
	}

	if semErr := l.deps.Breakers.MySQLReadSem.Acquire(ctx, 1); semErr != nil {
		l.deps.Logger.Warn("MySQL read semaphore acquire failed", zap.Error(semErr))
	} else {
		defer l.deps.Breakers.MySQLReadSem.Release(1)
	}

	var total int64
	var err error
	list := make([]response.CommentItem, 0)

	// 递归查询所有层级的回复（支持无限层级嵌套）
	// 策略：先分页查直接子评论，再对每一层子评论 BFS 查询其所有后代
	breakerErr := l.deps.Breakers.MySQL.Execute(func() error {
		switch targetType {
		case interfaces.CommentTargetVideo:
			replies, t, e := l.deps.VideoCommentRepo.FindReplies(ctx, req.CommentID, page, pageSize)
			if e != nil {
				total = t
				err = e
				return err
			}
			total = t
			replies = l.fetchAllCommentDescendants(ctx, l.deps.VideoCommentRepo, replies)
			list = l.assembleVideoReplyItems(ctx, replies)
		case interfaces.CommentTargetArticle:
			replies, t, e := l.deps.ArticleCommentRepo.FindReplies(ctx, req.CommentID, page, pageSize)
			if e != nil {
				total = t
				err = e
				return err
			}
			total = t
			replies = l.fetchAllArticleCommentDescendants(ctx, l.deps.ArticleCommentRepo, replies)
			list = l.assembleArticleReplyItems(ctx, replies)
		case interfaces.CommentTargetDynamic:
			replies, t, e := l.deps.DynamicCommentRepo.FindReplies(ctx, req.CommentID, page, pageSize)
			if e != nil {
				total = t
				err = e
				return err
			}
			total = t
			replies = l.fetchAllDynamicCommentDescendants(ctx, l.deps.DynamicCommentRepo, replies)
			list = l.assembleDynamicReplyItems(ctx, replies)
		}
		return err
	})
	if breakerErr != nil {
		if errors.Is(breakerErr, breaker.ErrCircuitOpen) {
			l.deps.Logger.Warn("MySQL circuit open during ListReplies")
		}
		return nil, 0, breakerErr
	}
	return list, total, nil
}

// fetchAllCommentDescendants BFS 递归查询视频评论的所有后代回复
func (l *CommentLogic) fetchAllCommentDescendants(ctx context.Context, repo interfaces.VideoCommentRepository, replies []database.Comment) []database.Comment {
	if len(replies) == 0 {
		return replies
	}
	parentIDs := make([]uint64, len(replies))
	for i, r := range replies {
		parentIDs[i] = r.ID
	}
	for len(parentIDs) > 0 {
		children, _, err := repo.FindRepliesByParentIDs(ctx, parentIDs, 200)
		if err != nil || len(children) == 0 {
			break
		}
		replies = append(replies, children...)
		nextIDs := make([]uint64, 0, len(children))
		for _, c := range children {
			nextIDs = append(nextIDs, c.ID)
		}
		parentIDs = nextIDs
	}
	return replies
}

// fetchAllArticleCommentDescendants BFS 递归查询文章评论的所有后代回复
func (l *CommentLogic) fetchAllArticleCommentDescendants(ctx context.Context, repo interfaces.ArticleCommentRepository, replies []database.ArticleComment) []database.ArticleComment {
	if len(replies) == 0 {
		return replies
	}
	parentIDs := make([]uint64, len(replies))
	for i, r := range replies {
		parentIDs[i] = r.ID
	}
	for len(parentIDs) > 0 {
		children, _, err := repo.FindRepliesByParentIDs(ctx, parentIDs, 200)
		if err != nil || len(children) == 0 {
			break
		}
		replies = append(replies, children...)
		nextIDs := make([]uint64, 0, len(children))
		for _, c := range children {
			nextIDs = append(nextIDs, c.ID)
		}
		parentIDs = nextIDs
	}
	return replies
}

// fetchAllDynamicCommentDescendants BFS 递归查询动态评论的所有后代回复
func (l *CommentLogic) fetchAllDynamicCommentDescendants(ctx context.Context, repo interfaces.DynamicCommentRepository, replies []database.DynamicComment) []database.DynamicComment {
	if len(replies) == 0 {
		return replies
	}
	parentIDs := make([]uint64, len(replies))
	for i, r := range replies {
		parentIDs[i] = r.ID
	}
	for len(parentIDs) > 0 {
		children, _, err := repo.FindRepliesByParentIDs(ctx, parentIDs, 200)
		if err != nil || len(children) == 0 {
			break
		}
		replies = append(replies, children...)
		nextIDs := make([]uint64, 0, len(children))
		for _, c := range children {
			nextIDs = append(nextIDs, c.ID)
		}
		parentIDs = nextIDs
	}
	return replies
}

func (l *CommentLogic) assembleVideoReplyItems(ctx context.Context, replies []database.Comment) []response.CommentItem {
	authorIDs := make([]string, 0, len(replies))
	seen := make(map[string]bool)
	for _, r := range replies {
		if r.UserID != "" && !seen[r.UserID] {
			authorIDs = append(authorIDs, r.UserID)
			seen[r.UserID] = true
		}
	}
	// 一次性批量取用户名+头像，避免 N+1 查询
	cardMap := l.deps.BackfillRepo.LookupAuthorCards(ctx, authorIDs)

	items := make([]response.CommentItem, 0, len(replies))
	for _, r := range replies {
		card := cardMap[r.UserID]
		items = append(items, response.CommentItem{
			ID:         r.ID,
			User:       response.UserCard{ID: r.UserID, Username: card.Username, AvatarURL: card.AvatarURL, Level: interfaces.CalcLevel(card.Experience)},
			Content:    r.Content,
			LikeCount:  r.LikeCount,
			Pinned:     r.Pinned,
			CreatedAt:  r.CreatedAt,
			IpLocation: r.IpLocation,
		})
	}
	return items
}

func (l *CommentLogic) assembleArticleReplyItems(ctx context.Context, replies []database.ArticleComment) []response.CommentItem {
	authorIDs := make([]string, 0, len(replies))
	seen := make(map[string]bool)
	for _, r := range replies {
		if r.UserID != "" && !seen[r.UserID] {
			authorIDs = append(authorIDs, r.UserID)
			seen[r.UserID] = true
		}
	}
	// 一次性批量取用户名+头像，避免 N+1 查询
	cardMap := l.deps.BackfillRepo.LookupAuthorCards(ctx, authorIDs)

	items := make([]response.CommentItem, 0, len(replies))
	for _, r := range replies {
		card := cardMap[r.UserID]
		items = append(items, response.CommentItem{
			ID:         r.ID,
			User:       response.UserCard{ID: r.UserID, Username: card.Username, AvatarURL: card.AvatarURL, Level: interfaces.CalcLevel(card.Experience)},
			Content:    r.Content,
			LikeCount:  r.LikeCount,
			Pinned:     r.Pinned,
			CreatedAt:  r.CreatedAt,
			IpLocation: r.IpLocation,
		})
	}
	return items
}

func (l *CommentLogic) assembleDynamicReplyItems(ctx context.Context, replies []database.DynamicComment) []response.CommentItem {
	authorIDs := make([]string, 0, len(replies))
	seen := make(map[string]bool)
	for _, r := range replies {
		uidStr := strconv.FormatUint(r.UserID, 10)
		if !seen[uidStr] {
			authorIDs = append(authorIDs, uidStr)
			seen[uidStr] = true
		}
	}
	// 一次性批量取用户名+头像，避免 N+1 查询
	cardMap := l.deps.BackfillRepo.LookupAuthorCards(ctx, authorIDs)

	items := make([]response.CommentItem, 0, len(replies))
	for _, r := range replies {
		uidStr := strconv.FormatUint(r.UserID, 10)
		card := cardMap[uidStr]
		items = append(items, response.CommentItem{
			ID:         r.ID,
			User:       response.UserCard{ID: uidStr, Username: card.Username, AvatarURL: card.AvatarURL, Level: interfaces.CalcLevel(card.Experience)},
			Content:    r.Content,
			LikeCount:  r.LikeCount,
			Pinned:     r.Pinned,
			CreatedAt:  r.CreatedAt,
			IpLocation: r.IpLocation,
		})
	}
	return items
}

// LikeComment 点赞评论（幂等：重复点赞不报错、不重复计数）
func (l *CommentLogic) LikeComment(ctx context.Context, userID string, req request.CommentLikeReq) error {
	likeRepo, commentRepo, err := l.resolveLikeRepos(req.TargetType)
	if err != nil {
		return err
	}

	if err := l.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); err != nil {
		return fmt.Errorf("点赞服务繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLWriteSem.Release(1)

	var created bool
	breakerErr := l.deps.Breakers.MySQL.Execute(func() error {
		var e error
		created, e = likeRepo.CreateLike(ctx, userID, req.CommentID)
		return e
	})
	if breakerErr != nil {
		if errors.Is(breakerErr, breaker.ErrCircuitOpen) {
			l.deps.Logger.Warn("MySQL circuit open during LikeComment")
		}
		return fmt.Errorf("点赞失败，请稍后重试")
	}

	// 仅首次点赞时更新计数
	if created {
		if cerr := l.deps.Breakers.MySQL.Execute(func() error {
			return commentRepo.IncrementLikeCount(ctx, req.CommentID, 1)
		}); cerr != nil {
			l.deps.Logger.Warn("IncrementLikeCount 失败",
				zap.Uint64("comment_id", req.CommentID), zap.Error(cerr))
		}
		// 给评论作者发「评论被点赞」通知（best-effort，不影响点赞主流程）
		l.createCommentLikeNotif(ctx, userID, interfaces.CommentTargetType(req.TargetType), req.CommentID)
	}
	return nil
}

// UnlikeComment 取消点赞评论（幂等：未点赞时不报错）
func (l *CommentLogic) UnlikeComment(ctx context.Context, userID string, req request.CommentLikeReq) error {
	likeRepo, commentRepo, err := l.resolveLikeRepos(req.TargetType)
	if err != nil {
		return err
	}

	if err := l.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); err != nil {
		return fmt.Errorf("操作繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLWriteSem.Release(1)

	var exists bool
	breakerErr := l.deps.Breakers.MySQL.Execute(func() error {
		var e error
		exists, e = likeRepo.ExistsLike(ctx, userID, req.CommentID)
		return e
	})
	if breakerErr != nil {
		return fmt.Errorf("操作失败，请稍后重试")
	}

	// 未点赞，幂等返回
	if !exists {
		return nil
	}

	breakerErr = l.deps.Breakers.MySQL.Execute(func() error {
		return likeRepo.DeleteLike(ctx, userID, req.CommentID)
	})
	if breakerErr != nil {
		return fmt.Errorf("取消点赞失败，请稍后重试")
	}

	if cerr := l.deps.Breakers.MySQL.Execute(func() error {
		return commentRepo.IncrementLikeCount(ctx, req.CommentID, -1)
	}); cerr != nil {
		l.deps.Logger.Warn("IncrementLikeCount -1 失败",
			zap.Uint64("comment_id", req.CommentID), zap.Error(cerr))
	}
	return nil
}

// DeleteComment 删除评论（仅评论作者或目标 UP 主可删除）
func (l *CommentLogic) DeleteComment(ctx context.Context, userID string, req request.DeleteCommentReq) error {
	var targetType interfaces.CommentTargetType
	switch req.TargetType {
	case string(interfaces.CommentTargetVideo):
		targetType = interfaces.CommentTargetVideo
	case string(interfaces.CommentTargetArticle):
		targetType = interfaces.CommentTargetArticle
	case string(interfaces.CommentTargetDynamic):
		targetType = interfaces.CommentTargetDynamic
	default:
		return fmt.Errorf("不支持的目标类型: %s", req.TargetType)
	}

	if semErr := l.deps.Breakers.MySQLReadSem.Acquire(ctx, 1); semErr != nil {
		l.deps.Logger.Warn("MySQL read semaphore acquire failed", zap.Error(semErr))
	} else {
		defer l.deps.Breakers.MySQLReadSem.Release(1)
	}

	// ---- 查询评论，获取作者 ID 和目标 ID ----
	var commentAuthorID string
	var targetID uint64
	var approved bool

	breakerErr := l.deps.Breakers.MySQL.Execute(func() error {
		switch targetType {
		case interfaces.CommentTargetVideo:
			c, err := l.deps.VideoCommentRepo.FindByID(ctx, req.CommentID)
			if err != nil {
				return err
			}
			commentAuthorID = c.UserID
			targetID = c.VideoID
			approved = c.Approved
		case interfaces.CommentTargetArticle:
			c, err := l.deps.ArticleCommentRepo.FindByID(ctx, req.CommentID)
			if err != nil {
				return err
			}
			commentAuthorID = c.UserID
			targetID = c.ArticleID
			approved = c.Approved
		case interfaces.CommentTargetDynamic:
			c, err := l.deps.DynamicCommentRepo.FindByID(ctx, req.CommentID)
			if err != nil {
				return err
			}
			commentAuthorID = strconv.FormatUint(c.UserID, 10)
			targetID = c.DynamicID
			approved = true // 动态评论无精选模式，视为已通过
		}
		return nil
	})
	if breakerErr != nil {
		return fmt.Errorf("评论不存在")
	}

	// ---- 查询目标 UP 主，校验权限 ----
	targetInfo, _, err := l.fetchTargetInfo(ctx, targetType, targetID)
	if err != nil {
		return fmt.Errorf("目标不存在")
	}

	// 仅评论作者或目标 UP 主可删除
	if commentAuthorID != userID && targetInfo.AuthorID != userID {
		return fmt.Errorf("无权限删除")
	}

	// ---- 删除评论 + 减少计数 ----
	if err := l.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); err != nil {
		return fmt.Errorf("操作繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLWriteSem.Release(1)

	breakerErr = l.deps.Breakers.MySQL.Execute(func() error {
		switch targetType {
		case interfaces.CommentTargetVideo:
			return l.deps.VideoCommentRepo.Delete(ctx, req.CommentID)
		case interfaces.CommentTargetArticle:
			return l.deps.ArticleCommentRepo.Delete(ctx, req.CommentID)
		case interfaces.CommentTargetDynamic:
			return l.deps.DynamicCommentRepo.Delete(ctx, req.CommentID)
		}
		return nil
	})
	if breakerErr != nil {
		return fmt.Errorf("删除失败，请稍后重试")
	}

	// 仅 approved=true 时减少计数
	if approved {
		if cerr := l.incrementCommentCount(ctx, targetType, targetID, -1); cerr != nil {
			l.deps.Logger.Warn("DecrementCommentCount 失败",
				zap.String("target_type", req.TargetType), zap.Uint64("target_id", targetID), zap.Error(cerr))
		}
	}
	return nil
}

// resolveLikeRepos 根据 target_type 返回对应的 likeRepo 和 commentRepo
func (l *CommentLogic) resolveLikeRepos(targetType string) (interfaces.CommentLikeRepository, commentLikeTargetRepo, error) {
	switch targetType {
	case string(interfaces.CommentTargetVideo):
		return l.deps.VideoCommentLikeRepo, l.deps.VideoCommentRepo, nil
	case string(interfaces.CommentTargetArticle):
		return l.deps.ArticleCommentLikeRepo, l.deps.ArticleCommentRepo, nil
	case string(interfaces.CommentTargetDynamic):
		return l.deps.DynamicCommentLikeRepo, l.deps.DynamicCommentRepo, nil
	default:
		return nil, nil, fmt.Errorf("不支持的目标类型: %s", targetType)
	}
}

// commentLikeTargetRepo 评论 repo 中点赞计数相关方法的子集
//
// 为了让 LikeComment / UnlikeComment 统一调用 IncrementLikeCount，
// 定义此接口收敛三套 comment repo 的共同方法。
type commentLikeTargetRepo interface {
	IncrementLikeCount(ctx context.Context, id uint64, delta int) error
}
