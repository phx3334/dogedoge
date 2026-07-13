package logic

import (
	"context"
	"encoding/json"
	"errors"
	"fake_tiktok/internal/breaker"
	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/dto/cache"
	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/dto/response"
	"fmt"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

// DanmakuLogic 弹幕业务逻辑层
type DanmakuLogic struct {
	deps *LogicDeps
	// sfGroup 用于弹幕缓存击穿保护：同一个 videoID 的并发回源请求只执行一次，
	// 其他请求共享结果，避免缓存失效瞬间大量请求穿透到 MySQL
	sfGroup singleflight.Group
}

func NewDanmakuLogic(deps *LogicDeps) *DanmakuLogic {
	return &DanmakuLogic{deps: deps}
}

// modeToType 将前端弹幕模式（0=滚动, 1=顶部, 2=底部）转换为 DB Type 字段
func modeToType(mode int) string {
	switch mode {
	case 1:
		return "top"
	case 2:
		return "bottom"
	default:
		return "scroll"
	}
}

// GetDanmakuList 获取视频弹幕列表
func (d *DanmakuLogic) GetDanmakuList(ctx context.Context, videoID uint64) ([]response.DanmakuItem, error) {
	// 检查弹幕是否已关闭
	// 修复：通过 Redis 熔断器保护 GetVideoCache 调用，
	// 避免 Redis 不可用时阻塞直到超时
	var videoMap map[uint]*cache.VideoCacheData
	_ = d.deps.Breakers.Redis.Execute(func() error {
		var err error
		videoMap, _, err = d.deps.VideoCacheRepo.GetVideoCache(ctx, []uint{uint(videoID)})
		return err
	})
	if vd, ok := videoMap[uint(videoID)]; ok && vd.DanmakuClosed {
		return []response.DanmakuItem{}, nil
	}

	// 判断是否热门视频，用于设置不同的缓存过期时间
	// 修复：通过 Redis 熔断器保护 IsHotVideo 调用
	var isHot bool
	_ = d.deps.Breakers.Redis.Execute(func() error {
		isHot = d.deps.RankingRepo.IsHotVideo(ctx, uint(videoID))
		return nil // IsHotVideo 失败不影响业务正确性，不触发熔断
	})

	var danmakus []*response.DanmakuItem
	var err error
	// 通过熔断器包装 Redis 缓存查询，透传错误让熔断器能统计 Redis 故障次数。
	// GetDanmakuCache 使用 ZRange 查询，缓存未命中时返回空切片 + nil error，
	// 不会产生 redis.Nil，因此可以直接 return err 而不会将缓存未命中误判为故障。
	redisErr := d.deps.Breakers.Redis.Execute(func() error {
		danmakus, err = d.deps.DanmakuCacheRepo.GetDanmakuCache(ctx, videoID)
		return err
	})
	if redisErr != nil {
		if errors.Is(redisErr, breaker.ErrCircuitOpen) {
			d.deps.Logger.Warn("Redis circuit open during GetDanmakuList", zap.Uint64("video_id", videoID))
		}
		// Redis breaker open: treat as cache miss, will fall through to MySQL backfill
	}
	// 检测弹幕缓存是否陈旧：SendDanmaku 的 Create 调用可能因 Redis 熔断/网络错误失败，
	// 导致新弹幕已写入 MySQL 且 DanmakuCnt 已自增，但弹幕缓存列表未更新。
	// 此时 DanmakuCnt > 缓存列表长度，需要回源 MySQL 获取最新弹幕。
	// 否则"活跃保活"机制会使陈旧缓存永不过期，其他用户无法看到新弹幕。
	var cachedDanmakuCount uint64
	if vd, ok := videoMap[uint(videoID)]; ok {
		cachedDanmakuCount = vd.DanmakuCnt
	}

	if err != nil || len(danmakus) == 0 || cachedDanmakuCount > uint64(len(danmakus)) {
		// 缓存未命中或陈旧：使用 singleflight 防击穿
		// 同一个 videoID 的并发回源请求只执行一次，其他请求共享结果，
		// 避免缓存失效瞬间大量请求穿透到 MySQL
		sfResult, sfErr, _ := d.sfGroup.Do(fmt.Sprintf("danmaku:%d", videoID), func() (interface{}, error) {
			// singleflight 回调内再次查缓存（双重检查），可能其他 goroutine 已回填
			var innerDanmakus []*response.DanmakuItem
			innerRedisErr := d.deps.Breakers.Redis.Execute(func() error {
				var err error
				innerDanmakus, err = d.deps.DanmakuCacheRepo.GetDanmakuCache(ctx, videoID)
				return err
			})
			// 双重检查：不仅检查缓存是否有数据，还要检查是否仍然陈旧
			// 如果只检查 len > 0，陈旧缓存（DanmakuCnt > 缓存列表长度）会被直接返回，
			// 跳过 MySQL 回填，导致其他用户看不到新发送的弹幕
			if innerRedisErr == nil && len(innerDanmakus) > 0 && cachedDanmakuCount <= uint64(len(innerDanmakus)) {
				// 其他 goroutine 已回填缓存且数据完整，直接返回
				d.deps.DanmakuCacheRepo.RefreshDanmakuCacheExpiry(ctx, videoID, isHot)
				return innerDanmakus, nil
			}
			// 信号量获取 MySQL 读取权限
			// 确认缓存未命中，回源 MySQL
			if semErr := d.deps.Breakers.MySQLReadSem.Acquire(ctx, 1); semErr != nil {
				d.deps.Logger.Warn("MySQL read semaphore acquire failed, skip BackfillDanmakuCache",
					zap.Uint64("video_id", videoID))
			} else {
				defer d.deps.Breakers.MySQLReadSem.Release(1)
			}
			var bd []*response.DanmakuItem
			backfillErr := d.deps.Breakers.MySQL.Execute(func() error {
				var err error
				bd, err = d.deps.BackfillRepo.BackfillDanmakuCache(ctx, videoID)
				return err
			})
			if backfillErr != nil {
				if errors.Is(backfillErr, breaker.ErrCircuitOpen) {
					d.deps.Logger.Warn("MySQL circuit open, skip BackfillDanmakuCache",
						zap.Uint64("video_id", videoID))
				}
				return nil, backfillErr
			}
			// 回填写入后刷新过期时间（热门 7d / 非热门 10min）
			d.deps.DanmakuCacheRepo.RefreshDanmakuCacheExpiry(ctx, videoID, isHot)
			return bd, nil
		})

		if sfErr != nil {
			// singleflight 回源失败（如 MySQL 熔断），返回空列表而非错误，
			// 弹幕是弱一致性数据，降级不阻塞用户观看
			d.deps.Logger.Warn("singleflight 回源弹幕缓存失败",
				zap.Uint64("video_id", videoID), zap.Error(sfErr))
		} else if result, ok := sfResult.([]*response.DanmakuItem); ok {
			danmakus = result
		}
	} else {
		// 缓存命中：刷新过期时间，实现"活跃保活"
		d.deps.DanmakuCacheRepo.RefreshDanmakuCacheExpiry(ctx, videoID, isHot)
	}

	items := make([]response.DanmakuItem, 0, len(danmakus))
	for _, dm := range danmakus {
		items = append(items, response.DanmakuItem{
			ID:        dm.ID,
			Content:   dm.Content,
			VideoTime: dm.VideoTime,
			Color:     dm.Color,
			FontSize:  dm.FontSize,
			Mode:      dm.Mode,
			UserID:    dm.UserID,
			CreatedAt: dm.CreatedAt,
		})
	}

	// 自愈：用实际弹幕列表长度校正 Redis 视频动态缓存中的 danmaku_count
	// 解决 IncrementDanmakuCount 的 MySQL/Redis 计数漂移问题
	// （如某次发送时 Redis HIncrBy 失败但 MySQL 成功，导致两者不一致）
	if len(items) > 0 {
		_ = d.deps.Breakers.Redis.Execute(func() error {
			return d.deps.VideoCacheRepo.SetDanmakuCount(ctx, uint(videoID), uint64(len(items)))
		})
	}

	return items, nil
}

// SendDanmaku 发送弹幕（写 MySQL + 广播 Pub/Sub）
func (d *DanmakuLogic) SendDanmaku(ctx context.Context, userID string, videoID uint, req request.SendDanmakuReq) error {
	// 检查弹幕是否已关闭
	// 修复：通过 Redis 熔断器保护 GetVideoCache 调用
	var videoMap map[uint]*cache.VideoCacheData
	_ = d.deps.Breakers.Redis.Execute(func() error {
		var err error
		videoMap, _, err = d.deps.VideoCacheRepo.GetVideoCache(ctx, []uint{videoID})
		return err
	})
	if vd, ok := videoMap[videoID]; ok && vd.DanmakuClosed {
		return fmt.Errorf("该视频已关闭弹幕")
	}

	danmaku := &database.Danmaku{
		VideoID:   req.VideoID,
		UserID:    userID,
		Content:   req.Content,
		VideoTime: req.VideoTime,
		Color:     req.Color,
		FontSize:  req.FontSize,
		// Type 字段在 DB 中为 NOT NULL 且无默认值，必须显式设置，否则 INSERT 失败
		// 导致弹幕创建报错，后续的计数自增逻辑不会执行，弹幕数量永远不增加
		Type:      modeToType(req.Mode),
		CreatedAt: time.Now(),
	}
	// FontSize 兜底：前端可能传空串，DB 字段 NOT NULL
	if danmaku.FontSize == "" {
		danmaku.FontSize = "md"
	}
	// Color 兜底：DB 字段 NOT NULL
	if danmaku.Color == "" {
		danmaku.Color = "#ffffff"
	}
	if semErr := d.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); semErr != nil {
		d.deps.Logger.Warn("MySQL write semaphore acquire failed", zap.Error(semErr))
	} else {
		defer d.deps.Breakers.MySQLWriteSem.Release(1)
	}
	// 修复：通过 MySQL 熔断器保护 Create 调用，
	// MySQL 持续不可用时快速失败，避免连接池耗尽和请求堆积
	var createErr error
	breakerErr := d.deps.Breakers.MySQL.Execute(func() error {
		createErr = d.deps.DanmakuRepo.Create(ctx, danmaku)
		return createErr
	})
	if breakerErr != nil {
		return fmt.Errorf("send danmaku failed: %w", breakerErr)
	}
	if createErr != nil {
		return fmt.Errorf("create danmaku failed: %w", createErr)
	}

	// 更新视频弹幕计数（MySQL + Redis 缓存）
	// 跟踪 Redis 操作是否失败：如果 Redis 计数自增或缓存写入任一失败，
	// 需要删除弹幕缓存 key，否则 DanmakuCnt 和缓存列表都保持旧值，
	// 陈旧检测（DanmakuCnt > len(cache)）无法触发，导致新弹幕虽已写入 MySQL
	// 但对其他用户不可见
	redisOpFailed := false
	if breakerErr := d.deps.Breakers.MySQL.Execute(func() error {
		return d.deps.VideoRepo.IncrementDanmakuCount(ctx, videoID, 1)
	}); breakerErr != nil {
		d.deps.Logger.Warn("IncrementDanmakuCount MySQL failed", zap.Uint("videoID", videoID), zap.Error(breakerErr))
	}
	if redisIncrErr := d.deps.Breakers.Redis.Execute(func() error {
		return d.deps.VideoCacheRepo.IncrementDanmakuCount(ctx, videoID)
	}); redisIncrErr != nil {
		d.deps.Logger.Warn("IncrementDanmakuCount Redis failed", zap.Uint("videoID", videoID), zap.Error(redisIncrErr))
		redisOpFailed = true
	}

	// 构造广播消息对象
	danmakuItem := response.DanmakuItem{
		ID:        danmaku.ID,
		Content:   danmaku.Content,
		VideoTime: danmaku.VideoTime,
		Color:     danmaku.Color,
		FontSize:  danmaku.FontSize,
		Mode:      req.Mode,
		UserID:    danmaku.UserID,
		CreatedAt: danmaku.CreatedAt.Unix(),
	}

	// 写入 Redis 弹幕缓存：保证发送后刷新页面能立即看到新弹幕
	// 失败只记录日志，不阻断发送成功响应
	if cacheErr := d.deps.Breakers.Redis.Execute(func() error {
		return d.deps.DanmakuCacheRepo.Create(ctx, req.VideoID, &danmakuItem)
	}); cacheErr != nil {
		d.deps.Logger.Warn("写入弹幕缓存失败", zap.Uint64("videoID", req.VideoID), zap.Error(cacheErr))
		redisOpFailed = true
	}

	// 如果 Redis 操作失败（计数自增或缓存写入），删除弹幕缓存 key
	// 强制下次 GetDanmakuList 从 MySQL 全量回填，确保新弹幕对其他用户可见
	if redisOpFailed {
		if delErr := d.deps.Breakers.Redis.Execute(func() error {
			return d.deps.DanmakuCacheRepo.DeleteDanmakuCache(ctx, req.VideoID)
		}); delErr != nil {
			d.deps.Logger.Warn("删除弹幕缓存失败", zap.Uint64("videoID", req.VideoID), zap.Error(delErr))
		}
	}

	// 广播弹幕到 Redis Pub/Sub（跨实例同步 + WebSocket 实时推送）
	// 修复：检查 json.Marshal 错误，避免发布 nil 消息到 Pub/Sub
	msg, marshalErr := json.Marshal(danmakuItem)
	if marshalErr != nil {
		d.deps.Logger.Error("序列化弹幕消息失败", zap.Error(marshalErr))
		// 弹幕已写入 MySQL，仅 Pub/Sub 广播失败，不影响数据正确性
	} else {
		if pubErr := d.deps.DanmakuPubSub.Publish(ctx, req.VideoID, msg); pubErr != nil {
			d.deps.Logger.Warn("弹幕 Pub/Sub 广播失败", zap.Uint64("videoID", req.VideoID), zap.Error(pubErr))
		}
	}

	return nil
}
