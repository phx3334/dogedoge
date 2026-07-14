package logic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"fake_tiktok/internal/breaker"
	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/dto/response"

	"go.uber.org/zap"
)

// DynamicLogic 用户动态（图文）业务逻辑
type DynamicLogic struct {
	deps *LogicDeps
}

func NewDynamicLogic(deps *LogicDeps) *DynamicLogic {
	return &DynamicLogic{deps: deps}
}

// CreateDynamic 发布动态
// 图片最多 9 张（B 站规范），超过截断
func (l *DynamicLogic) CreateDynamic(ctx context.Context, userID string, req request.CreateDynamicReq) (uint64, error) {
	// 校验图片数量
	if len(req.Images) > 9 {
		req.Images = req.Images[:9]
	}

	imagesJSON, err := json.Marshal(req.Images)
	if err != nil {
		return 0, fmt.Errorf("序列化图片列表失败")
	}

	userIDUint, _ := strconv.ParseUint(userID, 10, 64)
	if userIDUint == 0 {
		return 0, errors.New("invalid user id")
	}

	dynamic := &database.UserDynamicText{
		UserID:       userIDUint,
		Title:        req.Title,
		Content:      req.Content,
		ImagesJSON:   string(imagesJSON),
		CommentCount: 0,
		LikeCount:    0,
	}

	if err := l.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); err != nil {
		return 0, fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLWriteSem.Release(1)

	if err := l.deps.Breakers.MySQL.Execute(func() error {
		return l.deps.UserDynamicRepo.Create(ctx, dynamic)
	}); err != nil {
		if errors.Is(err, breaker.ErrCircuitOpen) {
			l.deps.Logger.Warn("MySQL circuit open during CreateDynamic",
				zap.String("user_id", userID))
		}
		return 0, fmt.Errorf("发布动态失败")
	}
	return dynamic.ID, nil
}

// ListUserDynamics 分页查询指定用户的动态
func (l *DynamicLogic) ListUserDynamics(ctx context.Context, userID string, currentUserID string, req request.ListUserDynamicsReq) (*response.PaginatedResp[response.DynamicItem], error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 || req.PageSize > 100 {
		req.PageSize = 20
	}

	// 转换 userID（被查询的用户）为 uint64
	userIDUint, _ := strconv.ParseUint(req.UserID, 10, 64)

	if err := l.deps.Breakers.MySQLReadSem.Acquire(ctx, 1); err != nil {
		return nil, fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLReadSem.Release(1)

	dynamics, total, err := l.deps.UserDynamicRepo.ListByUser(ctx, userIDUint, req.Page, req.PageSize)
	if err != nil {
		return nil, fmt.Errorf("查询动态失败")
	}

	// 批量查询作者信息
	cardMap := l.deps.BackfillRepo.LookupAuthorCards(ctx, []string{req.UserID})

	items := make([]response.DynamicItem, 0, len(dynamics))
	for _, d := range dynamics {
		card := cardMap[req.UserID]
		item := response.DynamicItem{
			ID:           d.ID,
			UserID:       strconv.FormatUint(d.UserID, 10),
			Username:     card.Username,
			AvatarURL:    card.AvatarURL,
			Type:         "dynamic",
			Title:        d.Title,
			Content:      d.Content,
			ImagesJSON:   d.ImagesJSON,
			LikeCount:    d.LikeCount,
			CommentCount: d.CommentCount,
			CreatedAt:    d.CreatedAt,
		}
		// 查询当前用户是否点赞
		if currentUserID != "" {
			curUserID, _ := strconv.ParseUint(currentUserID, 10, 64)
			if curUserID > 0 {
				liked, _ := l.deps.UserDynamicLikeRepo.IsLiked(ctx, curUserID, d.ID)
				item.IsLiked = liked
			}
		}
		items = append(items, item)
	}

	return &response.PaginatedResp[response.DynamicItem]{
		List:     items,
		Total:    total,
		Page:     req.Page,
		PageSize: req.PageSize,
	}, nil
}

// ListDynamicFeed 查询关注用户+自己的最新动态流（图文动态+视频投稿+文章投稿）
func (l *DynamicLogic) ListDynamicFeed(ctx context.Context, currentUserID string, req request.ListDynamicFeedReq) (*response.PaginatedResp[response.DynamicItem], error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 || req.PageSize > 100 {
		req.PageSize = 20
	}

	if err := l.deps.Breakers.MySQLReadSem.Acquire(ctx, 1); err != nil {
		return nil, fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLReadSem.Release(1)

	// 查询当前用户关注的所有用户
	followeeIDs, _, err := l.deps.InteractionRepo.ListFollowing(ctx, currentUserID, 1, 1000)
	if err != nil {
		return nil, fmt.Errorf("查询关注列表失败")
	}

	// 将自己加入 feed 列表：动态 = 自己发布的 + 关注的人发布的
	feedUserIDs := make([]string, 0, len(followeeIDs)+1)
	feedUserIDs = append(feedUserIDs, currentUserID)
	feedUserIDs = append(feedUserIDs, followeeIDs...)

	return l.mixedFeedQuery(ctx, currentUserID, feedUserIDs, req.Page, req.PageSize)
}

// ListUserMixedDynamics 查询指定用户主页的混合动态流（视频投稿+文章投稿+图文动态）。
// 与 ListDynamicFeed 共用 mixedFeedQuery：让个人主页"动态"Tab 也能看到该用户的
// 视频与文章，而不只是图文动态。
func (l *DynamicLogic) ListUserMixedDynamics(ctx context.Context, currentUserID string, req request.ListUserDynamicsReq) (*response.PaginatedResp[response.DynamicItem], error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 || req.PageSize > 100 {
		req.PageSize = 20
	}

	if err := l.deps.Breakers.MySQLReadSem.Acquire(ctx, 1); err != nil {
		return nil, fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLReadSem.Release(1)

	// 仅查该用户自己发布的内容（不含其关注流）
	userIDUint, _ := strconv.ParseUint(req.UserID, 10, 64)
	if userIDUint == 0 {
		return nil, fmt.Errorf("参数错误：user_id 非法")
	}
	feedUserIDs := []string{req.UserID}

	return l.mixedFeedQuery(ctx, currentUserID, feedUserIDs, req.Page, req.PageSize)
}

// mixedFeedQuery 并发拉取多类内容（图文动态/视频/文章），合并后按时间倒序分页。
// userIDs 为内容作者范围；currentUserID 用于判断当前用户对图文动态的已赞状态。
func (l *DynamicLogic) mixedFeedQuery(ctx context.Context, currentUserID string, userIDs []string, page, pageSize int) (*response.PaginatedResp[response.DynamicItem], error) {
	curUserID, _ := strconv.ParseUint(currentUserID, 10, 64)

	// 每种类型多查一些，合并后取 pageSize 条
	perPageFetch := pageSize * 2
	if perPageFetch > 50 {
		perPageFetch = 50
	}
	offset := (page - 1) * pageSize

	// 并发查询三类内容
	type queryResult struct {
		items []response.DynamicItem
		err   error
	}
	resultCh := make(chan queryResult, 3)

	// 1. 图文动态
	go func() {
		dynamics, _, err := l.deps.UserDynamicRepo.ListFeed(ctx, userIDs, 1, perPageFetch)
		if err != nil {
			resultCh <- queryResult{err: err}
			return
		}
		items := make([]response.DynamicItem, 0, len(dynamics))
		for _, d := range dynamics {
			item := response.DynamicItem{
				ID:           d.ID,
				UserID:       strconv.FormatUint(d.UserID, 10),
				Type:         "dynamic",
				Title:        d.Title,
				Content:      d.Content,
				ImagesJSON:   d.ImagesJSON,
				LikeCount:    d.LikeCount,
				CommentCount: d.CommentCount,
				CreatedAt:    d.CreatedAt,
			}
			if curUserID > 0 {
				liked, _ := l.deps.UserDynamicLikeRepo.IsLiked(ctx, curUserID, d.ID)
				item.IsLiked = liked
			}
			items = append(items, item)
		}
		resultCh <- queryResult{items: items}
	}()

	// 2. 视频投稿
	go func() {
		videos, err := l.deps.VideoRepo.FindPublishedVideosByAuthorIDs(ctx, userIDs, perPageFetch, 0)
		if err != nil {
			resultCh <- queryResult{err: err}
			return
		}
		items := make([]response.DynamicItem, 0, len(videos))
		for _, v := range videos {
			items = append(items, response.DynamicItem{
				ID:           uint64(v.ID),
				UserID:       v.AuthorID,
				Type:         "video",
				Title:        v.Title,
				VideoID:      v.ID,
				CoverURL:     v.CoverURL,
				Duration:     v.DurationSec,
				PlayCount:    v.PlayCount,
				CommentCount: uint64(v.CommentsCount),
				CreatedAt:    v.CreatedAt,
			})
		}
		resultCh <- queryResult{items: items}
	}()

	// 3. 文章投稿
	go func() {
		articles, err := l.deps.ArticleRepo.FindPublishedArticlesByAuthorIDs(ctx, userIDs, perPageFetch, 0)
		if err != nil {
			resultCh <- queryResult{err: err}
			return
		}
		items := make([]response.DynamicItem, 0, len(articles))
		for _, a := range articles {
			items = append(items, response.DynamicItem{
				ID:           a.ID,
				UserID:       a.UserID,
				Type:         "article",
				Title:        a.Title,
				Content:      a.BodyMD,
				ArticleID:    a.ID,
				CoverURL:     a.CoverURL,
				ViewCount:    a.ViewCount,
				CommentCount: a.CommentCount,
				CreatedAt:    a.CreatedAt,
			})
		}
		resultCh <- queryResult{items: items}
	}()

	// 收集结果
	var allItems []response.DynamicItem
	for i := 0; i < 3; i++ {
		r := <-resultCh
		if r.err != nil {
			l.deps.Logger.Warn("dynamic feed query failed", zap.Error(r.err))
			continue
		}
		allItems = append(allItems, r.items...)
	}

	// 按时间倒序排序
	sort.Slice(allItems, func(i, j int) bool {
		return allItems[i].CreatedAt.After(allItems[j].CreatedAt)
	})

	// 批量填充 username + avatar_url
	authorIDSet := make(map[string]bool)
	for _, item := range allItems {
		if item.UserID != "" && !authorIDSet[item.UserID] {
			authorIDSet[item.UserID] = true
		}
	}
	authorIDs := make([]string, 0, len(authorIDSet))
	for id := range authorIDSet {
		authorIDs = append(authorIDs, id)
	}
	cardMap := l.deps.BackfillRepo.LookupAuthorCards(ctx, authorIDs)
	for i := range allItems {
		card := cardMap[allItems[i].UserID]
		allItems[i].Username = card.Username
		allItems[i].AvatarURL = card.AvatarURL
	}

	// 分页截取
	total := int64(len(allItems))
	start := offset
	if start > len(allItems) {
		start = len(allItems)
	}
	end := start + pageSize
	if end > len(allItems) {
		end = len(allItems)
	}
	pageItems := allItems[start:end]

	return &response.PaginatedResp[response.DynamicItem]{
		List:     pageItems,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// LikeDynamic 点赞动态
func (l *DynamicLogic) LikeDynamic(ctx context.Context, userID string, req request.LikeDynamicReq) error {
	userIDUint, _ := strconv.ParseUint(userID, 10, 64)
	if userIDUint == 0 {
		return errors.New("invalid user id")
	}

	if err := l.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); err != nil {
		return fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLWriteSem.Release(1)

	var created bool
	if err := l.deps.Breakers.MySQL.Execute(func() error {
		var err error
		created, err = l.deps.UserDynamicLikeRepo.CreateLike(ctx, userIDUint, req.DynamicID)
		return err
	}); err != nil {
		return fmt.Errorf("点赞失败")
	}

	if created {
		// 真正新增点赞：动态 like_count +1
		if err := l.deps.Breakers.MySQL.Execute(func() error {
			return l.deps.UserDynamicRepo.IncrementLikeCount(ctx, req.DynamicID, 1)
		}); err != nil {
			l.deps.Logger.Warn("动态 like_count 自增失败",
				zap.Uint64("dynamic_id", req.DynamicID), zap.Error(err))
		}
	}
	return nil
}

// UnlikeDynamic 取消点赞动态
func (l *DynamicLogic) UnlikeDynamic(ctx context.Context, userID string, dynamicID uint64) error {
	userIDUint, _ := strconv.ParseUint(userID, 10, 64)

	if err := l.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); err != nil {
		return fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLWriteSem.Release(1)

	var deleted bool
	if err := l.deps.Breakers.MySQL.Execute(func() error {
		var err error
		deleted, err = l.deps.UserDynamicLikeRepo.DeleteLike(ctx, userIDUint, dynamicID)
		return err
	}); err != nil {
		return fmt.Errorf("取消点赞失败")
	}

	if deleted {
		if err := l.deps.Breakers.MySQL.Execute(func() error {
			return l.deps.UserDynamicRepo.IncrementLikeCount(ctx, dynamicID, -1)
		}); err != nil {
			l.deps.Logger.Warn("动态 like_count 自减失败",
				zap.Uint64("dynamic_id", dynamicID), zap.Error(err))
		}
	}
	return nil
}
