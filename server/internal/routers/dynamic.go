package routers

import (
	"fake_tiktok/internal/handler"

	"github.com/gin-gonic/gin"
)

// DynamicRouter 用户动态路由注册器
type DynamicRouter struct{}

// InitDynamicRouter 注册用户动态相关路由
//
// 路由分组：
//   - 公开组：GET /dynamic/user（指定用户动态列表）
//   - 私有组：
//   - POST   /dynamic/create  发布动态
//   - GET    /dynamic/feed    关注用户动态流
//   - POST   /dynamic/like    点赞动态
//   - DELETE /dynamic/like    取消点赞
func (r *DynamicRouter) InitDynamicRouter(publicGroup, privateGroup *gin.RouterGroup, handlerGroup *handler.HandlerGroup) {
	// 公开路由
	dynamicPublic := publicGroup.Group("dynamic")
	{
		dynamicPublic.GET("user", handlerGroup.DynamicHandler.ListUserDynamics)
		// 个人主页"动态"Tab：混合视频+文章+图文动态
		dynamicPublic.GET("user-mixed", handlerGroup.DynamicHandler.ListUserMixedDynamics)
	}

	// 私有路由
	dynamicPrivate := privateGroup.Group("dynamic")
	{
		dynamicPrivate.POST("create", handlerGroup.DynamicHandler.CreateDynamic)
		dynamicPrivate.GET("feed", handlerGroup.DynamicHandler.ListDynamicFeed)
		dynamicPrivate.POST("like", handlerGroup.DynamicHandler.LikeDynamic)
		dynamicPrivate.DELETE("like", handlerGroup.DynamicHandler.UnlikeDynamic)
	}
}
