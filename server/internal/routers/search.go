package routers

import (
	"fake_tiktok/internal/handler"

	"github.com/gin-gonic/gin"
)

type SearchRouter struct{}

func (s *SearchRouter) InitSearchRouter(publicGroup *gin.RouterGroup, handlerGroup *handler.HandlerGroup) {
	searchRouter := publicGroup.Group("search")
	{
		searchRouter.GET("video", handlerGroup.SearchHandler.SearchVideos)
	}
}
