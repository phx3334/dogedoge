package redis

import (
	"context"
	"strconv"
	"time"

	"go.uber.org/zap"

	"fake_tiktok/internal/dto/cache"
	"fake_tiktok/internal/repository/interfaces"

	"github.com/redis/go-redis/v9"
)

var _ interfaces.UserCacheRepository = (*UserCacheRepo)(nil)

type UserCacheRepo struct {
	client *RedisClient
	logger *zap.Logger
}

func NewUserCacheRepo(client *RedisClient, logger *zap.Logger) *UserCacheRepo {
	return &UserCacheRepo{client: client, logger: logger}
}

// GetUserCache 通过 Pipeline 并行查询 user:static 和 user:dynamic 两个 Hash，
// 合并为组合 UserCacheData 返回。
// 两个 Hash 都不存在时返回 (nil, nil)。
// 如果 static 未命中但 dynamic 命中，返回的 UserCacheData 中 StaticHit=false，
// 调用方可据此决定是否对 static 区做降级回源（动态区计数不需要回源）。
func (r *UserCacheRepo) GetUserCache(ctx context.Context, userID string) (*cache.UserCacheData, error) {
	staticKey := r.client.BuildKey(UserStaticHashKey, userID)
	dynamicKey := r.client.BuildKey(UserDynamicHashKey, userID)
	emptyKey := r.client.BuildKey(UserEmptyHashKey, userID)

	pipe := r.client.Pipeline()
	emptyCmd := pipe.Exists(ctx, emptyKey)
	staticCmd := pipe.HGetAll(ctx, staticKey)
	dynamicCmd := pipe.HGetAll(ctx, dynamicKey)

	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, err
	}

	// 修复：先检查空对象标记，命中则视为"用户不存在"的缓存命中，避免穿透到 MySQL
	if exists, _ := emptyCmd.Result(); exists > 0 {
		return nil, nil
	}

	staticMap, _ := staticCmd.Result()
	dynamicMap, _ := dynamicCmd.Result()

	// 两个 Hash 都不存在视为缓存未命中
	if len(staticMap) == 0 && len(dynamicMap) == 0 {
		return nil, nil
	}

	var static *cache.UserStaticData
	var dynamic *cache.UserDynamicData

	if len(staticMap) > 0 {
		static = &cache.UserStaticData{
			ID:        staticMap["id"],
			AvatarURL: staticMap["avatar_url"],
			Username:  staticMap["username"],
			Signature: staticMap["signature"],
			Address:   staticMap["address"],
			Birthday:  staticMap["birthday"],
			Gender:    staticMap["gender"],
		}
		if v, err := strconv.ParseBool(staticMap["privacy_public_favorites"]); err == nil {
			static.PrivacyPublicFavorites = v
		}
		if v, err := strconv.ParseBool(staticMap["privacy_public_following"]); err == nil {
			static.PrivacyPublicFollowing = v
		}
		if v, err := strconv.ParseBool(staticMap["privacy_public_fans"]); err == nil {
			static.PrivacyPublicFans = v
		}
		if v, err := strconv.ParseBool(staticMap["view_history_paused"]); err == nil {
			static.ViewHistoryPaused = v
		}
	}

	if len(dynamicMap) > 0 {
		dynamic = &cache.UserDynamicData{}
		if v, err := strconv.ParseInt(dynamicMap["video_count"], 10, 64); err == nil {
			dynamic.VideoCount = v
		}
		if v, err := strconv.ParseInt(dynamicMap["total_likes_received"], 10, 64); err == nil {
			dynamic.TotalLikesReceived = v
		}
		if v, err := strconv.ParseInt(dynamicMap["total_play_count"], 10, 64); err == nil {
			dynamic.TotalPlayCount = v
		}
		if v, err := strconv.ParseUint(dynamicMap["experience"], 10, 64); err == nil {
			dynamic.Experience = v
		}
		if v, err := strconv.ParseInt(dynamicMap["coin_balance_tenths"], 10, 64); err == nil {
			dynamic.CoinBalanceTenths = v
		}
		if v, err := strconv.ParseInt(dynamicMap["fans_count"], 10, 64); err == nil {
			dynamic.FansCount = v
		}
		if v, err := strconv.ParseInt(dynamicMap["following_count"], 10, 64); err == nil {
			dynamic.FollowingCount = v
		}
	}

	result := cache.MergeUserCacheData(static, dynamic)
	// 标记静态区是否命中，调用方据此决定是否对 static 做降级回源
	result.StaticHit = len(staticMap) > 0
	return result, nil
}

// BatchWriteUserCache 批量写入用户缓存，通过 Pipeline 同时写入 static 和 dynamic 两个 Hash
func (r *UserCacheRepo) BatchWriteUserCache(ctx context.Context, items []cache.UserCacheData) {
	if len(items) == 0 {
		return
	}

	pipe := r.client.Pipeline()

	ctxPipe, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	for _, item := range items {
		staticKey := r.client.BuildKey(UserStaticHashKey, item.ID)
		pipe.HSet(ctxPipe, staticKey,
			"id", item.ID,
			"username", item.Username,
			"avatar_url", item.AvatarURL,
			"signature", item.Signature,
			"address", item.Address,
			"birthday", item.Birthday,
			"gender", item.Gender,
			"privacy_public_favorites", strconv.FormatBool(item.PrivacyPublicFavorites),
			"privacy_public_following", strconv.FormatBool(item.PrivacyPublicFollowing),
			"privacy_public_fans", strconv.FormatBool(item.PrivacyPublicFans),
			"view_history_paused", strconv.FormatBool(item.ViewHistoryPaused),
		)
		pipe.Expire(ctxPipe, staticKey, TTLJitter(UserStaticHashExpire, 1*time.Hour))

		dynamicKey := r.client.BuildKey(UserDynamicHashKey, item.ID)
		// 修复：使用 HSETNX 回填动态计数，避免覆盖 HIncrBy 的并发递增结果
		pipe.HSetNX(ctxPipe, dynamicKey, "video_count", item.VideoCount)
		pipe.HSetNX(ctxPipe, dynamicKey, "total_likes_received", item.TotalLikesReceived)
		pipe.HSetNX(ctxPipe, dynamicKey, "total_play_count", item.TotalPlayCount)
		pipe.HSetNX(ctxPipe, dynamicKey, "experience", strconv.FormatUint(item.Experience, 10))
		pipe.HSetNX(ctxPipe, dynamicKey, "coin_balance_tenths", item.CoinBalanceTenths)
		pipe.HSetNX(ctxPipe, dynamicKey, "fans_count", item.FansCount)
		pipe.HSetNX(ctxPipe, dynamicKey, "following_count", item.FollowingCount)
		// 动态缓存不过期（UserDynamicHashExpire=0），由业务写操作实时维护
		if UserDynamicHashExpire > 0 {
			pipe.Expire(ctxPipe, dynamicKey, TTLJitter(UserDynamicHashExpire, 1*time.Hour))
		}
	}

	if _, err := pipe.Exec(ctxPipe); err != nil {
		r.logger.Error("[UserCache] batch write cache failed", zap.Error(err))
	}
}

// IncrementTotalPlayCount 原子自增用户总播放量（HIncrBy +1），写入 user:dynamic Hash
func (r *UserCacheRepo) IncrementTotalPlayCount(ctx context.Context, userID string) error {
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	key := r.client.BuildKey(UserDynamicHashKey, userID)
	// 修复：使用 RedisClient.HIncrBy 封装，避免直接访问 r.client.Client 导致 nil panic
	_, err := r.client.HIncrBy(ctx, key, "total_play_count", 1)
	return err
}

// IncrementFansCount 原子自增用户粉丝数（HIncrBy +1），写入 user:dynamic Hash
func (r *UserCacheRepo) IncrementFansCount(ctx context.Context, userID string) error {
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	key := r.client.BuildKey(UserDynamicHashKey, userID)
	// 修复：使用 RedisClient.HIncrBy 封装
	_, err := r.client.HIncrBy(ctx, key, "fans_count", 1)
	return err
}

// IncrementFollowingCount 原子自增用户关注数（HIncrBy +1），写入 user:dynamic Hash
func (r *UserCacheRepo) IncrementFollowingCount(ctx context.Context, userID string) error {
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	key := r.client.BuildKey(UserDynamicHashKey, userID)
	// 修复：使用 RedisClient.HIncrBy 封装
	_, err := r.client.HIncrBy(ctx, key, "following_count", 1)
	return err
}

// IncrementExperience 原子增减用户经验（HIncrBy delta），写入 user:dynamic Hash
// 与 AddExperience(DB) 同步调用，保证 /user/home、/user/info 读取的缓存经验是最新的
func (r *UserCacheRepo) IncrementExperience(ctx context.Context, userID string, delta int64) error {
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	key := r.client.BuildKey(UserDynamicHashKey, userID)
	_, err := r.client.HIncrBy(ctx, key, "experience", delta)
	return err
}

// DeleteUserCache 仅删除静态区缓存（user:static），动态区缓存（计数类）不受影响。
// 静态信息变更后调用此方法，下次请求时 static 未命中会触发降级回源回填；
// 动态区计数由业务写操作（HIncrBy 等）实时维护，无需删除。
func (r *UserCacheRepo) DeleteUserCache(ctx context.Context, userID string) error {
	staticKey := r.client.BuildKey(UserStaticHashKey, userID)
	// 修复：使用 RedisClient.Del 封装，避免直接访问 r.client.Client
	return r.client.Del(ctx, staticKey)
}

// WriteEmptyUserCache 为不存在的用户写入空对象标记，防止缓存穿透。
// TTL 较短（1分钟），允许数据被补充后快速失效。
func (r *UserCacheRepo) WriteEmptyUserCache(ctx context.Context, userID string) error {
	emptyKey := r.client.BuildKey(UserEmptyHashKey, userID)
	return r.client.Set(ctx, emptyKey, "1", UserEmptyHashExpire)
}

// IsEmptyUser 检查用户是否为空对象标记（用户不存在）。
func (r *UserCacheRepo) IsEmptyUser(ctx context.Context, userID string) (bool, error) {
	emptyKey := r.client.BuildKey(UserEmptyHashKey, userID)
	return r.client.Exists(ctx, emptyKey)
}
