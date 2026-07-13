package interfaces

import (
	"context"

	"github.com/redis/go-redis/v9"
)

// RankingRepository 定义 Redis ZSet（有序集合）排名操作接口。
// 用于实现热度榜、游标分页等排名相关功能。
type RankingRepository interface {
	// ---- 成员操作 ----
	ZAdd(ctx context.Context, key string, members ...redis.Z) error
	ZIncrBy(ctx context.Context, key string, members string, score float64) error

	// ---- 范围查询 ----
	ZRevRangeWithScores(ctx context.Context, key string, start, stop int64) ([]redis.Z, error)
	ZRangeWithScores(ctx context.Context, key string, start, stop int64) ([]redis.Z, error)
	ZRevRange(ctx context.Context, key string, start, stop int64) ([]string, error)

	// ---- 分数查询 ----
	ZRevRangeByScoreWithScores(ctx context.Context, key string, opt *redis.ZRangeBy) ([]redis.Z, error)
	ZScore(ctx context.Context, key, member string) (float64, error)

	// ---- 删除操作 ----
	ZRemRangeByRank(ctx context.Context, key string, start, stop int64) error
	ZRemRangeByScore(ctx context.Context, key, min, max string) error

	// ---- 集合操作 ----
	ZCard(ctx context.Context, key string) (int64, error)
	ZUnionStore(ctx context.Context, dst string, keys []string, aggregate string) error

	// ---- 游标分页 ----
	// ZSetCursorPaginate 实现基于 ZSet 的游标分页（高分优先），正确处理同分元素。
	// cursorScore=0 且 cursorMember="" 表示第一页；后续页通过 ZREVRANK 精确定位游标成员位置。
	// 返回值: (当前页成员, 下一页游标分数, 下一页游标成员ID, 错误)
	ZSetCursorPaginate(ctx context.Context, key string, cursorScore float64, cursorMember string, limit int64) ([]redis.Z, float64, string, error)

	// ---- 批量操作 ----
	// ZSetBatchAdd 通过 Pipeline 批量添加成员，用于定时任务全量重建
	ZSetBatchAdd(ctx context.Context, key string, members []redis.Z) error
	IsHotVideo(ctx context.Context, videoID uint) bool
}
