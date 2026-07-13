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

// ArticleHandler 专栏文章 HTTP 处理层。
type ArticleHandler struct {
	logic  *logic.LogicGroup
	logger *zap.Logger
}

// SaveDraft 保存文章草稿。
//
// 路由：POST /article/draft（私有组，需登录）
// 请求体：ArticleDraftReq
// 响应：{article_id: uint64}
func (h *ArticleHandler) SaveDraft(c *gin.Context) {
	var req request.ArticleDraftReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, "参数错误")
		return
	}

	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	articleID, err := h.logic.ArticleLogic.SaveDraft(c.Request.Context(), userID, req)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithData(c, gin.H{"article_id": articleID})
}

// PublishArticle 发布文章。
//
// 路由：POST /article/publish（私有组，需登录）
// 请求体：ArticlePublishReq
// 响应：success 消息
func (h *ArticleHandler) PublishArticle(c *gin.Context) {
	var req request.ArticlePublishReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, "参数错误")
		return
	}

	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	if err := h.logic.ArticleLogic.PublishArticle(c.Request.Context(), userID, req); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithMsg(c, "发布成功")
}

// GetArticleDetail 查询文章详情。
//
// 路由：GET /article/detail?article_id=（公开组，无需登录）
// 响应：ArticleDetailResp
func (h *ArticleHandler) GetArticleDetail(c *gin.Context) {
	articleIDStr := c.Query("article_id")
	if articleIDStr == "" {
		response.FailWithMsg(c, "参数错误")
		return
	}
	articleID, err := strconv.ParseUint(articleIDStr, 10, 64)
	if err != nil {
		response.FailWithMsg(c, "参数错误")
		return
	}

	data, err := h.logic.ArticleLogic.GetArticleDetail(c.Request.Context(), articleID)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithData(c, data)
}
