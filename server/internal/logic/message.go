package logic

import (
	"context"
	"fmt"
	"strings"
	"time"

	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/dto/response"
)

// MessageLogic 用户间私信业务逻辑
type MessageLogic struct {
	deps *LogicDeps
}

func NewMessageLogic(deps *LogicDeps) *MessageLogic {
	return &MessageLogic{deps: deps}
}

// SendMessage 发送一条私信（幂等于"不能发给自己"，并对收件人存在性做校验）
func (l *MessageLogic) SendMessage(ctx context.Context, senderID string, req request.SendMessageReq) (*response.MessageItem, error) {
	if req.RecipientID == "" {
		return nil, fmt.Errorf("收件人不能为空")
	}
	if req.RecipientID == senderID {
		return nil, fmt.Errorf("不能给自己发私信")
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		return nil, fmt.Errorf("私信内容不能为空")
	}

	if err := l.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); err != nil {
		return nil, fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLWriteSem.Release(1)

	// 校验收件人真实存在
	recipient, rerr := l.deps.AccountRepo.FindByID(ctx, req.RecipientID)
	if rerr != nil || recipient == nil {
		return nil, fmt.Errorf("收件人不存在")
	}

	// 私信权限校验：
	//   - 互相关注（双向都已关注）→ 无限私信
	//   - 否则（单向关注 / 互未关注）→ sender 向 recipient 至多发 1 条消息，
	//     超出则返回明确提示，避免陌生人滥发。
	mutual, merr := l.deps.InteractionRepo.IsMutualFollow(ctx, senderID, req.RecipientID)
	if merr != nil {
		return nil, fmt.Errorf("服务繁忙，请稍后重试")
	}
	if !mutual {
		sent, cerr := l.deps.MessageRepo.CountFromTo(ctx, senderID, req.RecipientID)
		if cerr != nil {
			return nil, fmt.Errorf("服务繁忙，请稍后重试")
		}
		if sent >= 1 {
			return nil, fmt.Errorf("互未关注，仅能向对方发送一条消息")
		}
	}

	now := time.Now()
	m := &database.Message{
		SenderID:    senderID,
		RecipientID: req.RecipientID,
		Content:     content,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := l.deps.Breakers.MySQL.Execute(func() error {
		return l.deps.MessageRepo.Create(ctx, m)
	}); err != nil {
		return nil, fmt.Errorf("发送失败，请稍后重试")
	}

	return &response.MessageItem{
		ID:          m.ID,
		SenderID:    m.SenderID,
		RecipientID: m.RecipientID,
		Content:     m.Content,
		IsRead:      false,
		CreatedAt:   m.CreatedAt,
	}, nil
}

// ListConversations 当前用户的会话列表（含对端资料、最后一条消息、未读数）
func (l *MessageLogic) ListConversations(ctx context.Context, userID string, page, pageSize int) ([]response.ConversationItem, int64, error) {
	if err := l.deps.Breakers.MySQLReadSem.Acquire(ctx, 1); err != nil {
		return nil, 0, fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLReadSem.Release(1)

	convs, total, err := l.deps.MessageRepo.ListConversations(ctx, userID, page, pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("查询会话失败")
	}
	if len(convs) == 0 {
		return nil, total, nil
	}

	peerIDs := make([]string, 0, len(convs))
	for _, c := range convs {
		peerIDs = append(peerIDs, c.PeerID)
	}
	cardMap := l.deps.BackfillRepo.LookupAuthorCards(ctx, peerIDs)

	items := make([]response.ConversationItem, 0, len(convs))
	for _, c := range convs {
		card := cardMap[c.PeerID]
		lastContent := ""
		if msgs, _, lerr := l.deps.MessageRepo.ListWithPeer(ctx, userID, c.PeerID, 1, 1); lerr == nil && len(msgs) > 0 {
			lastContent = msgs[0].Content
		}
		items = append(items, response.ConversationItem{
			PeerID:      c.PeerID,
			PeerName:    card.Username,
			PeerAvatar:  card.AvatarURL,
			LastContent: lastContent,
			LastAt:      c.LastAt,
			UnreadCount: c.UnreadCount,
		})
	}
	return items, total, nil
}

// GetMessages 与某对端的私信历史（时间升序）
func (l *MessageLogic) GetMessages(ctx context.Context, userID, peerID string, page, pageSize int) ([]response.MessageItem, int64, error) {
	if err := l.deps.Breakers.MySQLReadSem.Acquire(ctx, 1); err != nil {
		return nil, 0, fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLReadSem.Release(1)

	msgs, total, err := l.deps.MessageRepo.ListWithPeer(ctx, userID, peerID, page, pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("查询消息失败")
	}
	items := make([]response.MessageItem, 0, len(msgs))
	for _, m := range msgs {
		items = append(items, response.MessageItem{
			ID:          m.ID,
			SenderID:    m.SenderID,
			RecipientID: m.RecipientID,
			Content:     m.Content,
			IsRead:      m.ReadAt != nil,
			CreatedAt:   m.CreatedAt,
		})
	}
	return items, total, nil
}

// MarkRead 把与某对端的私信标记为已读
func (l *MessageLogic) MarkRead(ctx context.Context, userID, peerID string) (int64, error) {
	if err := l.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); err != nil {
		return 0, fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLWriteSem.Release(1)

	n, err := l.deps.MessageRepo.MarkReadWithPeer(ctx, userID, peerID)
	if err != nil {
		return 0, fmt.Errorf("标记已读失败")
	}
	return n, nil
}

// CountUnread 当前用户的私信未读数
func (l *MessageLogic) CountUnread(ctx context.Context, userID string) (int64, error) {
	if err := l.deps.Breakers.MySQLReadSem.Acquire(ctx, 1); err != nil {
		return 0, fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLReadSem.Release(1)

	n, err := l.deps.MessageRepo.CountUnread(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("查询未读数失败")
	}
	return n, nil
}
