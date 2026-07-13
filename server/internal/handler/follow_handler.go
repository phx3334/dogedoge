package handler

import (
	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/dto/response"
	"fake_tiktok/internal/logic"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// FollowHandler 关注/粉丝列表 HTTP 处理层
type FollowHandler struct {
	logic  *logic.LogicGroup
	logger *zap.Logger
}

// ListFollowers 分页查询某用户的粉丝列表
//
// 路由：GET /follow/followers（公开组）
// 查询参数：user_id, page, page_size
// 响应：PaginatedResp[FollowUserItem]
func (h *FollowHandler) ListFollowers(c *gin.Context) {
	var req request.ListFollowersReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMsg(c, "参数错误：user_id 必填")
		return
	}

	data, err := h.logic.FollowLogic.ListFollowers(c.Request.Context(), req)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithData(c, data)
}

// ListFollowing 分页查询某用户的关注列表
//
// 路由：GET /follow/following（公开组）
// 查询参数：user_id, page, page_size
// 响应：PaginatedResp[FollowUserItem]
func (h *FollowHandler) ListFollowing(c *gin.Context) {
	var req request.ListFollowingReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMsg(c, "参数错误：user_id 必填")
		return
	}

	data, err := h.logic.FollowLogic.ListFollowing(c.Request.Context(), req)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithData(c, data)
}
