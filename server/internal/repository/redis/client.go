package redis

import (
	"context"
	"errors"
	"fake_tiktok/internal/repository/interfaces"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrRedisUnavailable 表示 Redis 客户端未初始化或不可用。
// 与 redis.Nil（键不存在）区分：调用方可据此判断是"Redis 服务不可用"还是"键不存在/过期"，
// 避免将"不可用"误判为"键过期"导致业务逻辑错误（如放行已注销 Token）。
var ErrRedisUnavailable = errors.New("redis client is not available")

// 编译期接口校验：确保 RedisClient 实现了 ClientRepository 和 RankingRepository
var _ interfaces.ClientRepository = (*RedisClient)(nil)
var _ interfaces.RankingRepository = (*RedisClient)(nil)

// DefaultRedisTimeout 所有 Redis 操作的默认超时时间。
// 若调用方传入的 context 已携带 deadline，则优先使用已有的 deadline。
const DefaultRedisTimeout = 3 * time.Second

// RedisClient 封装 go-redis 客户端，提供带键前缀和超时控制的 Redis 操作。
// 通过内嵌 *redis.Client 继承原生方法，通过 KeyPrefix 实现键名前缀隔离。
type RedisClient struct {
	*redis.Client
	KeyPrefix string // 键名前缀，例如 "Dogedoge:v1"
}

// NewRedisClient 创建一个带键前缀的 Redis 客户端实例。
func NewRedisClient(client *redis.Client, keyPrefix string) *RedisClient {
	return &RedisClient{Client: client, KeyPrefix: keyPrefix}
}

// BuildKey 使用 "module:key" 格式构建带前缀的完整 Redis 键。
// 格式: {KeyPrefix}:{module}:{key}
// 示例: BuildKey("blacklist", "token123") → "Dogedoge:v1:blacklist:token123"
func (r *RedisClient) BuildKey(module, key string) string {
	if r.KeyPrefix == "" {
		return module + ":" + key
	}
	return r.KeyPrefix + ":" + module + ":" + key
}

// BuildPublishedZSetKey 构建已发布视频 ZSet 的完整 Redis 键。
// zone 为空时返回全局 ZSet key（module="video:published", key="global"），
// zone 非空时返回分区 ZSet key（module="video:published", key=zone）。
// 封装了 key 命名规则，避免上层直接依赖 redis 包的常量。
func (r *RedisClient) BuildPublishedZSetKey(zone string) string {
	if zone == "" {
		zone = "global"
	}
	return r.BuildKey(PublishedVideoZSetKey, zone)
}

// WithTimeout 为 context 添加超时控制。
// 若 ctx 已有 deadline，则直接返回原 ctx 和空 cancel 函数（透传模式），
// 避免覆盖调用方设置的更短超时时间。
// 若 ctx 无 deadline，则创建带 DefaultRedisTimeout 超时的新 context。
func (r *RedisClient) WithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, DefaultRedisTimeout)
}

// Set 写入键值对，支持设置过期时间。
func (r *RedisClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	if r.Client == nil {
		return ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	return r.Client.Set(ctx, key, value, expiration).Err()
}

// Get 读取键对应的值。键不存在时返回 redis.Nil 错误。
func (r *RedisClient) Get(ctx context.Context, key string) (string, error) {
	if r.Client == nil {
		return "", ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	return r.Client.Get(ctx, key).Result()
}

// Del 删除一个或多个键。
func (r *RedisClient) Del(ctx context.Context, keys ...string) error {
	if r.Client == nil {
		return ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	return r.Client.Del(ctx, keys...).Err()
}

// Pipeline 返回一个 Redis Pipeline 实例，用于批量提交命令减少网络往返。
// Client 为 nil 时返回 ErrRedisUnavailable，避免调用方在 nil Pipeline 上操作导致 panic。
func (r *RedisClient) Pipeline() redis.Pipeliner {
	if r.Client == nil {
		return nil
	}
	return r.Client.Pipeline()
}

// Ping 检测 Redis 连接是否可用。
// 注意：Ping 成功不代表业务键存在，也不代表熔断器已关闭。
// 优先使用熔断器模式（Breaker.Execute）来判断 Redis 可用性，而非 Ping。
func (r *RedisClient) Ping(ctx context.Context) error {
	if r.Client == nil {
		return ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	return r.Client.Ping(ctx).Err()
}

// HSet 设置 Hash 中一个或多个 field-value 对。
// 用法: HSet(ctx, key, "field1", val1, "field2", val2, ...)
func (r *RedisClient) HSet(ctx context.Context, key string, values ...interface{}) error {
	if r.Client == nil {
		return ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	return r.Client.HSet(ctx, key, values...).Err()
}

// HGet 获取 Hash 中指定字段的值。
func (r *RedisClient) HGet(ctx context.Context, key, field string) (string, error) {
	if r.Client == nil {
		return "", ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	return r.Client.HGet(ctx, key, field).Result()
}

// HMGet 批量获取 Hash 中多个字段的值，返回与 fields 顺序对应的 []interface{}。
func (r *RedisClient) HMGet(ctx context.Context, key string, fields ...string) ([]interface{}, error) {
	if r.Client == nil {
		return nil, ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	return r.Client.HMGet(ctx, key, fields...).Result()
}

// HGetAll 获取 Hash 中所有字段和值的 map。
func (r *RedisClient) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	if r.Client == nil {
		return nil, ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	return r.Client.HGetAll(ctx, key).Result()
}

// HDel 删除 Hash 中一个或多个字段。
func (r *RedisClient) HDel(ctx context.Context, key string, fields ...string) error {
	if r.Client == nil {
		return ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	return r.Client.HDel(ctx, key, fields...).Err()
}

// SIsMember 判断 member 是否在 SET 中（O(1) 复杂度）。
func (r *RedisClient) SIsMember(ctx context.Context, key, member string) (bool, error) {
	if r.Client == nil {
		return false, ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	return r.Client.SIsMember(ctx, key, member).Result()
}

// SAdd 向 SET 添加一个或多个 member。
func (r *RedisClient) SAdd(ctx context.Context, key string, members ...interface{}) error {
	if r.Client == nil {
		return ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	return r.Client.SAdd(ctx, key, members...).Err()
}

// SAddWithResult 向 SET 添加 member 并返回新增的 member 数量。
// 返回 1 表示新增 member，返回 0 表示 member 已存在（用于原子判重）。
func (r *RedisClient) SAddWithResult(ctx context.Context, key string, members ...interface{}) (int64, error) {
	if r.Client == nil {
		return 0, ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	return r.Client.SAdd(ctx, key, members...).Result()
}

// slidingWindowScript 滑动窗口限流的 Lua 脚本。
// KEYS[1]: 限流键
// ARGV[1]: 当前时间戳（毫秒）
// ARGV[2]: 窗口大小（毫秒）
// ARGV[3]: 窗口内最大请求数
// ARGV[4]: 请求唯一标识（时间戳+随机数）
// 返回 1 表示允许通过，0 表示被限流。
const slidingWindowScript = `
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])

redis.call('ZREMRANGEBYSCORE', key, '-inf', now - window)

local count = redis.call('ZCARD', key)

if count < limit then
    redis.call('ZADD', key, now, now .. ':' .. ARGV[4])
    redis.call('EXPIRE', key, window / 1000 + 1)
    return 1
else
    return 0
end
`

// SlidingWindowLimit 滑动窗口限流，通过 Lua 脚本保证原子性。
// key: 限流键，maxRequests: 窗口内最大请求数，window: 时间窗口。
// 返回 true 表示允许通过，false 表示被限流。
func (r *RedisClient) SlidingWindowLimit(ctx context.Context, key string, maxRequests int64, window time.Duration) (bool, error) {
	if r.Client == nil {
		return false, ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	now := time.Now().UnixMilli()
	result, err := r.Client.Eval(ctx, slidingWindowScript, []string{key},
		now,
		window.Milliseconds(),
		maxRequests,
		fmt.Sprintf("%d", now),
	).Int64()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

// HIncrBy 为 Hash 中指定字段的值原子递增 increment。
// 封装了 nil 检查和超时控制，避免调用方直接访问 r.Client 导致 nil panic。
func (r *RedisClient) HIncrBy(ctx context.Context, key, field string, increment int64) (int64, error) {
	if r.Client == nil {
		return 0, ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	return r.Client.HIncrBy(ctx, key, field, increment).Result()
}

// HSetNX 仅当 Hash 中指定字段不存在时设置值（HSETNX 语义）。
// 用于回填动态计数时避免覆盖 HIncrBy 的并发递增结果。
func (r *RedisClient) HSetNX(ctx context.Context, key, field string, value interface{}) (bool, error) {
	if r.Client == nil {
		return false, ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	return r.Client.HSetNX(ctx, key, field, value).Result()
}

// SRem 从 SET 中移除一个或多个 member。
// 用于 MySQL 写入失败时回滚 Redis SET 中的 SADD 操作。
func (r *RedisClient) SRem(ctx context.Context, key string, members ...interface{}) error {
	if r.Client == nil {
		return ErrRedisUnavailable
	}
	ctx, cancel := r.WithTimeout(ctx)
	defer cancel()
	return r.Client.SRem(ctx, key, members...).Err()
}
