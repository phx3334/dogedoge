package request

// =============================================================================
// 私信
// =============================================================================

// SendMessageReq 发送私信请求
type SendMessageReq struct {
	RecipientID string `json:"recipient_id" binding:"required"`
	Content      string `json:"content" binding:"required,min=1,max=2000"`
}

// MarkMessageReadReq 标记与某对端的私信为已读
type MarkMessageReadReq struct {
	PeerID string `json:"peer_id" binding:"required"`
}

// ListConversationsReq 会话列表请求
type ListConversationsReq struct {
	Page     int `form:"page"`
	PageSize int `form:"page_size"`
}

// ListMessagesReq 与某对端的私信历史请求
type ListMessagesReq struct {
	PeerID   string `form:"peer_id" binding:"required"`
	Page      int    `form:"page"`
	PageSize  int    `form:"page_size"`
}
