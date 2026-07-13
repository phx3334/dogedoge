package routers

import (
	"fake_tiktok/internal/handler"
	"fake_tiktok/internal/middleware"
	"fake_tiktok/internal/pkg"
	redis_repo "fake_tiktok/internal/repository/redis"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type BaseRouter struct{}

func (r *BaseRouter) InitBaseRouter(router *gin.RouterGroup, handlerGroup *handler.HandlerGroup, redisRepo *redis_repo.RedisClient, logger *zap.Logger) {
	baserouter := router.Group("base")

	baserouter.POST("captcha", middleware.Limit(redisRepo, "captcha", 15, time.Minute, pkg.KeyByIP, logger), handlerGroup.BaseHandler.Captcha)
	baserouter.POST("sendEmainCode", middleware.Limit(redisRepo, "sendEmailCode", 5, time.Minute, pkg.KeyByIP, logger), handlerGroup.BaseHandler.SendEmainlCode)
}
