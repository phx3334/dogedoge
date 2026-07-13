package handler

import (
	"strconv"

	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/dto/response"
	"fake_tiktok/internal/logic"
	"fake_tiktok/internal/pkg"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// DynamicHandler 用户动态 HTTP 处理层
type DynamicHandler struct {
	logic  *logic.LogicGroup
	logger *zap.Logger
}

// CreateDynamic 发布动态
//
// 路由：POST /dynamic/create（私有组，需登录）
// 请求体：CreateDynamicReq
// 响应：{dynamic_id: uint64}
func (h *DynamicHandler) CreateDynamic(c *gin.Context) {
	var req request.CreateDynamicReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, "参数错误：标题最长 20 字符，内容最长 233 字符")
		return
	}

	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	dynamicID, err := h.logic.DynamicLogic.CreateDynamic(c.Request.Context(), userID, req)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithData(c, gin.H{"dynamic_id": dynamicID})
}

// ListUserDynamics 分页查询指定用户的动态
//
// 路由：GET /dynamic/user（公开组）
// 查询参数：user_id, page, page_size
// 响应：PaginatedResp[DynamicItem]
func (h *DynamicHandler) ListUserDynamics(c *gin.Context) {
	var req request.ListUserDynamicsReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMsg(c, "参数错误：user_id 必填")
		return
	}

	// 当前登录用户（可能为空，未登录时 is_liked 恒为 false）
	currentUserID := pkg.GetUserID(c, h.logic.Config)

	data, err := h.logic.DynamicLogic.ListUserDynamics(c.Request.Context(), req.UserID, currentUserID, req)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithData(c, data)
}

// ListDynamicFeed 查询关注用户的最新动态流
//
// 路由：GET /dynamic/feed（私有组，需登录）
// 查询参数：page, page_size
// 响应：PaginatedResp[DynamicItem]
func (h *DynamicHandler) ListDynamicFeed(c *gin.Context) {
	var req request.ListDynamicFeedReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMsg(c, "参数错误")
		return
	}

	currentUserID := pkg.GetUserID(c, h.logic.Config)
	if currentUserID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	data, err := h.logic.DynamicLogic.ListDynamicFeed(c.Request.Context(), currentUserID, req)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithData(c, data)
}

// LikeDynamic 点赞动态
//
// 路由：POST /dynamic/like（私有组，需登录）
// 请求体：LikeDynamicReq
func (h *DynamicHandler) LikeDynamic(c *gin.Context) {
	var req request.LikeDynamicReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, "参数错误：dynamic_id 必填")
		return
	}

	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	if err := h.logic.DynamicLogic.LikeDynamic(c.Request.Context(), userID, req); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithMsg(c, "已点赞")
}

// UnlikeDynamic 取消点赞动态
//
// 路由：DELETE /dynamic/like（私有组，需登录）
// 查询参数：dynamic_id
func (h *DynamicHandler) UnlikeDynamic(c *gin.Context) {
	dynamicIDStr := c.Query("dynamic_id")
	if dynamicIDStr == "" {
		response.FailWithMsg(c, "参数错误：dynamic_id 必填")
		return
	}

	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	dynamicID, err := strconv.ParseUint(dynamicIDStr, 10, 64)
	if err != nil || dynamicID == 0 {
		response.FailWithMsg(c, "参数错误：dynamic_id 非法")
		return
	}

	if err := h.logic.DynamicLogic.UnlikeDynamic(c.Request.Context(), userID, dynamicID); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithMsg(c, "已取消点赞")
}
