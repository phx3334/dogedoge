package routers

import (
	"fake_tiktok/internal/handler"

	"github.com/gin-gonic/gin"
)

// MessageRouter 私信相关路由
//
// 路由分组（全部私有组，需登录）：
//   - POST /message/send          发送私信
//   - GET  /message/conversations 会话列表
//   - GET  /message/history       与某对端的私信历史
//   - POST /message/read          标记与某对端已读
//   - GET  /message/unread_count  私信未读数
type MessageRouter struct{}

func (r *MessageRouter) InitMessageRouter(privateGroup *gin.RouterGroup, handlerGroup *handler.HandlerGroup) {
	m := privateGroup.Group("message")
	{
		m.POST("send", handlerGroup.MessageHandler.SendMessage)
		m.GET("conversations", handlerGroup.MessageHandler.ListConversations)
		m.GET("history", handlerGroup.MessageHandler.GetMessages)
		m.POST("read", handlerGroup.MessageHandler.MarkRead)
		m.GET("unread_count", handlerGroup.MessageHandler.CountUnread)
	}
}
