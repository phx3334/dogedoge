package logic

import (
	"context"
	"encoding/json"
	"fmt"

	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/dto/response"
	"go.uber.org/zap"
)

// NotificationLogic 通知业务逻辑
type NotificationLogic struct {
	deps *LogicDeps
}

func NewNotificationLogic(deps *LogicDeps) *NotificationLogic {
	return &NotificationLogic{deps: deps}
}

// ListNotifications 分页查询当前用户的通知列表
func (l *NotificationLogic) ListNotifications(ctx context.Context, userID string, req request.ListNotificationsReq) (*response.PaginatedResp[response.NotificationItem], error) {
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

	list, total, err := l.deps.NotificationRepo.ListByRecipient(ctx, userID, req.Type, req.OnlyUnread, req.Page, req.PageSize)
	if err != nil {
		return nil, fmt.Errorf("查询通知失败")
	}

	result := make([]response.NotificationItem, 0, len(list))
	for _, n := range list {
		result = append(result, response.NotificationItem{
			ID:              n.ID,
			Type:            n.Type,
			RelatedID:       n.RelatedID,
			SenderNamesJSON: n.SenderNamesJSON,
			TotalLikes:      n.TotalLikes,
			CommentPreview:  n.CommentPreview,
			PayloadJSON:     n.PayloadJSON,
			IsRead:          n.IsRead,
			CreatedAt:       n.CreatedAt,
		})
	}

	return &response.PaginatedResp[response.NotificationItem]{
		List:     result,
		Total:    total,
		Page:     req.Page,
		PageSize: req.PageSize,
	}, nil
}

// CountUnread 统计未读通知数
func (l *NotificationLogic) CountUnread(ctx context.Context, userID string) (*response.UnreadCountResp, error) {
	count, err := l.deps.NotificationRepo.CountUnread(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("查询未读数失败")
	}
	return &response.UnreadCountResp{Count: count}, nil
}

// MarkRead 标记单条通知为已读
func (l *NotificationLogic) MarkRead(ctx context.Context, userID string, req request.MarkNotificationReadReq) error {
	if err := l.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); err != nil {
		return fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLWriteSem.Release(1)

	_, err := l.deps.NotificationRepo.MarkRead(ctx, userID, req.NotificationID)
	if err != nil {
		return fmt.Errorf("标记已读失败")
	}
	// updated=false（通知不存在/不属于该用户/已读）→ 幂等返回成功
	return nil
}

// MarkAllRead 标记所有未读通知为已读
func (l *NotificationLogic) MarkAllRead(ctx context.Context, userID string) error {
	if err := l.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); err != nil {
		return fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLWriteSem.Release(1)

	if err := l.deps.NotificationRepo.MarkAllRead(ctx, userID); err != nil {
		return fmt.Errorf("全部已读失败")
	}
	return nil
}

// MuteLikeNotif 静默某条评论的点赞通知
func (l *NotificationLogic) MuteLikeNotif(ctx context.Context, userID string, req request.MuteLikeNotifReq) error {
	if err := l.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); err != nil {
		return fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLWriteSem.Release(1)

	if err := l.deps.NotificationRepo.MuteLike(ctx, userID, req.CommentID); err != nil {
		return fmt.Errorf("静默失败")
	}
	return nil
}

// Delete 删除单条通知（仅当属于该用户）
func (l *NotificationLogic) Delete(ctx context.Context, userID string, req request.DeleteNotificationReq) error {
	if err := l.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); err != nil {
		return fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLWriteSem.Release(1)

	deleted, err := l.deps.NotificationRepo.Delete(ctx, userID, req.NotificationID)
	if err != nil {
		return fmt.Errorf("删除通知失败")
	}
	if !deleted {
		return fmt.Errorf("通知不存在或无权删除")
	}
	return nil
}

// NotifyPublish 作者发布视频 / 文章后，通知其所有粉丝（best-effort）。
//
// 设计：
//   - 粉丝量可能很大，按页拉取粉丝 ID 并批量写入通知，避免长时间持锁 / 单次巨事务。
//   - 通知内容：Type=new_video|new_article；RelatedID=视频/文章 ID；
//     SenderNamesJSON 为 [作者名]（前端取首个名字展示）；
//     PayloadJSON 携带 {title, target_type, target_id} 供前端跳转。
//   - 任何步骤失败仅记日志，不阻塞发布主流程。
//   - 建议由调用方以 goroutine + context.Background() 异步调用，避免拖慢发布响应。
func (l *NotificationLogic) NotifyPublish(ctx context.Context, authorID, authorName, notifType, relatedID, title string) {
	const pageSize = 200
	for page := 1; ; page++ {
		followerIDs, _, err := l.deps.InteractionRepo.ListFollowers(ctx, authorID, page, pageSize)
		if err != nil {
			l.deps.Logger.Warn("notify publish: list followers failed",
				zap.String("author_id", authorID), zap.Error(err))
			return
		}
		if len(followerIDs) == 0 {
			return
		}

		senderNames, _ := json.Marshal([]string{authorName})
		payload, _ := json.Marshal(map[string]any{
			"title":       title,
			"target_type": notifType,
			"target_id":   relatedID,
		})
		notifs := make([]database.Notification, 0, len(followerIDs))
		for _, fid := range followerIDs {
			notifs = append(notifs, database.Notification{
				RecipientID:     fid,
				Type:             notifType,
				RelatedID:        relatedID,
				SenderNamesJSON:  string(senderNames),
				PayloadJSON:      string(payload),
			})
		}
		if err := l.deps.NotificationRepo.CreateBatch(ctx, notifs); err != nil {
			l.deps.Logger.Warn("notify publish: create batch failed",
				zap.String("author_id", authorID), zap.Error(err))
			return
		}
		if len(followerIDs) < pageSize {
			return
		}
	}
}
