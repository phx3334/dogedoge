package routers

import (
	"fake_tiktok/internal/config"
	"fake_tiktok/internal/handler"
	"fake_tiktok/internal/middleware"
	"fake_tiktok/internal/pkg"
	"fake_tiktok/internal/repository/interfaces"
	redis_repo "fake_tiktok/internal/repository/redis"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type UserRouter struct{}

// addressResolver 接口用于 IP 地址解析，由 middleware.AddressResolver 定义
// 此处通过参数注入，避免路由层依赖 logic 包的全局变量
type addressResolver interface {
	GetAddressByIP(ip string) string
}

func (u *UserRouter) InitUserRouter(privateGroup *gin.RouterGroup, publicGroup *gin.RouterGroup, adminGroup *gin.RouterGroup, handlerGroup *handler.HandlerGroup, loginRepo interfaces.LoginRepository, redisRepo *redis_repo.RedisClient, cfg *config.Config, logger *zap.Logger, resolver addressResolver) {
	userPrivateRouter := privateGroup.Group("user")
	userPublicRouter := publicGroup.Group("user")
	userAdminRouter := adminGroup.Group("user")
	userLoginRouter := publicGroup.Group("user").Use(middleware.Limit(redisRepo, "login", 8, time.Minute, pkg.KeyByIP, logger)).Use(middleware.LoginRecord(loginRepo, resolver, logger))

	{
		userLoginRouter.POST("/login", handlerGroup.UserHandler.Login)
		userLoginRouter.POST("/register", handlerGroup.UserHandler.Register)
	}

	{
		userPrivateRouter.POST("logout", handlerGroup.UserHandler.Logout)
		userPrivateRouter.GET("info", handlerGroup.UserHandler.PersonalInfo)
		userPrivateRouter.PUT("changeInfo", middleware.Limit(redisRepo, "changeInfo", 20, time.Minute, func(c *gin.Context) (string, bool) {
			return pkg.KeyByUserID(c, cfg)
		}, logger), handlerGroup.UserHandler.UserChangeInfo)
		userPrivateRouter.POST("avatar", middleware.Limit(redisRepo, "avatar", 10, time.Minute, func(c *gin.Context) (string, bool) {
			return pkg.KeyByUserID(c, cfg)
		}, logger), handlerGroup.UserHandler.UploadAvatar)
		// 通用图片上传（收藏夹封面等）
		userPrivateRouter.POST("upload/image", handlerGroup.UserHandler.UploadImage)
	}

	{
		userAdminRouter.GET("list", handlerGroup.UserHandler.UserList)
		userAdminRouter.PUT("freeze", handlerGroup.UserHandler.UserFreeze)
		userAdminRouter.PUT("unfreeze", handlerGroup.UserHandler.UserUnfreeze)
		userAdminRouter.GET("loginList", handlerGroup.UserHandler.UserLoginList)
	}

	{
		userPublicRouter.POST("forgotPassword", middleware.Limit(redisRepo, "forgotPassword", 5, time.Minute, pkg.KeyByIP, logger), handlerGroup.UserHandler.ForgotPassword)
		userPublicRouter.GET("home", handlerGroup.UserHandler.UserHomePage)
	}
}
