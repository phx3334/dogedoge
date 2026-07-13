package interfaces

import (
	"context"
	"fake_tiktok/internal/dto/response"

	"github.com/redis/go-redis/v9"
)

type DanmakuCacheRepository interface {
	GetDanmakuCache(ctx context.Context, videoID uint64) ([]*response.DanmakuItem, error)
	Create(ctx context.Context, videoID uint64, item *response.DanmakuItem) error
	WriteDanmakuCache(ctx context.Context, videoID uint64, members []redis.Z)
	// RefreshDanmakuCacheExpiry 刷新弹幕缓存过期时间（热门 7d / 非热门 10min）
	RefreshDanmakuCacheExpiry(ctx context.Context, videoID uint64, isHot bool)
	// DeleteDanmakuCache 删除弹幕缓存 key，强制下次读取时从 MySQL 全量回填
	// 用于 SendDanmaku 中 Redis 写入失败时保证缓存一致性
	DeleteDanmakuCache(ctx context.Context, videoID uint64) error
}
