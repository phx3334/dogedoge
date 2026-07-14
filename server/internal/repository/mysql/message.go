package mysql

import (
	"context"
	"time"

	"fake_tiktok/internal/domain/database"

	"gorm.io/gorm"
)

// MessageRepo 私信表的 MySQL 数据存储
type MessageRepo struct {
	db *gorm.DB
}

func NewMessageRepo(db *gorm.DB) *MessageRepo {
	return &MessageRepo{db: db}
}

// Create 插入一条私信
func (r *MessageRepo) Create(ctx context.Context, m *database.Message) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Create(m).Error
}

// ListConversations 聚合会话列表
//
// 思路：用 UNION 把「我发出 / 我收到」的消息统一成 (peer, created_at, recipient_id, read_at) 三列，
// 再按 peer GROUP BY，取 MAX(created_at) 作为会话最近时间，并统计「我收到且未读」的条数。
func (r *MessageRepo) ListConversations(ctx context.Context, userID string, page, pageSize int) ([]database.MessageConversation, int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 20
	}

	var total int64
	if err := r.db.WithContext(ctx).
		Raw(`SELECT COUNT(DISTINCT peer) FROM (
			SELECT sender_id AS peer FROM messages WHERE recipient_id = ?
			UNION
			SELECT recipient_id AS peer FROM messages WHERE sender_id = ?
		) t`, userID, userID).Scan(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 || page < 1 {
		return nil, total, nil
	}
	offset := (page - 1) * pageSize

	var rows []database.MessageConversation
	if err := r.db.WithContext(ctx).
		Raw(`SELECT peer AS peer_id, MAX(created_at) AS last_at,
			SUM(CASE WHEN recipient_id = ? AND read_at IS NULL THEN 1 ELSE 0 END) AS unread_count
			FROM (
				SELECT sender_id AS peer, created_at, recipient_id, read_at FROM messages WHERE recipient_id = ?
				UNION ALL
				SELECT recipient_id AS peer, created_at, recipient_id, read_at FROM messages WHERE sender_id = ?
			) t
			GROUP BY peer ORDER BY last_at DESC LIMIT ? OFFSET ?`,
			userID, userID, userID, pageSize, offset).
		Scan(&rows).Error; err != nil {
		return nil, total, err
	}
	return rows, total, nil
}

// ListWithPeer 分页查询与某对端的全部私信
func (r *MessageRepo) ListWithPeer(ctx context.Context, userID, peerID string, page, pageSize int) ([]database.Message, int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 30
	}

	query := r.db.WithContext(ctx).Model(&database.Message{}).
		Where("((sender_id = ? AND recipient_id = ?) OR (sender_id = ? AND recipient_id = ?))", userID, peerID, peerID, userID)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return nil, total, nil
	}
	offset := (page - 1) * pageSize

	var items []database.Message
	if err := query.Order("created_at DESC").
		Offset(offset).Limit(pageSize).
		Find(&items).Error; err != nil {
		return nil, total, err
	}
	// 倒序分页取出后，翻转成时间升序，便于前端按对话顺序展示
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
	return items, total, nil
}

// MarkReadWithPeer 标记与某对端的私信为已读（仅收件人是当前用户且未读）
func (r *MessageRepo) MarkReadWithPeer(ctx context.Context, userID, peerID string) (int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	now := time.Now()
	res := r.db.WithContext(ctx).Model(&database.Message{}).
		Where("recipient_id = ? AND sender_id = ? AND read_at IS NULL", userID, peerID).
		Update("read_at", &now)
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

// CountUnread 统计未读私信数
func (r *MessageRepo) CountUnread(ctx context.Context, userID string) (int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var count int64
	err := r.db.WithContext(ctx).Model(&database.Message{}).
		Where("recipient_id = ? AND read_at IS NULL", userID).
		Count(&count).Error
	return count, err
}

// CountFromTo 统计 sender→recipient 方向的私信条数。
// 用于非互关场景限制"只能向对方发送一条消息"。
func (r *MessageRepo) CountFromTo(ctx context.Context, sender, recipient string) (int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var count int64
	err := r.db.WithContext(ctx).Model(&database.Message{}).
		Where("sender_id = ? AND recipient_id = ?", sender, recipient).
		Count(&count).Error
	return count, err
}
