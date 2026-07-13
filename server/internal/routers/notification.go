package routers

import (
	"fake_tiktok/internal/handler"

	"github.com/gin-gonic/gin"
)

// NotificationRouter 通知路由注册器
type NotificationRouter struct{}

// InitNotificationRouter 注册通知相关路由
//
// 路由分组（全部私有组，需登录）：
//   - GET  /notification/list           通知列表
//   - GET  /notification/unread_count   未读数
//   - POST /notification/read           标记单条已读
//   - POST /notification/read_all       全部已读
//   - POST /notification/mute_like      静默评论点赞通知
//   - POST /notification/delete         删除单条通知
func (r *NotificationRouter) InitNotificationRouter(privateGroup *gin.RouterGroup, handlerGroup *handler.HandlerGroup) {
	notif := privateGroup.Group("notification")
	{
		notif.GET("list", handlerGroup.NotificationHandler.ListNotifications)
		notif.GET("unread_count", handlerGroup.NotificationHandler.CountUnread)
		notif.POST("read", handlerGroup.NotificationHandler.MarkRead)
		notif.POST("read_all", handlerGroup.NotificationHandler.MarkAllRead)
		notif.POST("mute_like", handlerGroup.NotificationHandler.MuteLikeNotif)
		notif.POST("delete", handlerGroup.NotificationHandler.Delete)
	}
}
