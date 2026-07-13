package handler

import (
	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/dto/response"
	"fake_tiktok/internal/logic"
	"fake_tiktok/internal/pkg"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// UserHomeHandler 用户主页 HTTP 处理层
type UserHomeHandler struct {
	logic  *logic.LogicGroup
	logger *zap.Logger
}

// ListUserVideos 用户主页视频列表
//
// 路由：GET /user/videos（公开组）
// 查询参数：user_id, page, page_size
// 响应：PaginatedResp[HomeVideoInfo]
func (h *UserHomeHandler) ListUserVideos(c *gin.Context) {
	var req request.ListUserVideosReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMsg(c, "参数错误：user_id 必填")
		return
	}

	data, err := h.logic.UserHomeLogic.ListUserVideos(c.Request.Context(), req)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithData(c, data)
}

// GetLevel 获取当前登录用户的等级信息
//
// 路由：GET /user/level（私有组，需登录）
// 响应：UserLevelResp
func (h *UserHomeHandler) GetLevel(c *gin.Context) {
	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	data, err := h.logic.UserHomeLogic.GetLevel(c.Request.Context(), userID)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithData(c, data)
}
