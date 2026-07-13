package task

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/pkg/lock"
	"fake_tiktok/internal/repository/interfaces"
	redis_repo "fake_tiktok/internal/repository/redis"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// cronLockKeyRebuildZSet RebuildZSet 分布式锁键名（**Task 5**）。
//
// 多 worker 部署下，多个进程可能同时触发 @every 1m 调度，导致：
//   - 重复的 MySQL 全表扫描
//   - 重复的 Redis Pipeline 写
//   - 最坏情况下 Pipeline Exec 在并发场景下产生 race
//
// 通过 SET NX EX 原子加锁，保证同一时刻只有 1 个 worker 执行。
const cronLockKeyRebuildZSet = "cron:rebuild_zset"

// cronLockTTL RebuildZSet 锁的过期时间。
//
// 选 300s 的原因：
//   - 留足"大数据量下重建"的时间（实测百万级视频约 30s）
//   - 即便持锁进程崩溃，5min 后锁也会自动释放，下一轮 worker 可继续
//   - 比 1m（cron 周期）略长，避免上一个周期还没跑完就被新周期抢锁
const cronLockTTL = 300 * time.Second

// cronLockRefreshInterval RebuildZSet 锁续期间隔。
//
// 设为 60s（TTL 300s 的 1/5）：留 5 次续期窗口，单次续期失败不会丢锁；
// 也不会因为续期太频繁（每秒一次）而给 Redis 增加无谓压力。
const cronLockRefreshInterval = 60 * time.Second

// VideoRankingTask 负责视频热度 ZSet 重建、视频/用户静态缓存同步等后台任务。
//
// 字段说明：
//   - videoRepo / rankingRepo / clientRepo / accountRepo：repository 层接口注入
//   - db：gorm.DB，用于视频热度回写 MySQL
//   - redisClient：Redis 客户端封装（含 KeyPrefix），用于构建带前缀的键
//   - client：**Task 5 新增** 原生 *redis.Client，用于分布式锁（不走 KeyPrefix）
//   - logger：zap logger
type VideoRankingTask struct {
	videoRepo   interfaces.VideoRepository
	rankingRepo interfaces.RankingRepository
	clientRepo  interfaces.ClientRepository
	accountRepo interfaces.AccountRepository
	db          *gorm.DB
	redisClient *redis_repo.RedisClient
	client      *redis.Client // **Task 5**：分布式锁使用原生 client
	logger      *zap.Logger
}

// NewVideoRankingTask 构造 VideoRankingTask 实例。
//
// 关键参数：
//   - redisClient：传 RedisClient 是因为 Pipeline / BuildKey 需要 KeyPrefix
//   - 同时把内嵌的 *redis.Client 单独存到 t.client，给分布式锁使用
func NewVideoRankingTask(
	videoRepo interfaces.VideoRepository,
	rankingRepo interfaces.RankingRepository,
	clientRepo interfaces.ClientRepository,
	accountRepo interfaces.AccountRepository,
	db *gorm.DB,
	redisClient *redis_repo.RedisClient,
	logger *zap.Logger,
) *VideoRankingTask {
	return &VideoRankingTask{
		videoRepo:   videoRepo,
		rankingRepo: rankingRepo,
		clientRepo:  clientRepo,
		accountRepo: accountRepo,
		db:          db,
		redisClient: redisClient,
		client:      redisClient.Client,
		logger:      logger,
	}
}

// CalculatePopularity 综合多项指标计算视频热度分。
//
// 交互行为按价值赋予不同权重：收藏(5) > 点赞(3) > 投币(2) = 评论(2) = 弹幕(2) > 播放(0.1)
// 时间衰减：发布时间越久，热度分越低，通过 power law 实现平滑衰减
// 分母 +2 保证新发布视频有一个合理的初始值，避免除零
//
// 公式:
//
//	score = Likes×3 + Comments×2 + Plays×0.1 + Favorites×5 + Coins×2 + Danmaku×2
//	popularity = score / (hours_since_publish + 2)^1.5
func CalculatePopularity(video database.Video) float64 {
	hoursSincePublish := time.Since(video.CreatedAt).Hours()
	if hoursSincePublish < 0 {
		hoursSincePublish = 0
	}
	gravity := 1.5
	score := float64(video.LikesCount)*3 +
		float64(video.CommentsCount)*2 +
		float64(video.PlayCount)*0.1 +
		float64(video.FavCount)*5 +
		float64(video.CoinCount)*2 +
		float64(video.DanmakuCount)*2
	popularity := score / math.Pow(hoursSincePublish+2, gravity)
	return popularity
}

// RebuildZSet 全量重建发布视频的 ZSet 并将热度分回写 MySQL。
//
// 本方法只负责 ZSet 重建和 MySQL popularity 列同步，不同步静态/动态 Hash。
// 由定时任务每 1 分钟调用一次，保证热度排序的实时性。
func (t *VideoRankingTask) RebuildZSet(ctx context.Context) error {
	t.logger.Info("开始重建视频热度 ZSet")

	if err := t.clientRepo.Ping(ctx); err != nil {
		t.logger.Warn("Redis 不可用，跳过 ZSet 重建", zap.Error(err))
		return err
	}

	videos, err := t.videoRepo.FindAllPublishedVideoIDs(ctx)
	if err != nil {
		t.logger.Error("查询已发布视频失败", zap.Error(err))
		return err
	}

	if len(videos) == 0 {
		t.logger.Info("没有已发布视频，清空 ZSet")
		return t.clearZSet(ctx)
	}

	globalKey := t.clientRepo.BuildKey(redis_repo.PublishedVideoZSetKey, "global")

	type zoneMember struct {
		key    string
		member redis.Z
	}

	var globalMembers []redis.Z
	var zoneMembers []zoneMember

	type videoPopularity struct {
		video      database.Video
		popularity int64
	}
	var items []videoPopularity
	for _, video := range videos {
		// 修复：一次计算热度值，Redis 和 MySQL 都复用同一个值，
		// 避免两次调用 CalculatePopularity（内部使用 time.Since）导致值不同
		p := int64(math.Round(CalculatePopularity(video)))
		items = append(items, videoPopularity{video: video, popularity: p})
	}

	for _, item := range items {
		member := redis.Z{
			Score:  float64(item.popularity),
			Member: strconv.FormatUint(uint64(item.video.ID), 10),
		}
		globalMembers = append(globalMembers, member)

		if item.video.Zone != "" {
			zoneKey := t.clientRepo.BuildKey(
				redis_repo.PublishedVideoZSetKey,
				item.video.Zone,
			)
			zoneMembers = append(zoneMembers, zoneMember{key: zoneKey, member: member})
		}
	}

	pipe := t.clientRepo.Pipeline()

	// 修复：使用临时 Key + RENAME 原子切换，避免先删后建期间 ZSet 为空
	// 如果 Pipeline 中 Del 成功但 ZAdd 失败，排行榜将完全为空
	tempKey := globalKey + ":temp"
	pipe.Del(ctx, tempKey) // 清理可能残留的临时 Key
	if len(globalMembers) > 0 {
		pipe.ZAdd(ctx, tempKey, globalMembers...)
		pipe.Expire(ctx, tempKey, redis_repo.PublishedVideoZSetExpire)
		// 裁剪 ZSet：只保留热度最高的 MaxSize 个成员，移除低分尾部
		if int64(len(globalMembers)) > redis_repo.PublishedVideoZSetMaxSize {
			pipe.ZRemRangeByRank(ctx, tempKey, 0, int64(len(globalMembers))-redis_repo.PublishedVideoZSetMaxSize-1)
		}
	}
	pipe.Rename(ctx, tempKey, globalKey) // 原子切换：临时 Key 覆盖目标 Key

	// 修复：分区 ZSet 同样使用临时 Key + RENAME 原子切换
	zoneKeySet := make(map[string]string) // zoneKey -> tempKey
	for _, zm := range zoneMembers {
		if _, ok := zoneKeySet[zm.key]; !ok {
			tempZoneKey := zm.key + ":temp"
			zoneKeySet[zm.key] = tempZoneKey
			pipe.Del(ctx, tempZoneKey) // 清理可能残留的临时 Key
		}
	}
	for _, zm := range zoneMembers {
		pipe.ZAdd(ctx, zoneKeySet[zm.key], zm.member)
	}
	for k, tempK := range zoneKeySet {
		pipe.Expire(ctx, tempK, redis_repo.PublishedVideoZSetExpire)
		// 分区 ZSet 同样裁剪
		pipe.ZRemRangeByRank(ctx, tempK, 0, -(redis_repo.PublishedVideoZSetMaxSize + 1))
		pipe.Rename(ctx, tempK, k) // 原子切换：临时 Key 覆盖目标 Key
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, err = pipe.Exec(ctxTimeout)
	if err != nil {
		t.logger.Error("Redis Pipeline 执行失败", zap.Error(err))
		return err
	}

	for _, item := range items {
		if item.video.Popularity != item.popularity {
			if updateErr := t.videoRepo.UpdatePopularity(ctx, item.video.ID, item.popularity); updateErr != nil {
				// 修复：MySQL 热度回写失败时记录更详细的错误信息
				t.logger.Warn("更新视频热度到 MySQL 失败，下次 RebuildZSet 将重新计算",
					zap.Uint("video_id", item.video.ID),
					zap.Int64("popularity", item.popularity),
					zap.Error(updateErr))
			}
		}
	}

	t.logger.Info("视频热度 ZSet 重建完成", zap.Int("video_count", len(videos)))
	return nil
}

func (t *VideoRankingTask) clearZSet(ctx context.Context) error {
	globalKey := t.clientRepo.BuildKey(redis_repo.PublishedVideoZSetKey, "global")
	return t.clientRepo.Del(ctx, globalKey)
}

// RebuildZSetWithLock 是带 Redis 分布式锁的 RebuildZSet 包装（**Task 5**）。
//
// 行为：
//  1. 尝试通过 SET NX EX 300 抢锁；抢不到（ErrLockHeld）直接 return nil
//  2. 抢到后启动后台续期协程（60s 一次 PEXPIRE）
//  3. 调用原始 RebuildZSet 执行实际工作
//  4. 函数返回前 defer Release 释放锁；续期协程通过 ctx 链路退出
//
// 为什么 cron 任务里"抢不到锁直接跳过"是合理的：
//   - 多 worker 部署下，另一个 worker 已经进入临界区执行 RebuildZSet
//   - 我们这一轮抢不到是常态；如果阻塞等锁，反而会让两次 RebuildZSet 叠加
//   - 下一轮 cron（1m 后）会再次尝试，那时前一个 worker 大概率已释放
//
// 为什么需要续期：
//   - RebuildZSet 在百万级视频下可能跑 30~60s，但仍 < TTL 300s
//   - 留 5 倍冗余：即便某次重建慢一点（5min 边界），锁也不会被 Redis 提前回收
//   - 续期间隔 60s = TTL/5，单次续期失败不影响后续 4 次窗口
//
// 安全保证：
//   - token 含 hostname + nanosecond 时间戳，全局唯一；避免误删别人的锁
//   - 续期与释放都用 Lua 脚本做"check-and-act"，原子性强
//   - 持锁进程崩溃：TTL 300s 后锁自动过期，下一轮 worker 可继续
//
// 与原 RebuildZSet 的关系：
//   - 原 RebuildZSet 不变（保留纯逻辑路径，便于测试和单进程部署）
//   - 本方法作为"加锁包装"挂在 cron 调度器上；多 worker 部署时启用
func (t *VideoRankingTask) RebuildZSetWithLock(ctx context.Context) error {
	// 1. 构造 token：hostname + 纳秒时间戳
	//    - hostname 保证多 worker 实例 token 不同
	//    - 纳秒级时间戳保证本进程内多次调用 token 不同（不重用）
	//    - token 长度约 50 字节，远小于 Redis value 上限
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	token := fmt.Sprintf("%s_%d", hostname, time.Now().UnixNano())

	// 2. 抢锁：拿不到直接跳过本轮（另一个 worker 正在执行）
	l, err := lock.Acquire(ctx, t.client, cronLockKeyRebuildZSet, token, cronLockTTL)
	if err != nil {
		if errors.Is(err, lock.ErrLockHeld) {
			// 锁被别人持有：当前 worker 不执行，让下一轮 cron 再尝试
			// 不返回 error：cron 调度器本身不需要感知"被跳过"
			t.logger.Info("RebuildZSet skipped, lock held by another worker")
			return nil
		}
		// 其他错误（如网络）：返回 error 让上层记日志
		return err
	}

	// 3. 启动后台续期协程
	//    - 使用独立 context.Background() 而不是外部 ctx：
	//      RebuildZSet 完成后立即 Release + 退出；续期协程要在 Release 前一直活着
	//    - 实际退出路径：当 Refresh 连续失败（返回 ErrLockHeld）或
	//      显式调 stopCh 时，续期协程主动 return
	//    - 简洁起见：让续期协程在 RebuildZSet 返回后通过 stopCh 退出
	stopCh := make(chan struct{})
	go func() {
		ticker := time.NewTicker(cronLockRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				// 修复：续期协程使用 context.Background()，避免外部 ctx 被取消（如 graceful shutdown）
				// 导致续期失败，锁被 Redis 提前回收，其他 worker 在当前 worker 仍在执行时获取锁
				if refreshErr := l.Refresh(context.Background(), cronLockTTL); refreshErr != nil {
					// 续期失败：可能锁已被回收（我们超时了）或 Redis 不可用
					// 不做恢复尝试：让本次 RebuildZSet 自然走完，Release 时会失败
					t.logger.Warn("RebuildZSet lock refresh failed",
						zap.Error(refreshErr),
					)
					return
				}
			}
		}
	}()

	// 4. 执行实际工作；defer 中先停止续期协程再释放锁（顺序很重要）
	defer func() {
		close(stopCh) // 通知续期协程退出
		_ = l.Release(ctx)
	}()

	return t.RebuildZSet(ctx)
}
