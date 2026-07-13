package routers

import (
	"fake_tiktok/internal/breaker"
	"fake_tiktok/internal/config"
	"fake_tiktok/internal/handler"
	"fake_tiktok/internal/handler/ws"
	"fake_tiktok/internal/repository/interfaces"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type VideoRouter struct{}

func (v *VideoRouter) InitVideoRouter(publicGroup, privateGroup *gin.RouterGroup, handlerGroup *handler.HandlerGroup, hub *ws.DanmakuHub, cfg *config.Config, accountRepo interfaces.AccountRepository, danmakuRepo interfaces.DanmakuRepository, danmakuCacheRepo interfaces.DanmakuCacheRepository, danmakuPubSub interfaces.DanmakuPubSub, videoRepo interfaces.VideoRepository, videoCacheRepo interfaces.VideoCacheRepository, breakers *breaker.Group, logger *zap.Logger) {
	videorouter := publicGroup.Group("video")
	{
		videorouter.GET("list", handlerGroup.VideoHandler.ListPublishedVideos)
		videorouter.GET("detail", handlerGroup.VideoHandler.GetVideoDetail)
	}

	publicGroup.GET("ws/danmaku", func(c *gin.Context) {
		ws.ServeWS(hub, c.Writer, c.Request, &cfg.JWT, accountRepo, danmakuRepo, danmakuCacheRepo, danmakuPubSub, videoRepo, videoCacheRepo, breakers, logger)
	})

	// Phase 5: 视频草稿上传与转码状态查询（私有组，需登录）
	videodraftrouter := privateGroup.Group("video")
	{
		videodraftrouter.POST("draft/upload", handlerGroup.VideoDraftHandler.UploadDraft)
		videodraftrouter.GET("draft/status", handlerGroup.VideoDraftHandler.GetStatus)
		// 作者删除自己投稿的视频（权限校验在 Logic 层：操作者须为视频作者）
		videodraftrouter.DELETE("", handlerGroup.VideoHandler.DeleteVideo)
	}
}
