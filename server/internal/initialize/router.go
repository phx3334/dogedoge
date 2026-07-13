// Package initialize 内的 router.go：构建 HTTP 路由 + 注入依赖。
//
// 关键变更（Task 3）：
//   - 不再使用 app.RabbitMQ.Channel
//   - 直接传 app.RabbitMQConn 给 NewRepos
//   - publish buffer 已在 NewRepos 内部构造并启动；这里无需重复
package initialize

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"fake_tiktok/internal/handler"
	"fake_tiktok/internal/handler/ws"
	"fake_tiktok/internal/logic"
	"fake_tiktok/internal/middleware"
	"fake_tiktok/internal/pkg/storage"
	"fake_tiktok/internal/repository"
	"fake_tiktok/internal/repository/interfaces"
	redis_repo "fake_tiktok/internal/repository/redis"
	"fake_tiktok/internal/routers"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

// InitRouter 构造 gin.Engine 并注册所有路由。
//
// 依赖注入顺序：
//  1. RedisClient（用于 session / captcha store）
//  2. Repos（聚合所有 repository，**包含** PlayCountPublisher）
//  3. Hub（WebSocket Hub，单例）
//  4. CaptchaStore（Redis 实现，Task 4 改造）
//  5. LogicGroup / HandlerGroup（业务层）
//  6. RouterGroup（路由注册）
func InitRouter(app *App) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	Router := gin.New()

	Router.Use(middleware.GinLogger(app.Logger), middleware.GinRecovery(app.Logger))

	// 全局限流（保底）：每 IP 每秒最多 30 次请求
	// 局部路由上的限流中间件优先级更高，此处仅作兜底
	globalRateLimiter := middleware.Limit(
		redis_repo.NewRedisClient(app.Redis.Client, app.Redis.KeyPrefix),
		"global",
		21,
		time.Second,
		func(c *gin.Context) (string, bool) {
			return c.ClientIP(), true
		},
		app.Logger,
	)
	Router.Use(globalRateLimiter)

	store := cookie.NewStore([]byte(app.Config.Server.SessionsSecret))
	Router.Use(sessions.Sessions("mysession", store))

	Router.StaticFS("/uploads", http.Dir(app.Config.Upload.Path))

	// 健康检查端点：供容器编排系统（Docker / K8s）探测服务是否真正可用
	// 不走任何中间件（JWT / 限流），确保即使业务层出问题也能响应
	//
	// 检查内容：
	//   - MySQL：通过 sqlDB.Ping() 检测连接是否存活
	//   - Redis：通过 redis.Ping() 检测连接是否存活
	//   - 任一依赖不可用返回 503，全部正常返回 200
	//
	// docker-compose 中应将 healthcheck 改为：
	//   test: ["CMD-SHELL", "wget -qO- http://localhost:8080/health || exit 1"]
	Router.GET("/health", func(c *gin.Context) {
		// 检查 MySQL 连通性
		sqlDB, err := app.DB.DB()
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy", "mysql": err.Error()})
			return
		}
		if err := sqlDB.Ping(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy", "mysql": err.Error()})
			return
		}

		// 检查 Redis 连通性
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := app.Redis.Client.Ping(ctx).Err(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy", "redis": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	redisClient := redis_repo.NewRedisClient(app.Redis.Client, app.Redis.KeyPrefix)
	// Task 3 改造：把 *rabbitmq.Connection 抽象传给 NewRepos，
	// 内部会构造并启动 publish buffer
	repos := repository.NewRepos(app.DB, app.ESClient, redisClient, app.RabbitMQConn, app.Logger)

	hub := ws.NewDanmakuHub()

	// 设置 WebSocket Origin 白名单，防止跨站 WebSocket 劫持（CSWSH）攻击
	// 未配置时允许所有来源（向后兼容），生产环境应在配置文件中设置 allowed_origins
	ws.SetAllowedOrigins(app.Config.Server.AllowedOrigins)

	// captcha 存储后端：使用 RedisCaptchaStore 替换 base64Captcha.DefaultMemStore。
	// 改造后 captcha 数据写入 Redis（TTL 5min），支持 API 进程重启后短期内仍可校验，
	// 同时通过 GETDEL 原子消费防止重放。详见 internal/repository/redis/captcha_store.go。
	captchaStore := redis_repo.NewRedisCaptchaStore(redisClient)

	// Task 6 改造：构造全局熔断器组（Redis / MySQL 独立计数）
	// 在 NewRepos 之后、InitLogicGroup 之前构造，业务层（LogicDeps）按依赖类型选择对应熔断器
	breakers := NewBreakerGroup(app.Config, app.Logger)

	// Phase 5: 构造 Storage 实例供 VideoDraftLogic 使用
	// 配置错误（如 driver 不合法）会让进程在启动时即失败，符合 fail-fast 原则
	storageInstance, err := storage.NewStorage(&app.Config.Storage, &app.Config.Qiniu)
	if err != nil {
		panic(fmt.Sprintf("初始化存储抽象失败: %v", err))
	}

	logicGroup := logic.InitLogicGroup(&logic.LogicDeps{
		Config:                  app.Config,
		AccountRepo:             repos.AccountRepo,
		LoginRepo:               repos.LoginRepo,
		JWTRepo:                 repos.JWTRepo,
		VideoRepo:               repos.VideoRepo,
		PaginateRepo:            repos.PaginateRepo,
		CaptchaStore:            captchaStore,
		Logger:                  app.Logger,
		ClientRepo:              repos.ClientRepo,
		RankingRepo:             repos.RankingRepo,
		VideoCacheRepo:          repos.VideoCacheRepo,
		BackfillRepo:            repos.BackfillRepo,
		UserCacheRepo:           repos.UserCacheRepo,
		DanmakuRepo:             repos.DanmakuRepo,
		DanmakuPubSub:           repos.DanmakuPubSubRepo,
		DanmakuCacheRepo:        repos.DanmakuCacheRepo,
		PlayCountPublisher:      repos.PlayCountPublisher,
		InteractionCacheRepo:    repos.InteractionCacheRepo,
		InteractionRepo:         repos.InteractionRepo,
		FavoriteFolderRepo:      repos.FavoriteFolderRepo,
		UserPlayCountPublisher:  repos.UserPlayCountPublisher,
		VideoLikeCountPublisher: repos.VideoLikeCountPublisher,
		VideoSearchRepo:         repos.VideoSearchRepo,
		FavoriteRepo:            repos.FavoriteRepo,
		Breakers:                breakers,

		// Phase 2/3/4 新增依赖
		// VideoRepo 字段静态类型为 interfaces.VideoRepository（不含 CreateDraft），
		// 但具体类型 *mysql.VideoRepo 已在 Phase 2 扩展实现 VideoDraftRepository，
		// 通过类型断言取出该接口视图供 VideoDraftLogic 使用
		VideoDraftRepo:         repos.VideoRepo.(interfaces.VideoDraftRepository),
		TranscodePublisher:     repos.TranscodePublisher,
		VideoCommentRepo:       repos.VideoCommentRepo,
		ArticleCommentRepo:     repos.ArticleCommentRepo,
		DynamicCommentRepo:     repos.DynamicCommentRepo,
		VideoCommentLikeRepo:   repos.VideoCommentLikeRepo,
		ArticleCommentLikeRepo: repos.ArticleCommentLikeRepo,
		DynamicCommentLikeRepo: repos.DynamicCommentLikeRepo,
		NotificationRepo:       repos.NotificationRepo,
		ArticleRepo:            repos.ArticleRepo,
		Storage:                storageInstance,

		// P1-P3 个人中心相关依赖
		VideoCoinRepo:          repos.VideoCoinRepo,
		CoinLedgerRepo:         repos.CoinLedgerRepo,
		VideoViewHistoryRepo:   repos.VideoViewHistoryRepo,
		ArticleViewHistoryRepo: repos.ArticleViewHistoryRepo,
		UserSearchHistoryRepo:  repos.UserSearchHistoryRepo,
		UserDynamicRepo:        repos.UserDynamicRepo,
		UserDynamicLikeRepo:    repos.UserDynamicLikeRepo,
		DailyTaskRepo:          repos.DailyTaskRepo,

		// 用户间私信
		MessageRepo: repos.MessageRepo,

		// 直接持有 go-redis 客户端，供 BaseLogic.SendEmailCode / VerifyEmailCode 使用。
		// 之前用 cookie session 存验证码，跨域场景下 cookie 不会被回传导致 "email not match"。
		RedisClient: app.Redis.Client,
	})

	handlerGroup := handler.NewHandlerGroup(&handler.HandlerDeps{
		Logic:  logicGroup,
		Logger: app.Logger,
	})

	routerGroup := routers.RouterGroupApp

	publicGroup := Router.Group(app.Config.Server.RouterPrefix)

	privateGroup := Router.Group(app.Config.Server.RouterPrefix)
	privateGroup.Use(middleware.JWTAuth(app.Config, repos.JWTRepo, repos.AccountRepo))

	adminGroup := Router.Group(app.Config.Server.RouterPrefix)
	adminGroup.Use(middleware.JWTAuth(app.Config, repos.JWTRepo, repos.AccountRepo), middleware.AdminAuth(app.Config))

	routerGroup.InitBaseRouter(publicGroup, handlerGroup, redisClient, app.Logger)
	routerGroup.InitUserRouter(privateGroup, publicGroup, adminGroup, handlerGroup, repos.LoginRepo, redisClient, app.Config, app.Logger, logicGroup.GaodeLogic)
	routerGroup.InitVideoRouter(publicGroup, privateGroup, handlerGroup, hub, app.Config, repos.AccountRepo, repos.DanmakuRepo, repos.DanmakuCacheRepo, repos.DanmakuPubSubRepo, repos.VideoRepo, repos.VideoCacheRepo, breakers, app.Logger)
	routerGroup.InitSearchRouter(publicGroup, handlerGroup)
	routerGroup.InitCommentRouter(publicGroup, privateGroup, handlerGroup)
	routerGroup.InitArticleRouter(publicGroup, privateGroup, handlerGroup)
	routerGroup.InitInteractionRouter(publicGroup, privateGroup, handlerGroup)

	// P1-P3 个人中心相关路由
	routerGroup.InitCoinRouter(privateGroup, handlerGroup)
	routerGroup.InitFavoriteFolderRouter(privateGroup, handlerGroup)
	routerGroup.InitFollowRouter(publicGroup, handlerGroup)
	routerGroup.InitNotificationRouter(privateGroup, handlerGroup)
	routerGroup.InitHistoryRouter(privateGroup, handlerGroup)
	routerGroup.InitDynamicRouter(publicGroup, privateGroup, handlerGroup)
	routerGroup.InitDailyTaskRouter(privateGroup, handlerGroup)
	routerGroup.InitUserHomeRouter(publicGroup, privateGroup, handlerGroup)
	routerGroup.InitAIRouter(publicGroup, privateGroup, handlerGroup)
	routerGroup.InitMessageRouter(privateGroup, handlerGroup)

	return Router
}
