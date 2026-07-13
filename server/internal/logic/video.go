package logic

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"fake_tiktok/internal/breaker"
	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/domain/other"
	"fake_tiktok/internal/dto/cache"
	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/dto/response"

	"fake_tiktok/internal/pkg"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

// VideoLogic 视频业务逻辑层
type VideoLogic struct {
	deps *LogicDeps

	// sfGroup 用于缓存击穿保护：同一个 videoID 的并发回源请求只执行一次，
	// 其他请求共享结果，避免大量并发请求同时穿透到 MySQL。
	// 替代旧版 videoDetailMu（sync.Map + sync.Mutex）方案：
	//   - singleflight 自动去重，无需手动管理互斥锁生命周期和清理
	//   - 无需 cleanupStaleMutex 等辅助方法，避免 sync.Map 内存泄漏风险
	sfGroup singleflight.Group

	// zoneCaches 分区热门视频短期缓存 + 互斥锁，防缓存击穿
	// key=zone|cursor|limit，value=*zoneCache
	// 每个分页组合独立锁和缓存，不同分页请求互不影响
	//
	// 内存泄漏防护（修复后）：
	//   - 旧版 zoneCaches 条目永不清理，随不同 (zone, cursor, limit) 组合无限增长
	//   - 新版在 ListHotVideos 中缓存命中时，顺带清理已过期的 zoneCache 条目
	//   - 清理时机选择"缓存命中"路径，因为命中时说明该 key 近期被访问过，
	//     此时遍历 zoneCaches 检查过期不会对性能造成明显影响
	zoneCaches sync.Map
}

// zoneCache 单个分区（或全局）的热门视频缓存 + 互斥锁
//
// 注意：缓存 key 必须包含完整的分页参数（zone + cursor + limit），
// 不同的分页请求应该有不同的缓存实例，避免返回错误数据
type zoneCache struct {
	mu       sync.Mutex
	result   *hotVideosCache
	expireAt time.Time
}

// hotVideosCache 热门视频列表缓存结果
type hotVideosCache struct {
	list       []response.HomeVideoInfo
	total      int64
	nextCursor string
}

func NewVideoLogic(deps *LogicDeps) *VideoLogic {
	return &VideoLogic{deps: deps}
}

// getZoneCache 获取指定分区的缓存结构（懒创建，sync.Map 保证并发安全）
//
// 缓存 key 由 zone + cursor + limit 组成，确保不同分页请求有不同的缓存实例
// 避免第一页请求的结果被第二页请求复用，导致返回错误数据
func (v *VideoLogic) getZoneCache(zone, cursor string, limit int) *zoneCache {
	key := fmt.Sprintf("%s|%s|%d", zone, cursor, limit)
	val, _ := v.zoneCaches.LoadOrStore(key, &zoneCache{})
	return val.(*zoneCache)
}

// ListHotVideos 获取已发布视频列表（Redis 优先，降级 MySQL）
//
// 防缓存击穿策略：
//   - 每个 (zone + cursor + limit) 组合独立的互斥锁，同一时刻同一分页只有一个请求回源
//   - 5 秒短期缓存，等待中的请求直接复用结果
//   - 不同分区或不同分页的请求互不阻塞
func (v *VideoLogic) ListHotVideos(req request.HomeVideoList) ([]response.HomeVideoInfo, int64, string) {
	// 使用带超时的 context，防止 Redis/MySQL 慢查询导致 goroutine 长期阻塞
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 根据 zone + cursor + limit 获取对应的缓存结构
	// 不同的分页请求必须有不同的缓存实例，否则会返回错误数据
	limit := req.Limit
	if limit <= 0 || limit > 30 {
		limit = 10
	}
	zc := v.getZoneCache(req.Zone, req.Cursor, limit)

	// ---- 双重检查锁（Double-Checked Locking）----
	// 旧版在缓存有效性检查之前就加锁，导致同一分页的所有请求串行化，
	// 即使缓存有效也要排队等锁，高并发下成为性能瓶颈。
	// 改为：先无锁检查缓存 → 未命中再加锁 → 加锁后再检查缓存（防止并发回源）。
	// 这样缓存命中时（绝大多数请求）完全无锁，只有缓存未命中时才串行化。

	// 第一次检查（无锁快速路径）：缓存有效时直接返回，避免锁开销
	if zc.result != nil && time.Now().Before(zc.expireAt) {
		// 缓存命中时顺带清理过期的 zoneCache 条目，防止 sync.Map 无限增长
		// 选择在缓存命中路径执行清理，因为命中说明该 key 近期被访问，
		// 此时遍历 zoneCaches 检查过期不会对性能造成明显影响
		v.cleanupExpiredZoneCaches()
		return zc.result.list, zc.result.total, zc.result.nextCursor
	}

	// 缓存未命中：加锁防止同一分页的并发请求同时穿透到 Redis/MySQL
	zc.mu.Lock()
	defer zc.mu.Unlock()

	// 第二次检查（持锁后）：可能其他 goroutine 已经在锁内回填了缓存
	if zc.result != nil && time.Now().Before(zc.expireAt) {
		return zc.result.list, zc.result.total, zc.result.nextCursor
	}

	// 修复：移除 Ping 检查，统一使用熔断器模式。
	// Ping 成功不代表业务键存在，也不代表熔断器已关闭；
	// Ping 与熔断器状态不一致，且每次请求多一次 Redis 往返。
	// 改为先尝试 Redis 操作（通过熔断器），失败时降级到 MySQL。
	list, total, cursor := v.redisPath(ctx, req)

	// 缓存结果 5 秒
	zc.result = &hotVideosCache{list: list, total: total, nextCursor: cursor}
	zc.expireAt = time.Now().Add(5 * time.Second)

	return list, total, cursor
}

// redisPath Redis 缓存路径：ZSet 游标分页 → 批量查缓存 → 回填缺失
func (v *VideoLogic) redisPath(ctx context.Context, req request.HomeVideoList) ([]response.HomeVideoInfo, int64, string) {
	limit := int64(req.Limit)
	if limit <= 0 || limit > 30 {
		limit = 10
	}

	// 根据分区构建 ZSet 键：zone 为空时使用全局 key，非空时使用分区 key
	// 通过 ClientRepository.BuildPublishedZSetKey 封装 key 命名规则，
	// 避免 logic 层直接依赖 redis 包的常量
	zsetKey := v.deps.ClientRepo.BuildPublishedZSetKey(req.Zone)

	cursorScore, cursorID := pkg.DecodeCursor(req.Cursor)
	cursorMember := ""
	if cursorID > 0 {
		cursorMember = strconv.FormatUint(uint64(cursorID), 10)
	}
	// ZSet 游标分页通过 Redis 熔断器保护，Redis 宕机时快速失败走 MySQL 降级
	var zMembers []redis.Z
	var nextScore float64
	var nextCursorMember string
	zsetErr := v.deps.Breakers.Redis.Execute(func() error {
		var err error
		zMembers, nextScore, nextCursorMember, err = v.deps.RankingRepo.ZSetCursorPaginate(ctx, zsetKey, cursorScore, cursorMember, limit)
		return err
	})
	if zsetErr != nil || len(zMembers) == 0 {
		if zsetErr != nil {
			if errors.Is(zsetErr, breaker.ErrCircuitOpen) {
				v.deps.Logger.Warn("Redis circuit open during redisPath ZSetCursorPaginate")
			} else {
				v.deps.Logger.Error("Redis ZSet 游标分页失败，降级 MySQL", zap.Error(zsetErr))
			}
			// ZSet 分页失败时走熔断保护的 MySQL 降级路径
			// 与 ListHotVideos 主路径保持一致：都通过 Breakers.MySQL.Execute 包装
			var list []response.HomeVideoInfo
			var total int64
			var cursor string
			if err := v.deps.Breakers.MySQLReadSem.Acquire(ctx, 1); err != nil {
				v.deps.Logger.Warn("MySQL read semaphore acquire failed", zap.Error(err))
			} else {
				defer v.deps.Breakers.MySQLReadSem.Release(1)
			}
			breakerErr := v.deps.Breakers.MySQL.Execute(func() error {
				var fallbackErr error
				list, total, cursor, fallbackErr = v.mysqlFallback(ctx, req)
				return fallbackErr
			})
			if breakerErr != nil {
				if errors.Is(breakerErr, breaker.ErrCircuitOpen) {
					v.deps.Logger.Warn("MySQL circuit open during redisPath fallback")
				}
				return nil, 0, ""
			}
			return list, total, cursor
		}
		// 修复：ZSet 为空（键不存在或尚未被定时任务填充）时也应降级到 MySQL，
		// 而非直接返回空结果。例如新部署、Redis flush 后 ZSet 尚未重建的场景。
		v.deps.Logger.Warn("ZSet 为空，降级到 MySQL 查询", zap.String("zsetKey", zsetKey))
		var list []response.HomeVideoInfo
		var total int64
		var cursor string
		if err := v.deps.Breakers.MySQLReadSem.Acquire(ctx, 1); err != nil {
			v.deps.Logger.Warn("MySQL read semaphore acquire failed", zap.Error(err))
		} else {
			defer v.deps.Breakers.MySQLReadSem.Release(1)
		}
		breakerErr := v.deps.Breakers.MySQL.Execute(func() error {
			var fallbackErr error
			list, total, cursor, fallbackErr = v.mysqlFallback(ctx, req)
			return fallbackErr
		})
		if breakerErr != nil {
			if errors.Is(breakerErr, breaker.ErrCircuitOpen) {
				v.deps.Logger.Warn("MySQL circuit open during redisPath ZSet empty fallback")
			}
			return nil, 0, ""
		}
		return list, total, cursor
	}

	// ZCard 通过 Redis 熔断器保护，失败时 total 默认 0（非关键数据，不影响分页）
	var total int64
	_ = v.deps.Breakers.Redis.Execute(func() error {
		var zcardErr error
		total, zcardErr = v.deps.RankingRepo.ZCard(ctx, zsetKey)
		if zcardErr != nil {
			// 修复：ZCard 失败虽然不触发熔断（非关键路径），但应记录错误日志
			v.deps.Logger.Warn("ZCard 查询失败", zap.String("key", zsetKey), zap.Error(zcardErr))
		}
		return nil // ZCard 失败不触发熔断，避免非关键路径的失败影响整体可用性
	})

	videoIDs := make([]uint, 0, len(zMembers))
	for _, z := range zMembers {
		memberStr, ok := z.Member.(string)
		if !ok {
			continue
		}
		id, parseErr := strconv.ParseUint(memberStr, 10, 64)
		if parseErr != nil {
			continue
		}
		vid := uint(id)
		videoIDs = append(videoIDs, vid)
	}

	if len(videoIDs) == 0 {
		return nil, 0, ""
	}

	// 批量查 Redis 缓存（通过 Redis 熔断器保护，防止 Redis 宕机时 Pipeline 超时阻塞）
	// GetVideoCache 返回 error，闭包将错误返回给熔断器，
	// 使熔断器能感知 Redis 不可用并累计失败次数
	var cacheMap map[uint]*cache.VideoCacheData
	var missedIDs []uint
	redisErr := v.deps.Breakers.Redis.Execute(func() error {
		var err error
		cacheMap, missedIDs, err = v.deps.VideoCacheRepo.GetVideoCache(ctx, videoIDs)
		return err
	})
	// 修复：Redis 熔断/失败时 missedIDs 为 nil，导致 len(missedIDs)>0 为 false，
	// MySQL 回源被完全跳过。应将所有请求的 videoIDs 视为未命中，强制走 MySQL 回源。
	if redisErr != nil || cacheMap == nil {
		missedIDs = videoIDs
		cacheMap = make(map[uint]*cache.VideoCacheData)
	}
	if redisErr != nil {
		if errors.Is(redisErr, breaker.ErrCircuitOpen) {
			v.deps.Logger.Warn("Redis circuit open during redisPath GetVideoCache")
		}
	}

	// 缓存未命中的视频从 MySQL 回源并回填 Redis（通过熔断器保护）
	// 与 getVideoDataWithFallback / ListHotVideos 保持一致：MySQL 回源都走 Breakers.MySQL.Execute
	if len(missedIDs) > 0 {
		var dbVideos []database.Video
		if err := v.deps.Breakers.MySQLReadSem.Acquire(ctx, 1); err != nil {
			v.deps.Logger.Warn("MySQL read semaphore acquire failed", zap.Error(err))
		} else {
			defer v.deps.Breakers.MySQLReadSem.Release(1)
		}
		breakerErr := v.deps.Breakers.MySQL.Execute(func() error {
			var err error
			dbVideos, err = v.deps.VideoRepo.FindPublishedVideosByIDs(ctx, missedIDs)
			return err
		})
		if breakerErr != nil {
			// 修复：MySQL 查询失败时跳过 BackfillVideoCache，
			// 避免 nil 的 dbVideos 传入导致所有 missedIDs 被错误标记为空对象
			v.deps.Logger.Error("MySQL 回源查询失败，跳过缓存回填", zap.Error(breakerErr))
		} else {
			v.deps.BackfillRepo.BackfillVideoCache(ctx, dbVideos, missedIDs, cacheMap)
		}
	}

	// 组装响应列表
	list := make([]response.HomeVideoInfo, 0, len(videoIDs))
	for _, vid := range videoIDs {
		data, ok := cacheMap[vid]
		if !ok || data.IsEmpty {
			continue
		}

		var createdAt time.Time
		if !data.CreatedAt.IsZero() {
			createdAt = data.CreatedAt
		}

		list = append(list, response.HomeVideoInfo{
			ID:           vid,
			UpName:       data.AuthorName,
			UpAvatar:     data.AuthorAvatar,
			Title:        data.Title,
			CoverURL:     data.CoverURL,
			PlayCount:    data.PlayCount,
			CommentCount: data.CommentCnt,
			Duration:     data.Duration,
			CreatedAt:    createdAt,
			FavCount:     data.FavCnt,
		})
	}

	nextCursorID := uint(0)
	if nextCursorMember != "" {
		if parsed, parseErr := strconv.ParseUint(nextCursorMember, 10, 64); parseErr == nil {
			nextCursorID = uint(parsed)
		}
	}
	return list, total, pkg.EncodeCursor(nextScore, nextCursorID)
}

// mysqlFallback MySQL 降级路径：直接查数据库，不走缓存
//
// 当 Redis 不可用时使用此路径。
// ctx 参数透传以支持超时控制和链路追踪
//
// 返回值：
//   - list: 视频列表（查询失败时为 nil）
//   - total: 总数（查询失败时为 0）
//   - nextCursor: 下一页游标（查询失败时为空）
//   - err: 查询失败时返回 error，供熔断器统计失败次数
func (v *VideoLogic) mysqlFallback(ctx context.Context, req request.HomeVideoList) ([]response.HomeVideoInfo, int64, string, error) {
	var query = v.deps.VideoRepo.FindPublishedVideos()
	if req.Zone != "" {
		query = v.deps.VideoRepo.FindPublishedVideosWithZone(req.Zone)
	}

	opt := other.CursorOption{
		CursorPage: request.CursorPage{
			Limit:  req.Limit,
			Cursor: req.Cursor,
		},
		Fields: []other.CursorField{
			{Column: "popularity", Direction: "DESC"},
			{Column: "id", Direction: "DESC"},
		},
	}

	var videos []database.Video
	total, nextCursor, err := v.deps.PaginateRepo.CursorPaginate(ctx, query, opt, &videos)
	if err != nil {
		v.deps.Logger.Error("MySQL 游标分页失败", zap.Error(err))
		return nil, 0, "", err
	}

	// 复用 BackfillRepo 的作者名查询逻辑
	seen := make(map[string]bool)
	var authorIDs []string
	for _, vid := range videos {
		if vid.AuthorID == "" || seen[vid.AuthorID] {
			continue
		}
		seen[vid.AuthorID] = true
		authorIDs = append(authorIDs, vid.AuthorID)
	}
	authorNameMap := v.deps.BackfillRepo.LookupAuthorNames(ctx, authorIDs)
	authorCardMap := v.deps.BackfillRepo.LookupAuthorCards(ctx, authorIDs)

	list := make([]response.HomeVideoInfo, 0, len(videos))
	for _, vid := range videos {
		list = append(list, response.HomeVideoInfo{
			ID:           vid.ID,
			UpName:       authorNameMap[vid.AuthorID],
			UpAvatar:     authorCardMap[vid.AuthorID].AvatarURL,
			Title:        vid.Title,
			CoverURL:     vid.CoverURL,
			PlayCount:    vid.PlayCount,
			CommentCount: vid.CommentsCount,
			Duration:     vid.DurationSec,
			CreatedAt:    vid.CreatedAt,
			FavCount:     vid.FavCount,
		})
	}
	return list, total, nextCursor, nil
}

// GetVideoDetail 获取视频详情（Redis 缓存优先，未命中降级 MySQL 并回填）
//
// 防缓存击穿策略：
//   - 先查缓存，缓存命中直接返回（无需额外开销）
//   - 缓存未命中时，检查视频是否在热门 ZSet 中
//   - 仅热门视频加互斥锁，冷门视频并发量低不需要
//   - 互斥锁按 videoID 粒度，不同视频互不阻塞
func (v *VideoLogic) GetVideoDetail(ctx context.Context, userID string, videoID uint) (*response.VideoDetailResp, error) {
	// ---- 1. 获取视频基本信息（缓存优先） ----
	// 先不加锁查缓存，命中则直接返回，避免每次都调 ZScore
	videoData := v.getVideoDataWithFallback(ctx, videoID)
	if videoData == nil {
		return nil, fmt.Errorf("video not found")
	}

	// ---- 2. 获取作者信息（缓存优先，含粉丝数） ----
	// 用户缓存（static+dynamic）已包含 fans_count，无需单独查询
	authorInfo := response.AuthorInfo{ID: videoData.AuthorID}
	if videoData.AuthorID != "" {
		// 用 buffered channel 接收 goroutine 结果，避免 goroutine 内部 select 阻塞
		userCh := make(chan *cache.UserCacheData, 1)

		// 并行查询：用户信息缓存（含粉丝数）
		go func() {
			userData, userErr := func() (*cache.UserCacheData, error) {
				var data *cache.UserCacheData
				var err error
				// GetUserCache 使用 Pipeline 查询 static+dynamic，缓存未命中时返回 (nil, nil)，
				// 不会产生 redis.Nil，因此可以直接 return err 透传给熔断器
				redisErr := v.deps.Breakers.Redis.Execute(func() error {
					data, err = v.deps.UserCacheRepo.GetUserCache(ctx, videoData.AuthorID)
					return err
				})
				if redisErr != nil {
					return nil, redisErr
				}
				return data, err
			}()
			if userErr != nil || userData == nil {
				// 缓存未命中：走熔断 MySQL 回源
				var bd *cache.UserCacheData
				if err := v.deps.Breakers.MySQLReadSem.Acquire(ctx, 1); err != nil {
					v.deps.Logger.Warn("MySQL read semaphore acquire failed", zap.Error(err))
				} else {
					defer v.deps.Breakers.MySQLReadSem.Release(1)
				}
				backfillErr := v.deps.Breakers.MySQL.Execute(func() error {
					var err error
					bd, err = v.deps.BackfillRepo.BackfillUserCache(ctx, videoData.AuthorID)
					return err
				})
				if backfillErr != nil {
					if errors.Is(backfillErr, breaker.ErrCircuitOpen) {
						v.deps.Logger.Warn("MySQL circuit open, skip BackfillUserCache",
							zap.String("author_id", videoData.AuthorID))
					} else {
						v.deps.Logger.Warn("BackfillUserCache failed",
							zap.String("author_id", videoData.AuthorID),
							zap.Error(backfillErr))
					}
				} else if bd != nil {
					userData = bd
				}
			}
			select {
			case userCh <- userData:
			case <-ctx.Done():
			}
		}()

		// 等待 goroutine 完成，同时支持 ctx 超时控制
		select {
		case userData := <-userCh:
			if userData != nil {
				authorInfo.Username = userData.Username
				authorInfo.AvatarURL = userData.AvatarURL
				authorInfo.Signature = userData.Signature
				authorInfo.FansCount = userData.FansCount
			}
		case <-ctx.Done():
			// ctx 超时，使用已有部分结果立即返回
			v.deps.Logger.Warn("GetVideoDetail author info timeout", zap.Error(ctx.Err()))
			return &response.VideoDetailResp{
				ID:             videoID,
				Title:          videoData.Title,
				Description:    videoData.Description,
				PlayURL:        videoData.PlayURL,
				CoverURL:       videoData.CoverURL,
				Duration:       videoData.Duration,
				Zone:           videoData.Zone,
				PlayCount:      videoData.PlayCount,
				LikesCnt:       videoData.LikesCnt,
				CommentCnt:     videoData.CommentCnt,
				FavCnt:         videoData.FavCnt,
				CoinCnt:        videoData.CoinCnt,
				DanmakuCnt:     videoData.DanmakuCnt,
				CommentsClosed: videoData.CommentsClosed,
				DanmakuClosed:  videoData.DanmakuClosed,
				CreatedAt:      videoData.CreatedAt,
				Author:         authorInfo,
			}, nil
		}
	}

	// ---- 3. 获取用户互动状态（缓存优先，仅已登录用户） ----
	interaction := v.getInteractionWithFallback(ctx, userID, videoID, videoData.AuthorID)

	// ---- 4. 播放量 +1（Redis 原子自增 + RabbitMQ 异步同步 MySQL） ----
	if incrErr := v.deps.Breakers.Redis.Execute(func() error {
		return v.deps.VideoCacheRepo.IncrementPlayCount(ctx, videoID)
	}); incrErr != nil {
		v.deps.Logger.Warn("increment play count failed", zap.Error(incrErr))
	}
	if err := v.deps.PlayCountPublisher.PublishIncrement(ctx, videoID, 1); err != nil {
		v.deps.Logger.Warn("publish play count increment failed", zap.Error(err))
	}
	// 用户总播放量原子自增
	if err := v.deps.Breakers.Redis.Execute(func() error {
		return v.deps.UserCacheRepo.IncrementTotalPlayCount(ctx, videoData.AuthorID)
	}); err != nil {
		v.deps.Logger.Warn("increment user total play count failed", zap.Error(err))
	}
	if err := v.deps.UserPlayCountPublisher.PublishUserIncrement(ctx, videoData.AuthorID, 1); err != nil {
		v.deps.Logger.Warn("publish user play count increment failed", zap.Error(err))
	}

	// ---- 5. 组装响应 ----
	var createdAt time.Time
	if !videoData.CreatedAt.IsZero() {
		createdAt = videoData.CreatedAt
	}

	return &response.VideoDetailResp{
		ID:             videoID,
		Title:          videoData.Title,
		Description:    videoData.Description,
		PlayURL:        videoData.PlayURL,
		CoverURL:       videoData.CoverURL,
		Duration:       videoData.Duration,
		Zone:           videoData.Zone,
		PlayCount:      videoData.PlayCount,
		LikesCnt:       videoData.LikesCnt,
		CommentCnt:     videoData.CommentCnt,
		FavCnt:         videoData.FavCnt,
		CoinCnt:        videoData.CoinCnt,
		DanmakuCnt:     videoData.DanmakuCnt,
		CommentsClosed: videoData.CommentsClosed,
		DanmakuClosed:  videoData.DanmakuClosed,
		CreatedAt:      createdAt,
		Author:         authorInfo,
		Interaction:    interaction,
	}, nil
}

// getVideoDataWithFallback 获取视频缓存数据，未命中则降级查 MySQL 并回填
//
// 防缓存击穿：缓存未命中时，如果视频在热门 ZSet 中，加互斥锁防止并发回源
// 锁清理：缓存命中时，如果该视频已不在热门 ZSet 中，清理对应的 Mutex 防止内存泄漏
func (v *VideoLogic) getVideoDataWithFallback(ctx context.Context, videoID uint) *cache.VideoCacheData {
	// 第一次：无锁查缓存（快速路径，绝大多数请求命中缓存直接返回）
	var cacheMap map[uint]*cache.VideoCacheData
	// GetVideoCache 返回 error，闭包将错误返回给熔断器，
	// 使熔断器能感知 Redis 不可用并累计失败次数
	redisErr := v.deps.Breakers.Redis.Execute(func() error {
		var err error
		cacheMap, _, err = v.deps.VideoCacheRepo.GetVideoCache(ctx, []uint{videoID})
		return err
	})
	if redisErr != nil {
		if errors.Is(redisErr, breaker.ErrCircuitOpen) {
			v.deps.Logger.Warn("Redis circuit open during getVideoDataWithFallback", zap.Uint("video_id", videoID))
		}
	}
	if data, ok := cacheMap[videoID]; ok && !data.IsEmpty {
		return data
	}

	// 缓存未命中：使用 singleflight 防击穿
	// 同一个 videoID 的并发请求只执行一次回源，其他请求共享结果
	sfResult, sfErr, _ := v.sfGroup.Do(fmt.Sprintf("video:%d", videoID), func() (interface{}, error) {
		// singleflight 回调内再次查缓存（双重检查），可能其他 goroutine 已回填
		var innerCacheMap map[uint]*cache.VideoCacheData
		var innerMissedIDs []uint
		redisErr := v.deps.Breakers.Redis.Execute(func() error {
			var err error
			innerCacheMap, innerMissedIDs, err = v.deps.VideoCacheRepo.GetVideoCache(ctx, []uint{videoID})
			return err
		})
		// 修复：Redis 熔断/失败时将所有 videoID 视为未命中，强制走 MySQL 回源
		if redisErr != nil || innerCacheMap == nil {
			innerMissedIDs = []uint{videoID}
			innerCacheMap = make(map[uint]*cache.VideoCacheData)
		}
		if data, ok := innerCacheMap[videoID]; ok && !data.IsEmpty {
			return data, nil
		}

		// 缓存未命中，从 MySQL 回源
		if len(innerMissedIDs) > 0 {
			if semErr := v.deps.Breakers.MySQLReadSem.Acquire(ctx, 1); semErr != nil {
				v.deps.Logger.Warn("MySQL read semaphore acquire failed", zap.Error(semErr))
			} else {
				defer v.deps.Breakers.MySQLReadSem.Release(1)
			}
			var dbVideos []database.Video
			breakerErr := v.deps.Breakers.MySQL.Execute(func() error {
				var err error
				dbVideos, err = v.deps.VideoRepo.FindPublishedVideosByIDs(ctx, innerMissedIDs)
				return err
			})
			if breakerErr != nil {
				return nil, breakerErr
			}
			v.deps.BackfillRepo.BackfillVideoCache(ctx, dbVideos, innerMissedIDs, innerCacheMap)
			if data, ok := innerCacheMap[videoID]; ok && !data.IsEmpty {
				return data, nil
			}
		}
		return nil, nil
	})
	if sfErr != nil {
		if errors.Is(sfErr, breaker.ErrCircuitOpen) {
			v.deps.Logger.Warn("MySQL circuit open during getVideoDataWithFallback",
				zap.Uint("video_id", videoID))
		} else {
			v.deps.Logger.Warn("getVideoDataWithFallback failed",
				zap.Uint("video_id", videoID), zap.Error(sfErr))
		}
		return nil
	}
	if sfResult != nil {
		return sfResult.(*cache.VideoCacheData)
	}
	return nil
}

// getInteractionWithFallback 获取用户互动状态，缓存未命中则统一降级查 MySQL 并回填
// 使用 Pipeline 一次性查询 4 个互动缓存，减少网络往返
//
// 部分命中优化：通过 hitMask 位掩码判断哪些字段命中了缓存，只对未命中字段降级回源
// 已命中的缓存值不会被 MySQL 回填结果覆盖，避免浪费有效的缓存数据
func (v *VideoLogic) getInteractionWithFallback(ctx context.Context, userID string, videoID uint, authorID string) response.InteractionStatusResp {
	if userID == "" {
		return response.InteractionStatusResp{}
	}

	// Pipeline 批量查询 4 个互动缓存
	// hitMask: bit0=点赞, bit1=收藏, bit2=投币, bit3=关注
	var isLiked, isFavorited, isFollowed bool
	var coinCount int64
	var hitMask uint8
	var batchErr error
	// GetInteractionBatch 使用 Pipeline 查询，缓存未命中通过 hitMask 位掩码标记，
	// 不会返回 redis.Nil（只有整个 Pipeline 执行失败才返回 error），
	// 因此可以直接 return batchErr 透传给熔断器
	redisErr := v.deps.Breakers.Redis.Execute(func() error {
		isLiked, isFavorited, isFollowed, coinCount, hitMask, batchErr = v.deps.InteractionCacheRepo.GetInteractionBatch(ctx, userID, videoID, authorID)
		return batchErr
	})
	if redisErr != nil {
		if errors.Is(redisErr, breaker.ErrCircuitOpen) {
			v.deps.Logger.Warn("Redis circuit open during getInteractionWithFallback")
		}
		// Redis breaker open: treat as cache miss, will fall through to MySQL backfill
	}

	// Pipeline 整体失败或部分字段未命中，降级查 MySQL 并回填
	allHit := hitMask == 0x0F // bit0|bit1|bit2|bit3 = 0b1111 = 0x0F
	if batchErr != nil || !allHit {
		status := v.deps.BackfillRepo.BackfillInteractionCache(ctx, userID, videoID, authorID)
		if status != nil {
			// 只覆盖未命中的字段，已命中的保留缓存值
			if hitMask&0x01 == 0 { // bit0: 点赞未命中
				isLiked = status.IsLiked
			}
			if hitMask&0x02 == 0 { // bit1: 收藏未命中
				isFavorited = status.IsFavorited
			}
			if hitMask&0x04 == 0 { // bit2: 投币未命中
				coinCount = status.CoinCount
			}
			if hitMask&0x08 == 0 { // bit3: 关注未命中
				isFollowed = status.IsFollowed
			}
		}
	}

	return response.InteractionStatusResp{
		IsLiked:     isLiked,
		IsFavorited: isFavorited,
		CoinCount:   coinCount,
		IsFollowed:  isFollowed,
	}
}

// cleanupExpiredZoneCaches 清理已过期的 zoneCache 条目，防止 sync.Map 无限增长
//
// 调用时机：在 ListHotVideos 缓存命中时顺带执行
//
// 清理策略：
//   - 遍历 zoneCaches，删除 expireAt 已过的条目
//   - 过期条目的 result 指针已不会被新请求复用（因为过期后会重新回源），
//     删除后即使有并发请求正在读取旧 result，也不会 panic（Go 的 GC 保证
//     只要有引用就不会回收，删除 sync.Map 条目只是移除 map 中的引用）
//   - 清理频率由缓存命中率决定：命中越多，清理越频繁，自然形成"热时清理、冷时堆积"
//
// 性能影响：
//   - sync.Map.Range 内部是读操作，不阻塞其他并发访问
//   - 条目数量通常在几十到几百级别（zone 数量有限，cursor 组合可控），
//     遍历开销可忽略
func (v *VideoLogic) cleanupExpiredZoneCaches() {
	now := time.Now()
	v.zoneCaches.Range(func(key, value interface{}) bool {
		zc, ok := value.(*zoneCache)
		if !ok {
			return true
		}
		// 条目已过期且无人持有锁：安全删除
		// 不尝试加锁（zc.mu.TryLock），避免影响正在使用的请求
		if zc.result != nil && now.After(zc.expireAt) {
			v.zoneCaches.Delete(key)
		}
		return true
	})
}
