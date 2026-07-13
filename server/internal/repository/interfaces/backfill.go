package interfaces

import (
	"context"

	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/dto/cache"
	"fake_tiktok/internal/dto/response"
)

// BackfillRepository 统一管理所有「缓存未命中 → 查 MySQL → 回填 Redis」的降级回源逻辑

// AuthorCardInfo 作者简略信息（用户名 + 头像 + 经验），用于评论/通知等场景
type AuthorCardInfo struct {
	Username   string
	AvatarURL  string
	Experience uint64
}

type BackfillRepository interface {
	// BackfillVideoCache 从 MySQL 回源视频信息并回填 Redis 缓存
	BackfillVideoCache(ctx context.Context, dbVideos []database.Video, missedIDs []uint, cacheMap map[uint]*cache.VideoCacheData)
	// BackfillInteractionCache 从 MySQL 回源互动状态（点赞/收藏/投币/关注）并回填 Redis 缓存
	BackfillInteractionCache(ctx context.Context, userID string, videoID uint, authorID string) *InteractionStatus
	// BackfillUserCache 从 MySQL 回源用户信息（含粉丝数/关注数）并回填 Redis 缓存
	// 返回 (data, err)，err 用于上层记录日志，不阻塞业务
	BackfillUserCache(ctx context.Context, userID string) (*cache.UserCacheData, error)
	// LookupAuthorNames 批量查询作者名称（供 mysqlFallback 等场景复用）
	LookupAuthorNames(ctx context.Context, authorIDs []string) map[string]string
	// LookupAuthorCards 批量查询作者的用户名+头像 URL（供评论列表等场景复用）
	// 返回 map[userID]AuthorInfo，缺失的 ID 在 map 中不存在
	LookupAuthorCards(ctx context.Context, authorIDs []string) map[string]AuthorCardInfo
	// BackfillDanmakuCache 从 MySQL 回源弹幕并并回填 Redis 缓存
	BackfillDanmakuCache(ctx context.Context, videoID uint64) ([]*response.DanmakuItem, error)

	// FallbackCheckLike Redis 不可用时的降级点赞判重路径
	// 通过 MySQL VideoLike 表判断用户是否已点赞，如果未点赞则写入点赞记录
	// 返回值：
	//   - created=true：真正新建了记录（首次点赞），调用方应发布 MQ 增量
	//   - created=false：记录已存在（幂等），调用方不应发布 MQ 增量
	// 熔断器和信号量由 logic 层负责包装，此方法只做纯 MySQL 操作
	FallbackCheckLike(ctx context.Context, userID string, videoID uint) (created bool, err error)
}
