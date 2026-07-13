package routers

import (
	"fake_tiktok/internal/handler"

	"github.com/gin-gonic/gin"
)

type InteractionRouter struct{}

func (i *InteractionRouter) InitInteractionRouter(publicGroup, privateGroup *gin.RouterGroup, handlerGroup *handler.HandlerGroup) {
	// 公开路由：GET 弹幕列表（未登录用户也能查看弹幕）
	publicInteraction := publicGroup.Group("interaction/video")
	{
		publicInteraction.GET("danmaku", handlerGroup.InteractionHandler.GetDanmakuList)
	}

	// 私有路由：需要登录才能访问的操作
	interactionRouter := privateGroup.Group("interaction")
	interactionVideoRouter := interactionRouter.Group("video")
	{
		interactionVideoRouter.POST("like", handlerGroup.InteractionHandler.LikeVideo)
		interactionVideoRouter.POST("favorite", handlerGroup.InteractionHandler.FavoriteVideo)
		interactionVideoRouter.POST("unfavorite", handlerGroup.InteractionHandler.UnfavoriteVideo)
		interactionVideoRouter.POST("unlike", handlerGroup.InteractionHandler.UnlikeVideo)
		interactionVideoRouter.POST("danmaku", handlerGroup.InteractionHandler.SendDanmaku)
	}
	{
		interactionRouter.POST("follow", handlerGroup.InteractionHandler.FollowUser)
		interactionRouter.POST("unfollow", handlerGroup.InteractionHandler.UnfollowUser)
	}
}