package routers

import (
	"fake_tiktok/internal/handler"

	"github.com/gin-gonic/gin"
)

// UserHomeRouter 用户主页路由注册器
type UserHomeRouter struct{}

// InitUserHomeRouter 注册用户主页相关路由
//
// 路由分组：
//   - 公开组：GET /user/videos（用户主页视频列表）
//   - 私有组：GET /user/level（当前登录用户等级）
func (r *UserHomeRouter) InitUserHomeRouter(publicGroup, privateGroup *gin.RouterGroup, handlerGroup *handler.HandlerGroup) {
	userPublic := publicGroup.Group("user")
	{
		userPublic.GET("videos", handlerGroup.UserHomeHandler.ListUserVideos)
	}

	userPrivate := privateGroup.Group("user")
	{
		userPrivate.GET("level", handlerGroup.UserHomeHandler.GetLevel)
	}
}
