package handler

import (
	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/dto/response"
	"fake_tiktok/internal/logic"
	"fake_tiktok/internal/pkg"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// NotificationHandler 通知 HTTP 处理层
type NotificationHandler struct {
	logic  *logic.LogicGroup
	logger *zap.Logger
}

// ListNotifications 分页查询当前用户的通知列表
//
// 路由：GET /notification/list（私有组，需登录）
// 查询参数：type, only_unread, page, page_size
// 响应：PaginatedResp[NotificationItem]
func (h *NotificationHandler) ListNotifications(c *gin.Context) {
	var req request.ListNotificationsReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMsg(c, "参数错误")
		return
	}

	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	data, err := h.logic.NotificationLogic.ListNotifications(c.Request.Context(), userID, req)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithData(c, data)
}

// CountUnread 统计未读通知数
//
// 路由：GET /notification/unread_count（私有组，需登录）
// 响应：UnreadCountResp
func (h *NotificationHandler) CountUnread(c *gin.Context) {
	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	data, err := h.logic.NotificationLogic.CountUnread(c.Request.Context(), userID)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithData(c, data)
}

// MarkRead 标记单条通知为已读
//
// 路由：POST /notification/read（私有组，需登录）
// 请求体：MarkNotificationReadReq
func (h *NotificationHandler) MarkRead(c *gin.Context) {
	var req request.MarkNotificationReadReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, "参数错误：notification_id 必填")
		return
	}

	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	if err := h.logic.NotificationLogic.MarkRead(c.Request.Context(), userID, req); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithMsg(c, "已标记已读")
}

// MarkAllRead 标记所有未读通知为已读
//
// 路由：POST /notification/read_all（私有组，需登录）
func (h *NotificationHandler) MarkAllRead(c *gin.Context) {
	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	if err := h.logic.NotificationLogic.MarkAllRead(c.Request.Context(), userID); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithMsg(c, "全部已读")
}

// MuteLikeNotif 静默某条评论的点赞通知
//
// 路由：POST /notification/mute_like（私有组，需登录）
// 请求体：MuteLikeNotifReq
func (h *NotificationHandler) MuteLikeNotif(c *gin.Context) {
	var req request.MuteLikeNotifReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, "参数错误：comment_id 必填")
		return
	}

	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	if err := h.logic.NotificationLogic.MuteLikeNotif(c.Request.Context(), userID, req); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithMsg(c, "已静默")
}

// Delete 删除单条通知
//
// 路由：POST /notification/delete（私有组，需登录）
// 请求体：DeleteNotificationReq
func (h *NotificationHandler) Delete(c *gin.Context) {
	var req request.DeleteNotificationReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, "参数错误：notification_id 必填")
		return
	}

	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	if err := h.logic.NotificationLogic.Delete(c.Request.Context(), userID, req); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithMsg(c, "已删除")
}
