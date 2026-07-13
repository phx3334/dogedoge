package logic

import (
	"fake_tiktok/internal/breaker"
	"fake_tiktok/internal/config"
	"fake_tiktok/internal/pkg/storage"
	"fake_tiktok/internal/repository/interfaces"

	"github.com/mojocn/base64Captcha"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// LogicDeps 业务层依赖聚合
//
// 集中管理所有 repository / 缓存 / 外部 client，业务层通过 deps 访问这些资源。
// 保持扁平结构便于一次性注入；新增依赖时仅需在结构体内追加字段并在 InitLogicGroup 中使用。
type LogicDeps struct {
	Config                  *config.Config
	AccountRepo             interfaces.AccountRepository
	LoginRepo               interfaces.LoginRepository
	JWTRepo                 interfaces.JWTRepository
	VideoRepo               interfaces.VideoRepository
	PaginateRepo            interfaces.PaginateRepository
	CaptchaStore            base64Captcha.Store
	Logger                  *zap.Logger
	ClientRepo              interfaces.ClientRepository
	RankingRepo             interfaces.RankingRepository
	VideoCacheRepo          interfaces.VideoCacheRepository
	BackfillRepo            interfaces.BackfillRepository
	UserCacheRepo           interfaces.UserCacheRepository
	DanmakuRepo             interfaces.DanmakuRepository
	DanmakuPubSub           interfaces.DanmakuPubSub
	PlayCountPublisher      interfaces.PlayCountPublisher
	InteractionCacheRepo    interfaces.InteractionCacheRepository
	InteractionRepo         interfaces.InteractionRepository
	DanmakuCacheRepo        interfaces.DanmakuCacheRepository
	FavoriteFolderRepo      interfaces.FavoriteFolderRepository
	UserPlayCountPublisher  interfaces.UserPlayCountPublisher
	VideoLikeCountPublisher interfaces.VideoLikeCountPublisher
	VideoSearchRepo         interfaces.VideoSearchRepository
	FavoriteRepo            interfaces.FavoriteRepository

	// Breakers 全局熔断器组（Task 6 引入）
	//
	// 用途：在 Redis / MySQL 持续不可用时防止请求雪崩穿透到下游。
	//   - Breakers.MySQL 包装 BackfillRepo（MySQL 回源）
	//   - Breakers.Redis 包装缓存读 / 写路径
	//
	// 注意：熔断器状态由内部 mutex 保护，外部**不应**直接调用 State()，
	// 除非用于 metrics 导出 / 健康检查。
	Breakers *breaker.Group

	// Phase 2/3/4 新增依赖
	VideoDraftRepo         interfaces.VideoDraftRepository
	TranscodePublisher     interfaces.TranscodePublisher
	VideoCommentRepo       interfaces.VideoCommentRepository
	ArticleCommentRepo     interfaces.ArticleCommentRepository
	DynamicCommentRepo     interfaces.DynamicCommentRepository
	VideoCommentLikeRepo   interfaces.CommentLikeRepository
	ArticleCommentLikeRepo interfaces.CommentLikeRepository
	DynamicCommentLikeRepo interfaces.CommentLikeRepository
	NotificationRepo       interfaces.NotificationRepository
	ArticleRepo            interfaces.ArticleRepository
	Storage                storage.Storage // for VideoDraftLogic

	// P1-P3 个人中心相关依赖
	VideoCoinRepo          interfaces.VideoCoinRepository
	CoinLedgerRepo         interfaces.CoinLedgerRepository
	VideoViewHistoryRepo   interfaces.VideoViewHistoryRepository
	ArticleViewHistoryRepo interfaces.ArticleViewHistoryRepository
	UserSearchHistoryRepo  interfaces.UserSearchHistoryRepository
	UserDynamicRepo        interfaces.UserDynamicRepository
	UserDynamicLikeRepo    interfaces.UserDynamicLikeRepository
	DailyTaskRepo          interfaces.UserDailyTaskRepository
	DailyTaskLogic         *DailyTaskLogic

	// 用户间私信
	MessageRepo interfaces.MessageRepository

	// RedisClient 用于在业务层直接读写 Redis。
	// 当前用途：邮箱验证码的存储与校验（之前用 cookie session，
	// 跨域场景下 cookie 不会被浏览器自动回传，导致"email not match"问题）。
	RedisClient *redis.Client
}

type LogicGroup struct {
	Config           *config.Config
	GaodeLogic       *GaodeLogic
	UserLogic        *UserLogic
	BaseLogic        *BaseLogic
	VideoLogic       *VideoLogic
	DanmakuLogic     *DanmakuLogic
	SearchLogic      *SearchLogic
	InteractionLogic *InteractionLogic
	VideoDraftLogic  *VideoDraftLogic
	CommentLogic     *CommentLogic
	ArticleLogic     *ArticleLogic

	// P1-P3 个人中心相关 Logic
	CoinLogic           *CoinLogic
	FavoriteFolderLogic *FavoriteFolderLogic
	FollowLogic         *FollowLogic
	NotificationLogic   *NotificationLogic
	HistoryLogic        *HistoryLogic
	DynamicLogic        *DynamicLogic
	DailyTaskLogic      *DailyTaskLogic
	UserHomeLogic       *UserHomeLogic

	// 用户间私信
	MessageLogic *MessageLogic

	// AI 聊天
	AILogic *AILogic
}

var LogicGroupApp = &LogicGroup{}

// InitLogicGroup 构造所有 Logic 业务类
//
// 业务类按职责拆分（用户 / 视频 / 弹幕 / 高德 / 基础工具），所有业务类共享同一份 deps。
func InitLogicGroup(deps *LogicDeps) *LogicGroup {
	// DailyTaskLogic 需先于 CommentLogic 创建，便于评论成功后发放经验奖励
	dailyTaskLogic := NewDailyTaskLogic(deps)
	deps.DailyTaskLogic = dailyTaskLogic

	lg := &LogicGroup{
		Config:           deps.Config,
		GaodeLogic:       NewGaodeLogic(deps.Config.Gaode.APIKey, deps.Logger),
		UserLogic:        NewUserLogic(deps),
		BaseLogic:        NewBaseLogic(deps),
		VideoLogic:       NewVideoLogic(deps),
		DanmakuLogic:     NewDanmakuLogic(deps),
		SearchLogic:      NewSearchLogic(deps),
		InteractionLogic: NewInteractionLogic(deps),
		VideoDraftLogic:  NewVideoDraftLogic(deps, deps.Storage),
		ArticleLogic:     NewArticleLogic(deps),

		// P1-P3 个人中心相关 Logic
		CoinLogic:           NewCoinLogic(deps),
		FavoriteFolderLogic: NewFavoriteFolderLogic(deps),
		FollowLogic:         NewFollowLogic(deps),
		NotificationLogic:   NewNotificationLogic(deps),
		HistoryLogic:        NewHistoryLogic(deps),
		DynamicLogic:        NewDynamicLogic(deps),
		DailyTaskLogic:      dailyTaskLogic,
		UserHomeLogic:       NewUserHomeLogic(deps),

		CommentLogic: NewCommentLogic(deps),

		MessageLogic: NewMessageLogic(deps),

		AILogic: NewAILogic(&deps.Config.AI, deps.Logger),
	}
	LogicGroupApp = lg
	return lg
}
