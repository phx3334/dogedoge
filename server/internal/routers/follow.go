package routers

import (
	"fake_tiktok/internal/handler"

	"github.com/gin-gonic/gin"
)

// FollowRouter 关注/粉丝列表路由注册器
type FollowRouter struct{}

// InitFollowRouter 注册关注/粉丝列表相关路由
//
// 路由分组：
//   - 公开组：GET /follow/followers、GET /follow/following
func (r *FollowRouter) InitFollowRouter(publicGroup *gin.RouterGroup, handlerGroup *handler.HandlerGroup) {
	follow := publicGroup.Group("follow")
	{
		follow.GET("followers", handlerGroup.FollowHandler.ListFollowers)
		follow.GET("following", handlerGroup.FollowHandler.ListFollowing)
	}
}
