package handler

import (
	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/dto/response"
	"fake_tiktok/internal/logic"
	"fake_tiktok/internal/pkg"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// MessageHandler 私信 HTTP 处理层
type MessageHandler struct {
	logic  *logic.LogicGroup
	logger *zap.Logger
}

// SendMessage 发送私信（私有组，需登录）
func (h *MessageHandler) SendMessage(c *gin.Context) {
	var req request.SendMessageReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, "参数错误：recipient_id 和 content 必填")
		return
	}
	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	data, err := h.logic.MessageLogic.SendMessage(c.Request.Context(), userID, req)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithData(c, data)
}

// ListConversations 会话列表（私有组，需登录）
func (h *MessageHandler) ListConversations(c *gin.Context) {
	var req request.ListConversationsReq
	_ = c.ShouldBindQuery(&req)
	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	data, total, err := h.logic.MessageLogic.ListConversations(c.Request.Context(), userID, req.Page, req.PageSize)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithData(c, &response.PaginatedResp[response.ConversationItem]{
		List:     data,
		Total:    total,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
}

// GetMessages 与某对端的私信历史（私有组，需登录）
func (h *MessageHandler) GetMessages(c *gin.Context) {
	var req request.ListMessagesReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMsg(c, "参数错误：peer_id 必填")
		return
	}
	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	data, total, err := h.logic.MessageLogic.GetMessages(c.Request.Context(), userID, req.PeerID, req.Page, req.PageSize)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithData(c, &response.PaginatedResp[response.MessageItem]{
		List:     data,
		Total:    total,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
}

// MarkRead 标记与某对端的私信为已读（私有组，需登录）
func (h *MessageHandler) MarkRead(c *gin.Context) {
	var req request.MarkMessageReadReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, "参数错误：peer_id 必填")
		return
	}
	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	if _, err := h.logic.MessageLogic.MarkRead(c.Request.Context(), userID, req.PeerID); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithMsg(c, "ok")
}

// CountUnread 私信未读数（私有组，需登录）
func (h *MessageHandler) CountUnread(c *gin.Context) {
	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	n, err := h.logic.MessageLogic.CountUnread(c.Request.Context(), userID)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithData(c, &response.MessageUnreadResp{Count: n})
}
