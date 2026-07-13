package logic

import (
	"context"
	"errors"
	"fmt"

	"fake_tiktok/internal/breaker"
	"fake_tiktok/internal/dto/request"

	"go.uber.org/zap"
)

type InteractionLogic struct {
	deps *LogicDeps
}

func NewInteractionLogic(deps *LogicDeps) *InteractionLogic {
	return &InteractionLogic{
		deps: deps,
	}
}

// LikeVideo 用户点赞视频
//
// 流程：
//  1. Redis SADD 原子判重（通过 Redis 熔断器保护）
//  2. 如果 Redis 不可用（熔断或错误），降级到 MySQL（通过 MySQL 熔断器 + 写信号量保护）
//  3. 首次点赞时：
//     a. 同步写入 MySQL VideoLike 表（用户点赞记录，需要强一致）
//     b. MySQL 写入成功后，通过 RabbitMQ 发布点赞数 +1 增量消息
//     c. 更新 Redis 中视频动态缓存的 likes_count +1
//
// 关键设计决策：
//
// 为什么用 SADD 返回值而非 SISMEMBER + SADD：
//   - SISMEMBER + SADD 两步非原子，并发请求可能同时通过 SISMEMBER 检查
//   - SADD 本身是原子的：返回 1=新增 member，0=已存在
//   - 用 SADD 返回值判断是否首次点赞，天然解决并发竞态
//
// 为什么 CreateVideoLike 需要返回 created 标记：
//   - SET TTL 过期后，SADD 返回 added=true（以为首次点赞），但 MySQL 可能已有记录
//   - FirstOrCreate 幂等不报错，但如果不区分"真正新建"和"已存在"，会多发 MQ 增量
//   - 通过 RowsAffected 判断：created=true 才发 MQ，created=false 跳过
//   - 这是 B 站等大规模系统的常见做法：Redis 是快速判重，MySQL 是最终一致性兜底
//
// 为什么点赞数用 MQ 异步批量写入：
//   - 短时间内多个用户可能点赞同一个视频，聚合后一次 UPDATE likes_count + N 更高效
//   - 点赞数是弱一致性数据，延迟 3 秒对用户体验影响极小
//
// 为什么 MySQL 写入失败时跳过 MQ 发布：
//   - 如果 MySQL 写入失败但 MQ 仍然发布，会导致 likes_count 虚高
//   - MySQL 中没有点赞记录但计数被加，且无法通过对账修复
//
// 为什么 MySQL 降级路径需要回填 Redis SET：
//   - 降级路径成功后，Redis SET 仍然是空的
//   - 如果不回填，下次请求走 Redis → SET 不存在 → SADD 返回 added=true → 重复发 MQ
//   - 回填后 Redis SET 恢复可用，后续请求正常走 Redis 快速路径
func (i *InteractionLogic) LikeVideo(ctx context.Context, req request.InteractionVideo) error {
	userID := req.UserID
	videoID := req.VideoID

	// ---- 第 1 步：Redis SADD 原子判重（通过 Redis 熔断器保护） ----
	// SADD 返回 added=true 表示本次新增（首次点赞），added=false 表示已点赞（幂等返回）
	// 相比 SISMEMBER + SADD 两步操作，SADD 本身是原子的，天然解决并发竞态
	var added bool
	redisErr := i.deps.Breakers.Redis.Execute(func() error {
		var err error
		added, err = i.deps.InteractionCacheRepo.AddUserToVideoLikedSet(ctx, userID, videoID)
		return err
	})

	if redisErr != nil {
		// Redis 不可用（熔断开启或执行错误），降级到 MySQL
		if errors.Is(redisErr, breaker.ErrCircuitOpen) {
			i.deps.Logger.Warn("Redis 熔断器开启，点赞判重降级到 MySQL",
				zap.String("user_id", userID), zap.Uint("video_id", videoID))
		} else {
			i.deps.Logger.Warn("Redis 点赞判重失败，降级到 MySQL",
				zap.String("user_id", userID), zap.Uint("video_id", videoID), zap.Error(redisErr))
		}

		// ---- 降级路径：MySQL 熔断器 + 写信号量保护 ----
		// 信号量：限制并发写请求数，防止超出连接池容量
		if err := i.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); err != nil {
			i.deps.Logger.Warn("MySQL 写信号量获取失败，点赞请求被限流",
				zap.String("user_id", userID), zap.Uint("video_id", videoID), zap.Error(err))
			return fmt.Errorf("点赞服务繁忙，请稍后重试")
		}
		defer i.deps.Breakers.MySQLWriteSem.Release(1)

		// 熔断器：MySQL 持续不可用时快速失败，防止请求超时堆积
		var created bool
		mysqlErr := i.deps.Breakers.MySQL.Execute(func() error {
			var err error
			created, err = i.deps.BackfillRepo.FallbackCheckLike(ctx, userID, videoID)
			return err
		})
		if mysqlErr != nil {
			if errors.Is(mysqlErr, breaker.ErrCircuitOpen) {
				i.deps.Logger.Warn("MySQL 熔断器开启，点赞降级路径不可用",
					zap.String("user_id", userID), zap.Uint("video_id", videoID))
			} else {
				i.deps.Logger.Error("MySQL 降级点赞失败",
					zap.String("user_id", userID), zap.Uint("video_id", videoID), zap.Error(mysqlErr))
			}
			return fmt.Errorf("点赞服务暂时不可用，请稍后重试")
		}

		// created=false 表示记录已存在（幂等），无需发 MQ 增量
		// 场景：用户之前已点赞，但 Redis SET 过期导致降级到 MySQL
		if !created {
			return nil
		}

		// MySQL 降级路径中真正新建了点赞记录，发布 MQ 增量 + 回填 Redis SET
		i.publishLikeIncrement(ctx, videoID)
		i.backfillLikeCache(ctx, userID, videoID)
		return nil
	}

	// Redis 正常路径：added=false 表示已点赞（幂等返回）
	if !added {
		return nil
	}

	// ---- 第 2 步：同步写入 MySQL VideoLike 表 ----
	// 用户点赞记录需要强一致，用于后续查询和通知推送
	// MySQL 写操作也需要熔断器 + 写信号量保护
	if err := i.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); err != nil {
		i.deps.Logger.Warn("MySQL 写信号量获取失败",
			zap.String("user_id", userID), zap.Uint("video_id", videoID), zap.Error(err))
		return fmt.Errorf("点赞服务繁忙，请稍后重试")
	}
	defer i.deps.Breakers.MySQLWriteSem.Release(1)

	var created bool
	if err := i.deps.Breakers.MySQL.Execute(func() error {
		var err error
		created, err = i.deps.InteractionRepo.CreateVideoLike(ctx, userID, videoID)
		return err
	}); err != nil {
		if errors.Is(err, breaker.ErrCircuitOpen) {
			i.deps.Logger.Warn("MySQL 熔断器开启，点赞写入不可用",
				zap.String("user_id", userID), zap.Uint("video_id", videoID))
		} else {
			i.deps.Logger.Error("MySQL 写入点赞记录失败",
				zap.String("user_id", userID), zap.Uint("video_id", videoID), zap.Error(err))
		}
		// MySQL 写入失败时返回错误，让用户稍后重试
		// 不继续发布 MQ 增量，避免 likes_count 虚高（无记录但计数被加）
		// 修复：MySQL 写入失败时回滚 Redis SET 中的 SADD 操作，
		// 防止 SADD 成功但 MySQL 失败导致的数据不一致：
		// Redis 认为用户已点赞，但 MySQL 无记录，后续重试时 SADD 返回 added=false 被误判为已点赞
		if rollbackErr := i.deps.InteractionCacheRepo.RemoveUserFromVideoLikedSet(ctx, userID, videoID); rollbackErr != nil {
			i.deps.Logger.Error("回滚 Redis SET 失败，可能存在数据不一致",
				zap.String("userID", userID), zap.Uint("videoID", videoID), zap.Error(rollbackErr))
		}
		return fmt.Errorf("点赞失败，请稍后重试")
	}

	// created=false：SET 过期后 SADD 返回 added=true，但 MySQL 中记录已存在
	// 不发 MQ 增量，避免 likes_count 虚高
	// 场景：SET TTL 到期 → 用户再次点赞 → SADD 返回 added=true → MySQL FirstOrCreate 发现已存在
	if !created {
		i.deps.Logger.Info("SET 过期后重复点赞，MySQL 幂等跳过 MQ 增量",
			zap.String("user_id", userID), zap.Uint("video_id", videoID))
		return nil
	}

	// ---- 第 3、4 步：发布 MQ 增量 + 更新 Redis 缓存 ----
	i.publishLikeIncrement(ctx, videoID)
	return nil
}

// publishLikeIncrement 点赞成功后的后续操作：发布 MQ 增量 + 更新 Redis 缓存
// 仅在 created=true（真正新建了 MySQL 记录）时调用
func (i *InteractionLogic) publishLikeIncrement(ctx context.Context, videoID uint) {
	// 通过 RabbitMQ 发布点赞数增量
	// worker 消费后延迟 3 秒批量聚合写入 MySQL videos.likes_count
	// 仅在 MySQL 写入成功后才发布，保证数据一致性
	if err := i.deps.VideoLikeCountPublisher.PublishLikeIncrement(ctx, videoID, 1); err != nil {
		i.deps.Logger.Warn("RabbitMQ 发布点赞增量失败",
			zap.Uint("video_id", videoID), zap.Error(err))
		// MQ 发布失败不影响点赞操作，增量数据会在下次发布时重试
	}

	// 更新 Redis 视频动态缓存中的 likes_count +1
	// 使后续请求能立即看到更新后的点赞数
	if err := i.deps.VideoCacheRepo.IncrementLikeCount(ctx, videoID); err != nil {
		i.deps.Logger.Warn("Redis 更新视频点赞数缓存失败",
			zap.Uint("video_id", videoID), zap.Error(err))
	}
}

// backfillLikeCache 降级路径成功后回填 Redis 点赞 SET
// 使后续请求能正常走 Redis 快速路径，避免每次都降级到 MySQL
func (i *InteractionLogic) backfillLikeCache(ctx context.Context, userID string, videoID uint) {
	// 尝试将用户加入 Redis 点赞 SET（可能 Redis 仍然不可用，但尝试一下）
	// 不通过熔断器：降级路径已经确认 MySQL 成功，回填失败不影响业务
	if _, err := i.deps.InteractionCacheRepo.AddUserToVideoLikedSet(ctx, userID, videoID); err != nil {
		i.deps.Logger.Warn("降级路径回填 Redis 点赞 SET 失败",
			zap.String("user_id", userID), zap.Uint("video_id", videoID), zap.Error(err))
	}
}

// UnlikeVideo 取消点赞视频
//
// 流程：
//  1. Redis SREM 从点赞 SET 中移除用户（如果存在）
//  2. MySQL DeleteVideoLike 删除点赞记录
//  3. 仅在真正删除时发布点赞数 -1 增量 + 更新视频缓存 likes_count -1
//
// 与 LikeVideo 对称：失败时仅 Warn 不阻塞，保证幂等
func (i *InteractionLogic) UnlikeVideo(ctx context.Context, req request.InteractionVideo) error {
	userID := req.UserID
	videoID := req.VideoID

	// 1. Redis SREM 移除（即使不存在也不报错）
	_ = i.deps.Breakers.Redis.Execute(func() error {
		return i.deps.InteractionCacheRepo.RemoveUserFromVideoLikedSet(ctx, userID, videoID)
	})

	// 2. MySQL 删除
	if err := i.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); err != nil {
		return fmt.Errorf("点赞服务繁忙，请稍后重试")
	}
	defer i.deps.Breakers.MySQLWriteSem.Release(1)

	var deleted bool
	if err := i.deps.Breakers.MySQL.Execute(func() error {
		var err error
		deleted, err = i.deps.InteractionRepo.DeleteVideoLike(ctx, userID, videoID)
		return err
	}); err != nil {
		return fmt.Errorf("取消点赞失败，请稍后重试")
	}

	// deleted=false：原本未点赞，幂等返回
	if !deleted {
		return nil
	}

	// 3. 发布 -1 增量 + 更新缓存
	if err := i.deps.VideoLikeCountPublisher.PublishLikeIncrement(ctx, videoID, -1); err != nil {
		i.deps.Logger.Warn("RabbitMQ 发布取消点赞增量失败",
			zap.Uint("video_id", videoID), zap.Error(err))
	}
	if err := i.deps.VideoCacheRepo.DecrementLikeCount(ctx, videoID); err != nil {
		i.deps.Logger.Warn("Redis 更新视频点赞数缓存失败",
			zap.Uint("video_id", videoID), zap.Error(err))
	}
	return nil
}

// FavoriteVideo 收藏视频到指定收藏夹
func (i *InteractionLogic) FavoriteVideo(ctx context.Context, userID string, req request.FavoriteVideoReq) error {
	if err := i.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); err != nil {
		return fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer i.deps.Breakers.MySQLWriteSem.Release(1)

	var created bool
	if err := i.deps.Breakers.MySQL.Execute(func() error {
		var err error
		created, err = i.deps.FavoriteRepo.AddFavorite(ctx, userID, req.VideoID, req.FolderID)
		return err
	}); err != nil {
		return fmt.Errorf("收藏失败，请稍后重试")
	}

	// 仅在真正新增收藏时更新 fav_count
	if created {
		if err := i.deps.VideoRepo.IncrementFavCount(ctx, req.VideoID, 1); err != nil {
			i.deps.Logger.Warn("更新视频 fav_count 失败",
				zap.Uint("video_id", req.VideoID), zap.Error(err))
		}
		// 同步更新 Redis 视频动态缓存中的 fav_count
		_ = i.deps.Breakers.Redis.Execute(func() error {
			return i.deps.VideoCacheRepo.IncrementFavCount(ctx, req.VideoID)
		})
	}
	return nil
}

// UnfavoriteVideo 取消收藏视频
// FolderID=0 表示从所有收藏夹移除；非 0 表示仅从指定收藏夹移除
func (i *InteractionLogic) UnfavoriteVideo(ctx context.Context, userID string, req request.UnfavoriteVideoReq) error {
	if err := i.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); err != nil {
		return fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer i.deps.Breakers.MySQLWriteSem.Release(1)

	if req.FolderID != 0 {
		// 仅从指定收藏夹移除，可能用户在其他收藏夹中还有该视频
		var deleted bool
		if err := i.deps.Breakers.MySQL.Execute(func() error {
			var err error
			deleted, err = i.deps.FavoriteRepo.RemoveFromFolder(ctx, userID, req.VideoID, req.FolderID)
			return err
		}); err != nil {
			return fmt.Errorf("取消收藏失败")
		}
		// 仅在真正删除时才减计数，避免重复取消收藏导致 fav_count 变为负数
		if deleted {
			if err := i.deps.VideoRepo.DecrementFavCount(ctx, req.VideoID, 1); err != nil {
				i.deps.Logger.Warn("更新视频 fav_count 失败",
					zap.Uint("video_id", req.VideoID), zap.Error(err))
			}
			_ = i.deps.Breakers.Redis.Execute(func() error {
				return i.deps.VideoCacheRepo.DecrementFavCount(ctx, req.VideoID)
			})
		}
		return nil
	}

	// FolderID=0：从所有收藏夹移除
	var deleted bool
	if err := i.deps.Breakers.MySQL.Execute(func() error {
		var err error
		deleted, err = i.deps.FavoriteRepo.RemoveFavorite(ctx, userID, req.VideoID)
		return err
	}); err != nil {
		return fmt.Errorf("取消收藏失败")
	}
	if deleted {
		if err := i.deps.VideoRepo.DecrementFavCount(ctx, req.VideoID, 1); err != nil {
			i.deps.Logger.Warn("更新视频 fav_count 失败",
				zap.Uint("video_id", req.VideoID), zap.Error(err))
		}
		// 同步更新 Redis 视频动态缓存中的 fav_count
		_ = i.deps.Breakers.Redis.Execute(func() error {
			return i.deps.VideoCacheRepo.DecrementFavCount(ctx, req.VideoID)
		})
	}
	return nil
}

// FollowUser 关注用户
// 不允许关注自己；幂等：已关注不报错
func (i *InteractionLogic) FollowUser(ctx context.Context, followerID, followeeID string) error {
	if followerID == followeeID {
		return errors.New("不能关注自己")
	}

	if err := i.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); err != nil {
		return fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer i.deps.Breakers.MySQLWriteSem.Release(1)

	var created bool
	if err := i.deps.Breakers.MySQL.Execute(func() error {
		var err error
		created, err = i.deps.InteractionRepo.CreateFollow(ctx, followerID, followeeID)
		return err
	}); err != nil {
		return fmt.Errorf("关注失败，请稍后重试")
	}

	// 仅在真正新建关注时增加被关注者的 fans_count
	// 实际 accounts 表没有 fans_count 字段，fans 数通过 GetFansCount 实时查询 user_follows 表
	// 这里 created 仅用于决定是否发送通知
	if created {
		// 通知被关注者（可选：复用 Notification 表）
		_ = i.notifyFollow(ctx, followerID, followeeID)
	}
	return nil
}

// UnfollowUser 取关用户
func (i *InteractionLogic) UnfollowUser(ctx context.Context, followerID, followeeID string) error {
	if followerID == followeeID {
		return errors.New("不能取关自己")
	}

	if err := i.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); err != nil {
		return fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer i.deps.Breakers.MySQLWriteSem.Release(1)

	if err := i.deps.Breakers.MySQL.Execute(func() error {
		_, err := i.deps.InteractionRepo.DeleteFollow(ctx, followerID, followeeID)
		return err
	}); err != nil {
		return fmt.Errorf("取关失败，请稍后重试")
	}
	return nil
}

// notifyFollow 发送关注通知（best-effort，失败不影响业务）
func (i *InteractionLogic) notifyFollow(ctx context.Context, followerID, followeeID string) error {
	// TODO: 复用 NotificationRepo 写入 type="new_follower" 通知
	// 暂不实现，避免引入 NotificationRepo 依赖
	_ = followerID
	_ = followeeID
	return nil
}
