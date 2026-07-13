package handler

import (
	"strings"
	"unicode"

	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/dto/response"
	"fake_tiktok/internal/logic"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type SearchHandler struct {
	logic  *logic.LogicGroup
	logger *zap.Logger
}

// sanitizeKeyword 清洗搜索关键词，防止极端输入导致 ES 性能问题
// 1. 去除首尾空白字符
// 2. 将连续空白字符合并为单个空格
// 3. 过滤纯空白/控制字符的输入
func sanitizeKeyword(keyword string) string {
	// 去除首尾空白
	keyword = strings.TrimSpace(keyword)

	// 将连续空白（空格、制表符、换行等）合并为单个空格，
	// 防止大量空白字符被 ik 分词器展开为过多 token
	var b strings.Builder
	b.Grow(len(keyword))
	prevSpace := false
	for _, r := range keyword {
		if unicode.IsSpace(r) {
			if !prevSpace {
				b.WriteRune(' ')
				prevSpace = true
			}
			continue
		}
		// 过滤控制字符（如 \x00-\x1F 中的非空白字符）
		if unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
		prevSpace = false
	}

	return strings.TrimSpace(b.String())
}

func (h *SearchHandler) SearchVideos(c *gin.Context) {
	var req request.SearchVideoReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMsg(c, "参数错误")
		return
	}

	// 清洗关键词：去除多余空白和控制字符，防止极端输入影响 ES 性能
	req.Keyword = sanitizeKeyword(req.Keyword)
	if req.Keyword == "" {
		response.FailWithMsg(c, "搜索关键词不能为空")
		return
	}

	videos, nextCursor, err := h.logic.SearchLogic.SearchVideos(c.Request.Context(), req)
	if err != nil {
		h.logger.Error("搜索视频失败", zap.Error(err))
		response.FailWithMsg(c, "搜索失败")
		return
	}

	response.OkWithData(c, gin.H{
		"list":        videos,
		"next_cursor": nextCursor,
		"has_more":    nextCursor != "",
	})
}
