// Package repository 内的 repos.go：聚合所有 repository 实例，供 logic 层使用。
//
// 关键变更（Task 3）：
//   - NewRepos 接收的不再是 *amqp091.Channel 而是 *rabbitmq.Connection
//   - 内部用 NewPublishBuffer 包装 Connection 后传给 PlayCountPublisherRepo
//   - PublishBuffer.Start(context.Background()) 在构造后启动 drainLoop
package repository

import (
	"context"
	es_repo "fake_tiktok/internal/repository/es"
	"fake_tiktok/internal/repository/interfaces"
	"fake_tiktok/internal/repository/mysql"
	"fake_tiktok/internal/repository/rabbitmq"
	redis_repo "fake_tiktok/internal/repository/redis"

	"github.com/elastic/go-elasticsearch/v8"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Repos 是所有 repository 的容器，被 logic 层统一引用。
//
// 字段对应 interfaces 下定义的接口；构造时填入具体实现。
// 整个进程只有一份（单例模式），通过 initialize.InitRouter 注入到 handler。
type Repos struct {
	AccountRepo             interfaces.AccountRepository
	LoginRepo               interfaces.LoginRepository
	JWTRepo                 interfaces.JWTRepository
	VideoRepo               interfaces.VideoRepository
	SearchIndexRepo         interfaces.SearchIndexRepository
	ClientRepo              interfaces.ClientRepository
	PaginateRepo            interfaces.PaginateRepository
	RankingRepo             interfaces.RankingRepository
	VideoCacheRepo          interfaces.VideoCacheRepository
	BackfillRepo            interfaces.BackfillRepository
	UserCacheRepo           interfaces.UserCacheRepository
	DanmakuRepo             interfaces.DanmakuRepository
	DanmakuPubSubRepo       interfaces.DanmakuPubSub
	PlayCountPublisher      interfaces.PlayCountPublisher
	UserPlayCountPublisher  interfaces.UserPlayCountPublisher
	VideoLikeCountPublisher interfaces.VideoLikeCountPublisher
	InteractionCacheRepo    interfaces.InteractionCacheRepository
	InteractionRepo         interfaces.InteractionRepository
	DanmakuCacheRepo        interfaces.DanmakuCacheRepository
	FavoriteFolderRepo      interfaces.FavoriteFolderRepository
	VideoSearchRepo         interfaces.VideoSearchRepository

	// P0 修复新增：视频收藏独立 repo
	FavoriteRepo interfaces.FavoriteRepository

	// Phase 2/3/4 新增 repo
	VideoCommentRepo       interfaces.VideoCommentRepository
	ArticleCommentRepo     interfaces.ArticleCommentRepository
	DynamicCommentRepo     interfaces.DynamicCommentRepository
	VideoCommentLikeRepo   interfaces.CommentLikeRepository
	ArticleCommentLikeRepo interfaces.CommentLikeRepository
	DynamicCommentLikeRepo interfaces.CommentLikeRepository
	NotificationRepo       interfaces.NotificationRepository
	ArticleRepo            interfaces.ArticleRepository
	TranscodePublisher     interfaces.TranscodePublisher

	// P1-P3 个人中心相关 repo
	VideoCoinRepo          interfaces.VideoCoinRepository
	CoinLedgerRepo         interfaces.CoinLedgerRepository
	VideoViewHistoryRepo   interfaces.VideoViewHistoryRepository
	ArticleViewHistoryRepo interfaces.ArticleViewHistoryRepository
	UserSearchHistoryRepo  interfaces.UserSearchHistoryRepository
	UserDynamicRepo        interfaces.UserDynamicRepository
	UserDynamicLikeRepo    interfaces.UserDynamicLikeRepository
	DailyTaskRepo          interfaces.UserDailyTaskRepository

	// 用户间私信
	MessageRepo interfaces.MessageRepository
}

// NewRepos 构造 Repos 容器，注入所有 repository 实例。
//
// 参数：
//   - db：gorm.DB（业务强依赖）
//   - esClient：ES typed client
//   - redisClient：Redis 客户端封装
//   - rabbitConn：RabbitMQ 弹性连接抽象（Task 3 改造后替代原来的 *amqp091.Channel）
//   - logger：zap logger
//
// Task 3 变更点：
//   - 内部构造 publishBuffer := rabbitmq.NewPublishBuffer(rabbitConn, logger)
//   - 立即 Start(context.Background()) 启动后台 drainLoop
//   - 之后 PlayCountPublisher 用 publishBuffer 包装
//
// 为什么用 context.Background()：
//   - publish buffer 应当跟随进程生命周期；它没有"业务请求"概念
//   - 在 Connection.Close() 触发后 drainLoop 会自动退出（监听 conn.runCtx.Done()）
func NewRepos(db *gorm.DB, esClient *elasticsearch.TypedClient, redisClient *redis_repo.RedisClient, rabbitConn *rabbitmq.Connection, logger *zap.Logger) *Repos {
	videoCacheRepo := redis_repo.NewVideoCacheRepo(redisClient, logger)
	interactionCacheRepo := redis_repo.NewInteractionCacheRepo(redisClient, logger)
	accountRepo := mysql.NewAccountRepo(db)
	userCacheRepo := redis_repo.NewUserCacheRepo(redisClient, logger)

	// 构造 MQ 缓冲发布器并启动后台 drainLoop
	// 注意：Start 的 ctx 用 Background 是有意为之——buffer 跟随进程生命周期，
	// 不是某个具体业务请求；关闭走 Connection.Close 路径
	publishBuffer := rabbitmq.NewPublishBuffer(rabbitConn, logger)
	publishBuffer.Start(context.Background())

	return &Repos{
		AccountRepo:     accountRepo,
		LoginRepo:       mysql.NewLoginRepo(db),
		JWTRepo:         redis_repo.NewJWTRepo(redisClient),
		VideoRepo:       mysql.NewVideoRepo(db),
		SearchIndexRepo: es_repo.NewSearchIndexRepo(esClient),
		ClientRepo:      redis_repo.NewRedisClient(redisClient.Client, redisClient.KeyPrefix),
		PaginateRepo:    mysql.NewPaginateRepo(db),
		RankingRepo:     redisClient,
		VideoCacheRepo:  videoCacheRepo,
		BackfillRepo: mysql.NewBackfillRepo(accountRepo, videoCacheRepo, userCacheRepo, mysql.NewInteractionRepo(db),
			interactionCacheRepo, mysql.NewDanmakuRepo(db),
			redis_repo.NewDanmakuCacheRepo(redisClient, logger), logger),
		UserCacheRepo:           userCacheRepo,
		DanmakuRepo:             mysql.NewDanmakuRepo(db),
		DanmakuPubSubRepo:       redis_repo.NewDanmakuPubSubRepo(redisClient, logger),
		PlayCountPublisher:      rabbitmq.NewPlayCountPublisherRepo(publishBuffer),
		UserPlayCountPublisher:  rabbitmq.NewUserPlayCountPublisherRepo(publishBuffer),
		VideoLikeCountPublisher: rabbitmq.NewVideoLikeCountPublisherRepo(publishBuffer),
		InteractionCacheRepo:    interactionCacheRepo,
		InteractionRepo:         mysql.NewInteractionRepo(db),
		DanmakuCacheRepo:        redis_repo.NewDanmakuCacheRepo(redisClient, logger),
		FavoriteFolderRepo:      mysql.NewFavoriteFolderRepo(db),
		VideoSearchRepo:         es_repo.NewVideoSearchRepo(esClient, logger),
		FavoriteRepo:            mysql.NewFavoriteRepo(db),

		// Phase 2/3/4 新增依赖
		VideoCommentRepo:       mysql.NewVideoCommentRepo(db),
		ArticleCommentRepo:     mysql.NewArticleCommentRepo(db),
		DynamicCommentRepo:     mysql.NewDynamicCommentRepo(db),
		VideoCommentLikeRepo:   mysql.NewVideoCommentLikeRepo(db),
		ArticleCommentLikeRepo: mysql.NewArticleCommentLikeRepo(db),
		DynamicCommentLikeRepo: mysql.NewDynamicCommentLikeRepo(db),
		NotificationRepo:       mysql.NewNotificationRepo(db),
		ArticleRepo:            mysql.NewArticleRepo(db),
		TranscodePublisher:     rabbitmq.NewTranscodePublisherRepo(publishBuffer),

		// P1-P3 个人中心相关依赖
		VideoCoinRepo:          mysql.NewVideoCoinRepo(db),
		CoinLedgerRepo:         mysql.NewCoinLedgerRepo(db),
		VideoViewHistoryRepo:   mysql.NewVideoViewHistoryRepo(db),
		ArticleViewHistoryRepo: mysql.NewArticleViewHistoryRepo(db),
		UserSearchHistoryRepo:  mysql.NewUserSearchHistoryRepo(db),
		UserDynamicRepo:        mysql.NewUserDynamicRepo(db),
		UserDynamicLikeRepo:    mysql.NewUserDynamicLikeRepo(db),
		DailyTaskRepo:          mysql.NewUserDailyTaskRepo(db),

		// 用户间私信
		MessageRepo: mysql.NewMessageRepo(db),
	}
}
