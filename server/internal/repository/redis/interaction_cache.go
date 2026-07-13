package redis

import (
	"context"
	"strconv"
	"time"

	"fake_tiktok/internal/repository/interfaces"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var _ interfaces.InteractionCacheRepository = (*InteractionCacheRepo)(nil)

type InteractionCacheRepo struct {
	client *RedisClient
	logger *zap.Logger
}

func NewInteractionCacheRepo(client *RedisClient, logger *zap.Logger) *InteractionCacheRepo {
	return &InteractionCacheRepo{client: client, logger: logger}
}

// saddWithExpireScript 将 SADD + EXPIRE 合并为原子操作，避免 SADD 成功但 EXPIRE 失败导致 SET 永不过期。
// KEYS[1]: SET 键名
// ARGV[1]: member（用户ID）
// ARGV[2]: 过期时间（秒）
// 返回：SADD 的新增 member 数量（1=新增，0=已存在）
const saddWithExpireScript = `
local added = redis.call('SADD', KEYS[1], ARGV[1])
if added == 1 then
    redis.call('EXPIRE', KEYS[1], ARGV[2])
end
return added
`

// GetInteractionBatch 通过 Pipeline 一次性查询点赞/收藏/投币/关注 4 个互动缓存。
//
// 数据结构：
//   - 点赞/收藏/关注：SET，使用 SISMEMBER 查询用户是否在集合中
//   - 投币数：Hash field "count"，使用 HGet 查询
//
// 返回值说明：
//   - isLiked/isFavorited/isFollowed: 已命中字段的值（未命中字段为零值）
//   - coinCount: 投币数（未命中为零值）
//   - hitMask: 位掩码，标记哪些字段命中了缓存（调用方据此决定是否降级回源）
//     bit0=点赞, bit1=收藏, bit2=投币, bit3=关注
//   - err: Pipeline 整体执行错误（非单个 key 缺失错误）
func (r *InteractionCacheRepo) GetInteractionBatch(ctx context.Context, userID string, videoID uint, authorID string) (isLiked, isFavorited, isFollowed bool, coinCount int64, hitMask uint8, err error) {
	ctxPipe, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	pipe := r.client.Pipeline()

	// 点赞状态：先检查 SET 是否存在，再查 SISMEMBER
	likeKey := r.client.BuildKey(VideoLikedUsersSetKey, strconv.FormatUint(uint64(videoID), 10))
	pipe.Exists(ctxPipe, likeKey)
	pipe.SIsMember(ctxPipe, likeKey, userID)

	// 收藏状态：先检查 SET 是否存在，再查 SISMEMBER
	favKey := r.client.BuildKey(VideoFavoritedUsersSetKey, strconv.FormatUint(uint64(videoID), 10))
	pipe.Exists(ctxPipe, favKey)
	pipe.SIsMember(ctxPipe, favKey, userID)

	// 投币数：HGet 查询 Hash 中的 count 字段
	coinKey := r.client.BuildKey(VideoCoinHashKey, userID+":"+strconv.FormatUint(uint64(videoID), 10))
	pipe.HGet(ctxPipe, coinKey, "count")

	// 关注状态：先检查 SET 是否存在，再查 SISMEMBER
	var followCmdIdx int
	if authorID != "" {
		followKey := r.client.BuildKey(UserFollowersSetKey, authorID)
		pipe.Exists(ctxPipe, followKey)
		pipe.SIsMember(ctxPipe, followKey, userID)
		followCmdIdx = 7 // exists(0) + sismember(1) + exists(2) + sismember(3) + hget(4) + exists(5) + sismember(6)
	}

	cmds, execErr := pipe.Exec(ctxPipe)
	if execErr != nil && len(cmds) == 0 {
		err = execErr
		return
	}

	// 解析点赞（bit0）：EXISTS + SISMEMBER
	// 修复：SISMEMBER 对不存在的 SET 返回 (false, nil)，会被误判为"缓存命中且未点赞"。
	// 增加 EXISTS 检查：只有 SET 存在时才标记为缓存命中，SET 不存在视为缓存未命中需回源 MySQL。
	if len(cmds) > 1 {
		if existsCmd, ok := cmds[0].(*redis.IntCmd); ok {
			if exists, _ := existsCmd.Result(); exists > 0 {
				if cmd, ok := cmds[1].(*redis.BoolCmd); ok {
					val, hErr := cmd.Result()
					if hErr == nil {
						isLiked = val
						hitMask |= 0x01
					}
				}
			}
		}
	}

	// 解析收藏（bit1）：EXISTS + SISMEMBER
	if len(cmds) > 3 {
		if existsCmd, ok := cmds[2].(*redis.IntCmd); ok {
			if exists, _ := existsCmd.Result(); exists > 0 {
				if cmd, ok := cmds[3].(*redis.BoolCmd); ok {
					val, hErr := cmd.Result()
					if hErr == nil {
						isFavorited = val
						hitMask |= 0x02
					}
				}
			}
		}
	}

	// 解析投币数（bit2）：HGet 返回 StringCmd
	if len(cmds) > 4 {
		if cmd, ok := cmds[4].(*redis.StringCmd); ok {
			val, hErr := cmd.Result()
			if hErr == nil {
				coinCount, _ = strconv.ParseInt(val, 10, 64)
				hitMask |= 0x04
			}
		}
	}

	// 解析关注（bit3）：EXISTS + SISMEMBER
	if followCmdIdx > 0 && len(cmds) > followCmdIdx {
		if existsCmd, ok := cmds[followCmdIdx-1].(*redis.IntCmd); ok {
			if exists, _ := existsCmd.Result(); exists > 0 {
				if cmd, ok := cmds[followCmdIdx].(*redis.BoolCmd); ok {
					val, hErr := cmd.Result()
					if hErr == nil {
						isFollowed = val
						hitMask |= 0x08
					}
				}
			}
		}
	}

	return
}

// ---- 点赞 SET ----

// IsVideoLikedByUser 使用 SET 判断用户是否已点赞某视频（O(1) 复杂度）
// SET key 格式: {prefix}:video:liked_users:{videoID}，member=userID
// 返回 true=已点赞，false=未点赞
func (r *InteractionCacheRepo) IsVideoLikedByUser(ctx context.Context, userID string, videoID uint) (bool, error) {
	key := r.client.BuildKey(VideoLikedUsersSetKey, strconv.FormatUint(uint64(videoID), 10))
	return r.client.SIsMember(ctx, key, userID)
}

// AddUserToVideoLikedSet 将用户加入视频的点赞 SET（标记已点赞）
// 返回值 added=true 表示本次新增（首次点赞），added=false 表示用户已在 SET 中（重复点赞）
//
// 原子判重设计：
//   - 使用 SADD 返回值（新增 member 数）判断是否首次点赞，而非 SISMEMBER + SADD 两步操作
//   - SADD 本身是原子的：返回 1 表示新增 member，返回 0 表示 member 已存在
//   - 避免了 SISMEMBER + SADD 两步非原子的竞态条件（并发请求可能同时通过 SISMEMBER 检查）
//
// TTL 策略：
//   - 使用 Lua 脚本原子执行 SADD + EXPIRE，避免 SADD 成功但 EXPIRE 失败导致 SET 永不过期
//   - 冷门视频的 SET 会在 TTL 到期后自动清理，防止内存膨胀
func (r *InteractionCacheRepo) AddUserToVideoLikedSet(ctx context.Context, userID string, videoID uint) (bool, error) {
	key := r.client.BuildKey(VideoLikedUsersSetKey, strconv.FormatUint(uint64(videoID), 10))
	// 使用 Lua 脚本原子执行 SADD + EXPIRE，避免 SADD 成功但 EXPIRE 失败导致 SET 永不过期
	result, err := r.client.Client.Eval(ctx, saddWithExpireScript, []string{key}, userID, int64(InteractionCacheExpire/time.Second)).Int64()
	if err != nil {
		return false, err
	}
	return result > 0, nil
}

// ---- 收藏 SET ----

// IsVideoFavoritedByUser 使用 SET 判断用户是否已收藏某视频（O(1) 复杂度）
// SET key 格式: {prefix}:video:favorited_users:{videoID}，member=userID
// 返回 true=已收藏，false=未收藏
func (r *InteractionCacheRepo) IsVideoFavoritedByUser(ctx context.Context, userID string, videoID uint) (bool, error) {
	key := r.client.BuildKey(VideoFavoritedUsersSetKey, strconv.FormatUint(uint64(videoID), 10))
	return r.client.SIsMember(ctx, key, userID)
}

// AddUserToVideoFavoriteSet 将用户加入视频的收藏 SET（标记已收藏）
//
// 与 AddUserToVideoLikedSet 设计一致：
//   - Lua 脚本原子执行 SADD + EXPIRE
func (r *InteractionCacheRepo) AddUserToVideoFavoriteSet(ctx context.Context, userID string, videoID uint) (bool, error) {
	key := r.client.BuildKey(VideoFavoritedUsersSetKey, strconv.FormatUint(uint64(videoID), 10))
	result, err := r.client.Client.Eval(ctx, saddWithExpireScript, []string{key}, userID, int64(InteractionCacheExpire/time.Second)).Int64()
	if err != nil {
		return false, err
	}
	return result > 0, nil
}

// ---- 投币 Hash ----

// GetCoinCount 查询用户投币数（Hash field "count"）
func (r *InteractionCacheRepo) GetCoinCount(ctx context.Context, userID string, videoID uint) (int64, error) {
	key := r.client.BuildKey(VideoCoinHashKey, userID+":"+strconv.FormatUint(uint64(videoID), 10))
	val, err := r.client.HGet(ctx, key, "count")
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(val, 10, 64)
}

// SetCoinCount 写入投币数缓存（Hash field "count"）
// 使用 Pipeline 将 HSet + Expire 合并为单次网络往返
func (r *InteractionCacheRepo) SetCoinCount(ctx context.Context, userID string, videoID uint, count int64) error {
	key := r.client.BuildKey(VideoCoinHashKey, userID+":"+strconv.FormatUint(uint64(videoID), 10))
	pipe := r.client.Pipeline()
	pipe.HSet(ctx, key, "count", count)
	pipe.Expire(ctx, key, InteractionCacheExpire)
	_, err := pipe.Exec(ctx)
	return err
}

// ---- 关注 SET ----

// IsUserFollowedByUser 使用 SET 判断用户是否已关注某作者（O(1) 复杂度）
// SET key 格式: {prefix}:user:followers:{followeeID}，member=followerID
// 返回 true=已关注，false=未关注
func (r *InteractionCacheRepo) IsUserFollowedByUser(ctx context.Context, followerID, followeeID string) (bool, error) {
	key := r.client.BuildKey(UserFollowersSetKey, followeeID)
	return r.client.SIsMember(ctx, key, followerID)
}

// AddUserToFollowersSet 将用户加入关注对象的粉丝 SET（标记已关注）
// 返回值 added=true 表示本次新增（首次关注），added=false 表示已在 SET 中（重复关注）
//
// 与 AddUserToVideoLikedSet 设计一致：
//   - Lua 脚本原子执行 SADD + EXPIRE
func (r *InteractionCacheRepo) AddUserToFollowersSet(ctx context.Context, followerID, followeeID string) (bool, error) {
	key := r.client.BuildKey(UserFollowersSetKey, followeeID)
	result, err := r.client.Client.Eval(ctx, saddWithExpireScript, []string{key}, followerID, int64(InteractionCacheExpire/time.Second)).Int64()
	if err != nil {
		return false, err
	}
	return result > 0, nil
}

// RemoveUserFromVideoLikedSet 从视频点赞 SET 中移除用户（回滚 SADD 操作）。
// 用于 MySQL 写入失败时回滚 Redis SET，防止 SADD 成功但 MySQL 失败导致的数据不一致。
func (r *InteractionCacheRepo) RemoveUserFromVideoLikedSet(ctx context.Context, userID string, videoID uint) error {
	key := r.client.BuildKey(VideoLikedUsersSetKey, strconv.FormatUint(uint64(videoID), 10))
	return r.client.SRem(ctx, key, userID)
}

// RemoveUserFromVideoFavoriteSet 从视频收藏 SET 中移除用户（回滚 SADD 操作）。
func (r *InteractionCacheRepo) RemoveUserFromVideoFavoriteSet(ctx context.Context, userID string, videoID uint) error {
	key := r.client.BuildKey(VideoFavoritedUsersSetKey, strconv.FormatUint(uint64(videoID), 10))
	return r.client.SRem(ctx, key, userID)
}

// RemoveUserFromFollowersSet 从关注 SET 中移除用户（回滚 SADD 操作）。
func (r *InteractionCacheRepo) RemoveUserFromFollowersSet(ctx context.Context, followerID, followeeID string) error {
	key := r.client.BuildKey(UserFollowersSetKey, followeeID)
	return r.client.SRem(ctx, key, followerID)
}

// BackfillBatch 使用 Redis Pipeline 将多个互动缓存写操作合并为一次原子执行，
// 避免部分成功导致缓存不一致（如点赞缓存回填成功但关注缓存回填失败）。
func (r *InteractionCacheRepo) BackfillBatch(ctx context.Context, userID string, videoID uint, authorID string, status *interfaces.InteractionStatus) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	pipe := r.client.Pipeline()

	// 回填点赞缓存：如果已点赞则加入 SET
	if status.IsLiked {
		likeKey := r.client.BuildKey(VideoLikedUsersSetKey, strconv.FormatUint(uint64(videoID), 10))
		pipe.SAdd(ctx, likeKey, userID)
		pipe.Expire(ctx, likeKey, InteractionCacheExpire)
	}

	// 回填收藏缓存：如果已收藏则加入 SET
	if status.IsFavorited {
		favKey := r.client.BuildKey(VideoFavoritedUsersSetKey, strconv.FormatUint(uint64(videoID), 10))
		pipe.SAdd(ctx, favKey, userID)
		pipe.Expire(ctx, favKey, InteractionCacheExpire)
	}

	// 回填投币数缓存（0 也写，防止穿透）
	coinKey := r.client.BuildKey(VideoCoinHashKey, userID+":"+strconv.FormatUint(uint64(videoID), 10))
	pipe.HSet(ctx, coinKey, "count", status.CoinCount)
	pipe.Expire(ctx, coinKey, InteractionCacheExpire)

	// 回填关注状态缓存：如果已关注则加入 SET
	if status.IsFollowed && authorID != "" {
		followKey := r.client.BuildKey(UserFollowersSetKey, authorID)
		pipe.SAdd(ctx, followKey, userID)
		pipe.Expire(ctx, followKey, InteractionCacheExpire)
	}

	_, err := pipe.Exec(ctx)
	return err
}
