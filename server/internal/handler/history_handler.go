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

// HistoryHandler 用户历史 HTTP 处理层（观看历史 / 阅读历史 / 搜索历史）
type HistoryHandler struct {
	logic  *logic.LogicGroup
	logger *zap.Logger
}

// =============================================================================
// 视频观看历史
// =============================================================================

// RecordVideoView 记录视频观看进度
//
// 路由：POST /history/video/view（私有组，需登录）
// 请求体：RecordVideoViewReq
func (h *HistoryHandler) RecordVideoView(c *gin.Context) {
	var req request.RecordVideoViewReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, "参数错误")
		return
	}

	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	if err := h.logic.HistoryLogic.RecordVideoView(c.Request.Context(), userID, req); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithMsg(c, "已记录")
}

// ListVideoHistory 分页查询视频观看历史
//
// 路由：GET /history/video/list（私有组，需登录）
// 查询参数：page, page_size
// 响应：PaginatedResp[VideoHistoryItem]
func (h *HistoryHandler) ListVideoHistory(c *gin.Context) {
	var req request.ListVideoHistoryReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMsg(c, "参数错误")
		return
	}

	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	data, err := h.logic.HistoryLogic.ListVideoHistory(c.Request.Context(), userID, req)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithData(c, data)
}

// DeleteVideoHistory 删除单条视频观看历史
//
// 路由：DELETE /history/video（私有组，需登录）
// 请求体：DeleteVideoHistoryReq
func (h *HistoryHandler) DeleteVideoHistory(c *gin.Context) {
	var req request.DeleteVideoHistoryReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, "参数错误：video_id 必填")
		return
	}

	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	if err := h.logic.HistoryLogic.DeleteVideoHistory(c.Request.Context(), userID, req); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithMsg(c, "已删除")
}

// ClearVideoHistory 清空视频观看历史
//
// 路由：POST /history/video/clear（私有组，需登录）
func (h *HistoryHandler) ClearVideoHistory(c *gin.Context) {
	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	if err := h.logic.HistoryLogic.ClearVideoHistory(c.Request.Context(), userID); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithMsg(c, "已清空")
}

// =============================================================================
// 文章阅读历史
// =============================================================================

// ListArticleHistory 分页查询文章阅读历史
//
// 路由：GET /history/article/list（私有组，需登录）
// 查询参数：page, page_size
// 响应：PaginatedResp[ArticleHistoryItem]
func (h *HistoryHandler) ListArticleHistory(c *gin.Context) {
	var req request.ListArticleHistoryReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMsg(c, "参数错误")
		return
	}

	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	data, err := h.logic.HistoryLogic.ListArticleHistory(c.Request.Context(), userID, req)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithData(c, data)
}

// DeleteArticleHistory 删除单条文章阅读历史
//
// 路由：DELETE /history/article（私有组，需登录）
// 请求体：DeleteArticleHistoryReq
func (h *HistoryHandler) DeleteArticleHistory(c *gin.Context) {
	var req request.DeleteArticleHistoryReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, "参数错误：article_id 必填")
		return
	}

	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	if err := h.logic.HistoryLogic.DeleteArticleHistory(c.Request.Context(), userID, req); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithMsg(c, "已删除")
}

// =============================================================================
// 搜索历史
// =============================================================================

// SaveSearchHistory 保存搜索历史
//
// 路由：POST /history/search（私有组，需登录）
// 请求体：SaveSearchHistoryReq
func (h *HistoryHandler) SaveSearchHistory(c *gin.Context) {
	var req request.SaveSearchHistoryReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, "参数错误：keyword 必填")
		return
	}

	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	if err := h.logic.HistoryLogic.SaveSearchHistory(c.Request.Context(), userID, req); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithMsg(c, "已保存")
}

// ListSearchHistory 查询搜索历史
//
// 路由：GET /history/search/list（私有组，需登录）
// 查询参数：limit（默认 20，最大 100）
// 响应：[]SearchHistoryItem
func (h *HistoryHandler) ListSearchHistory(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "20")
	limit, _ := strconv.Atoi(limitStr)

	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	data, err := h.logic.HistoryLogic.ListSearchHistory(c.Request.Context(), userID, limit)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithData(c, data)
}

// DeleteSearchHistory 删除单条搜索历史
//
// 路由：DELETE /history/search（私有组，需登录）
// 请求体：DeleteSearchHistoryReq
func (h *HistoryHandler) DeleteSearchHistory(c *gin.Context) {
	var req request.DeleteSearchHistoryReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, "参数错误：keyword 必填")
		return
	}

	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	if err := h.logic.HistoryLogic.DeleteSearchHistory(c.Request.Context(), userID, req); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithMsg(c, "已删除")
}

// ClearSearchHistory 清空搜索历史
//
// 路由：POST /history/search/clear（私有组，需登录）
func (h *HistoryHandler) ClearSearchHistory(c *gin.Context) {
	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	if err := h.logic.HistoryLogic.ClearSearchHistory(c.Request.Context(), userID); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithMsg(c, "已清空")
}
