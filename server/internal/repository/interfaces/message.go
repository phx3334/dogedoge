package interfaces

import (
	"context"

	"fake_tiktok/internal/domain/database"
)

// MessageRepository 用户间私信数据接口
//
// 私信采用单表存储，每条消息存 sender_id / recipient_id / content / read_at，
// 会话（conversation）是查询时的逻辑概念：按对端用户聚合得到。
type MessageRepository interface {
	// Create 写入一条私信
	Create(ctx context.Context, m *database.Message) error

	// ListConversations 聚合当前用户的所有会话（每个对端一条），按最近消息时间倒序
	ListConversations(ctx context.Context, userID string, page, pageSize int) ([]database.MessageConversation, int64, error)

	// ListWithPeer 分页查询当前用户与某个对端的全部私信（按创建时间倒序分页，返回时按时间升序便于前端正序展示）
	ListWithPeer(ctx context.Context, userID, peerID string, page, pageSize int) ([]database.Message, int64, error)

	// MarkReadWithPeer 把「recipient=当前用户 且 sender=对端」的未读私信标记为已读，返回更新条数
	MarkReadWithPeer(ctx context.Context, userID, peerID string) (int64, error)

	// CountUnread 统计当前用户的未读私信数
	CountUnread(ctx context.Context, userID string) (int64, error)

	// CountFromTo 统计 sender→recipient 方向的私信条数（用于非互关时限制仅能发 1 条）
	CountFromTo(ctx context.Context, sender, recipient string) (int64, error)
}
