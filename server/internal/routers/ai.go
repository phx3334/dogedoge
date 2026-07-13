package routers

import (
	"fake_tiktok/internal/handler"

	"github.com/gin-gonic/gin"
)

// AIRouter AI 聊天路由注册器
type AIRouter struct{}

// InitAIRouter 注册 AI 聊天相关路由
//
// 路由分组：
//   - GET  /ai/characters  角色列表（公开组，无需登录）
//   - POST /ai/chat        AI 对话（私有组，需登录）
func (r *AIRouter) InitAIRouter(publicGroup, privateGroup *gin.RouterGroup, handlerGroup *handler.HandlerGroup) {
	pub := publicGroup.Group("ai")
	{
		pub.GET("characters", handlerGroup.AIHandler.ListCharacters)
	}

	priv := privateGroup.Group("ai")
	{
		priv.POST("chat", handlerGroup.AIHandler.Chat)
	}
}
