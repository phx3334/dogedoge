package handler

import (
	"fmt"
	"net/http"

	"fake_tiktok/internal/dto/response"
	"fake_tiktok/internal/logic"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AIHandler AI 聊天 HTTP 处理层
type AIHandler struct {
	logic  *logic.LogicGroup
	logger *zap.Logger
}

// ListCharacters 返回可用的 AI 角色列表
// 路由：GET /ai/characters（公开组，无需登录）
func (h *AIHandler) ListCharacters(c *gin.Context) {
	response.OkWithData(c, gin.H{
		"list": h.logic.AILogic.ListCharacters(),
	})
}

// ChatReq AI 聊天请求体
type ChatReq struct {
	CharacterID string             `json:"character_id" binding:"required"`
	Messages    []logic.ChatMessage `json:"messages" binding:"required"`
}

// Chat 与 AI 角色对话
// 路由：POST /ai/chat（私有组，需登录）
func (h *AIHandler) Chat(c *gin.Context) {
	var req ChatReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, "参数错误：character_id 和 messages 必填")
		return
	}

	if len(req.Messages) == 0 {
		response.FailWithMsg(c, "消息不能为空")
		return
	}

	// 限制历史消息数量，避免 token 超限
	if len(req.Messages) > 20 {
		req.Messages = req.Messages[len(req.Messages)-20:]
	}

	// 流式 SSE 响应
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		response.FailWithMsg(c, "当前环境不支持流式响应")
		return
	}

	err := h.logic.AILogic.ChatStream(c.Request.Context(), req.CharacterID, req.Messages,
		func(content string) {
			fmt.Fprintf(c.Writer, "data: {\"content\":%s}\n\n", logic.JsonString(content))
			flusher.Flush()
		})
	if err != nil {
		h.logger.Warn("AI 聊天失败", zap.Error(err))
		fmt.Fprintf(c.Writer, "data: {\"error\":%s}\n\n", logic.JsonString(err.Error()))
		flusher.Flush()
	}
	fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
	flusher.Flush()
}
