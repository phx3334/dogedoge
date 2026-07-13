package redis

import (
	"context"
	"strconv"
	"time"

	"fake_tiktok/internal/dto/cache"
	"fake_tiktok/internal/pkg"
	"fake_tiktok/internal/repository/interfaces"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var _ interfaces.VideoCacheRepository = (*VideoCacheRepo)(nil)

type VideoCacheRepo struct {
	client *RedisClient
	logger *zap.Logger
}

func NewVideoCacheRepo(client *RedisClient, logger *zap.Logger) *VideoCacheRepo {
	return &VideoCacheRepo{client: client, logger: logger}
}

// GetVideoCache 批量查询视频缓存（静态 Hash + 动态 Hash）
//
// 返回值说明：
//   - map[uint]*cache.VideoCacheData: 命中的视频缓存数据（包括空对象标记）
//   - []uint: 未命中的视频 ID 列表（需要从 MySQL 回源）
//   - error: Pipeline 整体执行错误（如 Redis 连接断开、超时等）
//
// 错误处理策略：
//   - Pipeline 完全失败（execErr != nil && len(cmds) == 0）时返回 error，
//     调用方应将此 error 返回给熔断器闭包，使熔断器能感知 Redis 不可用
//   - Pipeline 部分失败（某些 key 的命令失败）时，仅该 key 被标记为未命中，
//     不影响其他 key 的结果
func (r *VideoCacheRepo) GetVideoCache(ctx context.Context, videoIDs []uint) (map[uint]*cache.VideoCacheData, []uint, error) {
	cacheMap := make(map[uint]*cache.VideoCacheData)
	hitMap := make(map[uint]bool)

	// 为 Pipeline 设置超时，防止 Redis 慢查询阻塞业务
	ctxPipe, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	pipe := r.client.Pipeline()

	for _, vid := range videoIDs {
		staticKey := r.client.BuildKey(VideoStaticHashKey, strconv.FormatUint(uint64(vid), 10))
		pipe.HGetAll(ctxPipe, staticKey)

		dynamicKey := r.client.BuildKey(VideoDynamicHashKey, strconv.FormatUint(uint64(vid), 10))
		pipe.HGetAll(ctxPipe, dynamicKey)

		// 修复：查询空对象标记，防止缓存穿透
		// 之前只写入了 VideoEmptyHashKey 但从未读取，导致空对象防穿透机制完全失效
		emptyKey := r.client.BuildKey(VideoEmptyHashKey, strconv.FormatUint(uint64(vid), 10))
		pipe.HGetAll(ctxPipe, emptyKey)
	}

	cmds, execErr := pipe.Exec(ctxPipe)
	if execErr != nil && len(cmds) == 0 {
		// Pipeline 完全失败（如连接断开），返回错误让熔断器感知
		return cacheMap, pkg.CacheMissIDs(hitMap, videoIDs), execErr
	}

	for i, vid := range videoIDs {
		cmdIdx := i * 3 // 修复：每个 videoID 现在有 3 个命令（static + dynamic + empty）
		if cmdIdx+2 >= len(cmds) {
			break
		}

		data := &cache.VideoCacheData{}
		hasStaticData := false

		// 检查空对象标记
		if emptyCmd, ok := cmds[cmdIdx+2].(*redis.MapStringStringCmd); ok {
			emptyMap := emptyCmd.Val()
			if len(emptyMap) > 0 {
				// 命中空对象标记：视频确实不存在，标记为命中避免穿透到 MySQL
				data.IsEmpty = true
				hitMap[vid] = true
				cacheMap[vid] = data
				continue
			}
		}

		if staticCmd, ok := cmds[cmdIdx].(*redis.MapStringStringCmd); ok {
			staticMap := staticCmd.Val()
			if len(staticMap) > 0 {
				hasStaticData = true
				data.PlayURL = staticMap["play_url"]
				data.CoverURL = staticMap["cover_url"]
				if dur, err := strconv.ParseFloat(staticMap["duration"], 64); err == nil {
					data.Duration = dur
				}
				data.AuthorID = staticMap["author_id"]
				data.Title = staticMap["title"]
				data.Description = staticMap["description"]
				data.Zone = staticMap["zone"]
				data.AuthorName = staticMap["author_name"]
			data.AuthorAvatar = staticMap["author_avatar"]
				if bv, err := strconv.ParseBool(staticMap["comments_closed"]); err == nil {
					data.CommentsClosed = bv
				}
				if bv, err := strconv.ParseBool(staticMap["danmaku_closed"]); err == nil {
					data.DanmakuClosed = bv
				}
				if pop, err := strconv.ParseFloat(staticMap["popularity"], 64); err == nil {
					data.Popularity = pop
				}
				if ts, err := strconv.ParseInt(staticMap["created_at"], 10, 64); err == nil {
					data.CreatedAt = time.Unix(ts, 0)
				}
			}
		}

		// 关键修复：只有静态数据存在时，才认为缓存命中
		// 静态+动态都为空 = 缓存不存在，必须加入 missedIDs 触发 MySQL 回源
		// 避免返回全零值的 data 被误认为"已命中"
		if !hasStaticData {
			// Redis 中无静态数据且无空对象标记 → 标记 IsEmpty=true，
			// 使调用方的 !data.IsEmpty 检查跳过此条目，触发 MySQL 回源
			data.IsEmpty = true
		}
		if hasStaticData {
			if dynCmd, ok := cmds[cmdIdx+1].(*redis.MapStringStringCmd); ok {
				dynMap := dynCmd.Val()
				if pl, err := strconv.ParseInt(dynMap["play_count"], 10, 64); err == nil {
					data.PlayCount = pl
				}
				if cc, err := strconv.ParseInt(dynMap["comment_count"], 10, 64); err == nil {
					data.CommentCnt = cc
				}
				if lc, err := strconv.ParseInt(dynMap["likes_count"], 10, 64); err == nil {
					data.LikesCnt = lc
				}
				if fc, err := strconv.ParseUint(dynMap["fav_count"], 10, 64); err == nil {
					data.FavCnt = fc
				}
				if cc, err := strconv.ParseUint(dynMap["coin_count"], 10, 64); err == nil {
					data.CoinCnt = cc
				}
				if dc, err := strconv.ParseUint(dynMap["danmaku_count"], 10, 64); err == nil {
					data.DanmakuCnt = dc
				}
			}
			hitMap[vid] = true
		}

		cacheMap[vid] = data
	}

	return cacheMap, pkg.CacheMissIDs(hitMap, videoIDs), nil
}

func (r *VideoCacheRepo) IncrementPlayCount(ctx context.Context, videoID uint) error {
	dynamicKey := r.client.BuildKey(VideoDynamicHashKey, strconv.FormatUint(uint64(videoID), 10))
	// 修复：使用 RedisClient.HIncrBy 封装，避免直接访问 r.client.Client 导致 nil panic
	_, err := r.client.HIncrBy(ctx, dynamicKey, "play_count", 1)
	return err
}

// IncrementLikeCount 视频动态缓存中 likes_count +1
// 使后续请求能立即看到更新后的点赞数，无需等待 MySQL 同步
func (r *VideoCacheRepo) IncrementLikeCount(ctx context.Context, videoID uint) error {
	dynamicKey := r.client.BuildKey(VideoDynamicHashKey, strconv.FormatUint(uint64(videoID), 10))
	// 修复：使用 RedisClient.HIncrBy 封装，避免直接访问 r.client.Client 导致 nil panic
	_, err := r.client.HIncrBy(ctx, dynamicKey, "likes_count", 1)
	return err
}

// DecrementLikeCount 视频动态缓存中 likes_count -1（取消点赞）
// Redis HINCRBY 允许负数；缓存未命中时 key 不存在会自动建为 -1，但
// 下次 MySQL 回源时会被正确值覆盖，业务可容忍小幅偏差
func (r *VideoCacheRepo) DecrementLikeCount(ctx context.Context, videoID uint) error {
	dynamicKey := r.client.BuildKey(VideoDynamicHashKey, strconv.FormatUint(uint64(videoID), 10))
	_, err := r.client.HIncrBy(ctx, dynamicKey, "likes_count", -1)
	return err
}

// IncrementDanmakuCount 视频动态缓存中 danmaku_count +1（发送弹幕时调用）
func (r *VideoCacheRepo) IncrementDanmakuCount(ctx context.Context, videoID uint) error {
	dynamicKey := r.client.BuildKey(VideoDynamicHashKey, strconv.FormatUint(uint64(videoID), 10))
	_, err := r.client.HIncrBy(ctx, dynamicKey, "danmaku_count", 1)
	return err
}

// SetDanmakuCount 直接设置视频动态缓存中的 danmaku_count
// 用于 GetDanmakuList 自愈校正：用实际弹幕列表长度覆盖可能漂移的计数值
func (r *VideoCacheRepo) SetDanmakuCount(ctx context.Context, videoID uint, count uint64) error {
	dynamicKey := r.client.BuildKey(VideoDynamicHashKey, strconv.FormatUint(uint64(videoID), 10))
	return r.client.HSet(ctx, dynamicKey, "danmaku_count", count)
}

// IncrementFavCount 视频动态缓存中 fav_count +1（收藏时调用）
func (r *VideoCacheRepo) IncrementFavCount(ctx context.Context, videoID uint) error {
	dynamicKey := r.client.BuildKey(VideoDynamicHashKey, strconv.FormatUint(uint64(videoID), 10))
	_, err := r.client.HIncrBy(ctx, dynamicKey, "fav_count", 1)
	return err
}

// DecrementFavCount 视频动态缓存中 fav_count -1（取消收藏时调用）
// 使用 Lua 脚本保证不会减为负数：仅当当前值 > 0 时才减 1
// 避免 Redis 缓存出现 -1 导致前端显示异常（与 MySQL 端 GREATEST(fav_count-?, 0) 对齐）
func (r *VideoCacheRepo) DecrementFavCount(ctx context.Context, videoID uint) error {
	if r.client.Client == nil {
		return ErrRedisUnavailable
	}
	dynamicKey := r.client.BuildKey(VideoDynamicHashKey, strconv.FormatUint(uint64(videoID), 10))
	// Lua 脚本：field 不存在时 HGET 返回 false，不会进入减法分支
	// 存在且 > 0 才减 1；不存在或已为 0 则不动
	script := `local v = redis.call('HGET', KEYS[1], ARGV[1])
if v then
  local n = tonumber(v)
  if n and n > 0 then
    return redis.call('HINCRBY', KEYS[1], ARGV[1], -1)
  end
end
return 0`
	ctx, cancel := r.client.WithTimeout(ctx)
	defer cancel()
	return r.client.Client.Eval(ctx, script, []string{dynamicKey}, "fav_count").Err()
}

// IncrementCoinCount 视频动态缓存中 coin_count +delta（投币时调用）
func (r *VideoCacheRepo) IncrementCoinCount(ctx context.Context, videoID uint, delta int) error {
	dynamicKey := r.client.BuildKey(VideoDynamicHashKey, strconv.FormatUint(uint64(videoID), 10))
	_, err := r.client.HIncrBy(ctx, dynamicKey, "coin_count", int64(delta))
	return err
}

// IncrementCommentCount 视频动态缓存中 comment_count +delta（评论时调用）
func (r *VideoCacheRepo) IncrementCommentCount(ctx context.Context, videoID uint, delta int) error {
	dynamicKey := r.client.BuildKey(VideoDynamicHashKey, strconv.FormatUint(uint64(videoID), 10))
	_, err := r.client.HIncrBy(ctx, dynamicKey, "comment_count", int64(delta))
	return err
}

func (r *VideoCacheRepo) WriteVideoCache(ctx context.Context, items []*cache.VideoCacheData, emptyIDs []uint) {
	if len(items) == 0 && len(emptyIDs) == 0 {
		return
	}

	pipe := r.client.Pipeline()

	ctxPipe, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	for _, item := range items {
		vidStr := strconv.FormatUint(uint64(item.VideoID), 10)

		staticKey := r.client.BuildKey(VideoStaticHashKey, vidStr)
		pipe.HSet(ctxPipe, staticKey,
			"play_url", item.PlayURL,
			"cover_url", item.CoverURL,
			"duration", strconv.FormatFloat(item.Duration, 'f', 2, 64),
			"author_id", item.AuthorID,
			"title", item.Title,
			"description", item.Description,
			"zone", item.Zone,
			"author_name", item.AuthorName,
			"author_avatar", item.AuthorAvatar,
			"comments_closed", strconv.FormatBool(item.CommentsClosed),
			"danmaku_closed", strconv.FormatBool(item.DanmakuClosed),
			"popularity", strconv.FormatFloat(item.Popularity, 'f', 2, 64),
			"created_at", item.CreatedAt.Unix(),
		)
		pipe.Expire(ctxPipe, staticKey, TTLJitter(VideoStaticHashExpire, 1*time.Hour))

		dynamicKey := r.client.BuildKey(VideoDynamicHashKey, vidStr)
		// 修复：使用 HSETNX 回填动态计数，避免覆盖 HIncrBy 的并发递增结果
		// HSETNX 仅当字段不存在时写入，保证 HIncrBy 的增量不被回填的旧值覆盖
		pipe.HSetNX(ctxPipe, dynamicKey, "play_count", item.PlayCount)
		pipe.HSetNX(ctxPipe, dynamicKey, "comment_count", item.CommentCnt)
		pipe.HSetNX(ctxPipe, dynamicKey, "likes_count", item.LikesCnt)
		pipe.HSetNX(ctxPipe, dynamicKey, "fav_count", item.FavCnt)
		pipe.HSetNX(ctxPipe, dynamicKey, "coin_count", item.CoinCnt)
		pipe.HSetNX(ctxPipe, dynamicKey, "danmaku_count", item.DanmakuCnt)
		// 动态缓存不过期（VideoDynamicHashExpire=0），由业务写操作实时维护
		if VideoDynamicHashExpire > 0 {
			pipe.Expire(ctxPipe, dynamicKey, TTLJitter(VideoDynamicHashExpire, 1*time.Hour))
		}
	}

	for _, vid := range emptyIDs {
		emptyKey := r.client.BuildKey(VideoEmptyHashKey, strconv.FormatUint(uint64(vid), 10))
		pipe.HSet(ctxPipe, emptyKey, "empty", "1")
		pipe.Expire(ctxPipe, emptyKey, VideoEmptyHashExpire)
	}

	if _, err := pipe.Exec(ctxPipe); err != nil {
		r.logger.Error("[BatchCache] batch write cache failed", zap.Error(err))
	}
}

// DeleteVideoCache 删除指定视频的全部 Redis 缓存：静态 Hash、动态 Hash、空对象标记。
// 删除后该视频从缓存失效，后续请求回源 MySQL；若视频已软删除，回源返回 404。
func (r *VideoCacheRepo) DeleteVideoCache(ctx context.Context, videoID uint) error {
	if r.client.Client == nil {
		return ErrRedisUnavailable
	}
	ctx, cancel := r.client.WithTimeout(ctx)
	defer cancel()
	vid := strconv.FormatUint(uint64(videoID), 10)
	keys := []string{
		r.client.BuildKey(VideoStaticHashKey, vid),
		r.client.BuildKey(VideoDynamicHashKey, vid),
		r.client.BuildKey(VideoEmptyHashKey, vid),
	}
	return r.client.Client.Del(ctx, keys...).Err()
}
