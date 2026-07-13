package interfaces

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// ClientRepository 定义 Redis 客户端的基础操作接口。
// 包含键值操作、Hash 操作、Pipeline、限流和连接检测等功能。
type ClientRepository interface {
	// ---- 基础键值操作 ----
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	Del(ctx context.Context, keys ...string) error
	Close() error

	// BuildKey 使用 "module:key" 格式构建带前缀的 Redis 键
	BuildKey(module, key string) string

	// BuildPublishedZSetKey 构建已发布视频 ZSet 的完整 Redis 键。
	// zone 为空时返回全局 ZSet key，非空时返回分区 ZSet key。
	// 封装了 key 命名规则，避免 logic 层直接依赖 redis 包的常量。
	BuildPublishedZSetKey(zone string) string

	// ---- 限流 ----
	SlidingWindowLimit(ctx context.Context, key string, maxRequests int64, window time.Duration) (bool, error)

	// ---- Pipeline ----
	// Pipeline 返回 go-redis Pipeliner，用于批量提交命令减少网络往返
	Pipeline() redis.Pipeliner

	// ---- 连接检测 ----
	// Ping 用于判断 Redis 是否可用，是降级逻辑的判断依据
	Ping(ctx context.Context) error

	// ---- 超时控制 ----
	// WithTimeout 为无 deadline 的 context 添加默认超时
	WithTimeout(ctx context.Context) (context.Context, context.CancelFunc)

	// ---- Hash 操作 ----
	HSet(ctx context.Context, key string, values ...interface{}) error
	HGet(ctx context.Context, key, field string) (string, error)
	HGetAll(ctx context.Context, key string) (map[string]string, error)
	HMGet(ctx context.Context, key string, fields ...string) ([]interface{}, error)
	HDel(ctx context.Context, key string, fields ...string) error

	// Expire 为指定键设置过期时间
	Expire(ctx context.Context, key string, ttl time.Duration) error
}