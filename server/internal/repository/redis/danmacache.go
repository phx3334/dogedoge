package redis

import (
	"context"
	"encoding/json"
	"fake_tiktok/internal/dto/response"
	"fake_tiktok/internal/repository/interfaces"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var _ interfaces.DanmakuCacheRepository = (*DanmakuCacheRepo)(nil)

type DanmakuCacheRepo struct {
	client *RedisClient
	logger *zap.Logger
}

func NewDanmakuCacheRepo(client *RedisClient, logger *zap.Logger) *DanmakuCacheRepo {
	return &DanmakuCacheRepo{client, logger}
}

func (d *DanmakuCacheRepo) GetDanmakuCache(ctx context.Context, videoID uint64) ([]*response.DanmakuItem, error) {
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	key := d.client.BuildKey(DanmakuCacheKey, strconv.FormatUint(uint64(videoID), 10))
	results, err := d.client.ZRevRangeWithScores(ctx, key, 0, -1)
	if err != nil || len(results) == 0 {
		return nil, err
	}
	items := make([]*response.DanmakuItem, 0, len(results))
	for _, s := range results {
		var item response.DanmakuItem
		// 兼容 Member 为 string 或 []byte 两种类型：
		// - WriteDanmakuCache/BackfillDanmakuCache 写入的是 []byte（json.Marshal 返回值）
		// - Create 方法写入的也是 []byte
		// go-redis 读取时统一返回 string，但保险起见做类型断言
		var memberStr string
		switch v := s.Member.(type) {
		case string:
			memberStr = v
		case []byte:
			memberStr = string(v)
		default:
			d.logger.Warn("danmaku cache member has unexpected type",
				zap.Uint64("video_id", videoID),
				zap.String("type", fmt.Sprintf("%T", s.Member)))
			continue
		}
		if err := json.Unmarshal([]byte(memberStr), &item); err != nil {
			d.logger.Warn("failed to unmarshal danmaku cache item",
				zap.Uint64("video_id", videoID), zap.Error(err))
			continue
		}
		items = append(items, &item)
	}
	return items, nil
}

// createIfExistsScript 仅当弹幕缓存 key 已存在时才 ZADD，避免在缓存过期后创建只含单条弹幕的"部分缓存"
// 部分缓存会导致 GetDanmakuList 误判为缓存命中，跳过 MySQL 回源，使旧弹幕全部丢失
var createIfExistsScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 1 then
    return redis.call('ZADD', KEYS[1], ARGV[1], ARGV[2])
else
    return 0
end
`)

// Create 向 Redis 弹幕有序集合中新增单条弹幕
// 发送弹幕后调用，保证缓存中的弹幕列表与 MySQL 实时一致，避免刷新页面后新弹幕消失
// 关键修复：仅当缓存 key 已存在时才追加，避免缓存过期后创建只含单条弹幕的部分缓存
// 如果缓存已过期，不写入缓存，下次 GET 请求会从 MySQL 全量回填
func (d *DanmakuCacheRepo) Create(ctx context.Context, videoID uint64, item *response.DanmakuItem) error {
	if d.client == nil || d.client.Client == nil {
		return ErrRedisUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	jsonData, err := json.Marshal(item)
	if err != nil {
		return err
	}
	key := d.client.BuildKey(DanmakuCacheKey, strconv.FormatUint(uint64(videoID), 10))
	_, err = createIfExistsScript.Run(ctx, d.client.Client, []string{key},
		float64(item.CreatedAt), jsonData).Result()
	return err
}

func (d *DanmakuCacheRepo) WriteDanmakuCache(ctx context.Context, videoID uint64, members []redis.Z) {
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	key := d.client.BuildKey(DanmakuCacheKey, strconv.FormatUint(uint64(videoID), 10))
	// 修复：使用 Pipeline 将 ZAdd + Expire 合并为一次网络往返，
	// 避免调用方忘记调用 RefreshDanmakuCacheExpiry 导致缓存永不过期
	pipe := d.client.Pipeline()
	pipe.ZAdd(ctx, key, members...)
	pipe.Expire(ctx, key, 10*time.Minute) // 默认 10 分钟 TTL，后续由 RefreshDanmakuCacheExpiry 调整
	if _, err := pipe.Exec(ctx); err != nil {
		d.logger.Warn("write danmaku cache failed", zap.Error(err))
	}
}

// DeleteDanmakuCache 删除弹幕缓存 key，强制下次 GetDanmakuList 从 MySQL 全量回填
// 用于 SendDanmaku 中 Redis 写入失败（Create/IncrementDanmakuCount）时保证缓存一致性：
// 如果不删除，DanmakuCnt 和缓存列表都保持旧值，陈旧检测无法触发，
// 导致新弹幕虽已写入 MySQL 但对其他用户不可见
func (d *DanmakuCacheRepo) DeleteDanmakuCache(ctx context.Context, videoID uint64) error {
	if d.client == nil || d.client.Client == nil {
		return ErrRedisUnavailable
	}
	key := d.client.BuildKey(DanmakuCacheKey, strconv.FormatUint(uint64(videoID), 10))
	return d.client.Del(ctx, key)
}

// RefreshDanmakuCacheExpiry 刷新弹幕缓存的过期时间。
// 热门视频 7 天过期，非热门视频 10 分钟过期。
// 在缓存命中读取和回填写入后都调用，实现"活跃保活"：频繁访问的视频缓存不会过期。
func (d *DanmakuCacheRepo) RefreshDanmakuCacheExpiry(ctx context.Context, videoID uint64, isHot bool) {
	key := d.client.BuildKey(DanmakuCacheKey, strconv.FormatUint(uint64(videoID), 10))
	var ttl time.Duration
	if isHot {
		ttl = 7 * 24 * time.Hour // 热门视频 7 天
	} else {
		ttl = 10 * time.Minute // 非热门视频 10 分钟
	}
	if err := d.client.Expire(ctx, key, ttl); err != nil {
		d.logger.Warn("refresh danmaku cache expiry failed",
			zap.Uint64("video_id", videoID), zap.Error(err))
	}
}
