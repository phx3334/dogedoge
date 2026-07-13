package redis

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"fake_tiktok/internal/repository/interfaces"

	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var _ interfaces.DanmakuPubSub = (*DanmakuPubSubRepo)(nil)

// danmakuSub 管理单个 videoID 的 Redis Pub/Sub 订阅
//
// 设计要点：
//   - 同一 videoID 的所有客户端共享一个 Redis Pub/Sub 连接（ds.sub）
//   - 每个客户端有独立的 listener channel，互不影响
//   - 转发 goroutine 从 Redis Pub/Sub 读取消息，分发给所有 listener
//   - 所有 listener 移除后，转发 goroutine 自动退出并清理资源
//   - Redis 连接断开时自动重连，重连期间消息可能丢失（可接受，弹幕是弱一致性数据）
type danmakuSub struct {
	sub       *goredis.PubSub     // Redis Pub/Sub 连接，同一 videoID 共享
	listeners []chan []byte       // 所有客户端的消息通道列表
	mu        sync.Mutex          // 保护 listeners 和 sub 的并发访问
	done      chan struct{}       // 通知转发 goroutine 退出（所有 listener 都离开了）
	closeOnce sync.Once           // 防止 close(ds.done) 被重复调用导致 panic
	cancelReconnect context.CancelFunc // 取消重连 goroutine
}

// DanmakuPubSubRepo 基于 Redis Pub/Sub 的弹幕跨实例广播实现
//
// 架构说明：
//
//	┌─────────────┐     Publish      ┌──────────────────┐
//	│  实例 A      │ ──────────────► │  Redis Pub/Sub   │
//	│  ReadPump   │                  │  danmaku:room:123 │
//	└─────────────┘                  └────────┬─────────┘
//	                                           │
//	                              ┌────────────┼────────────┐
//	                              ▼            ▼            ▼
//	                         实例 A 转发    实例 B 转发    实例 C 转发
//	                              │            │            │
//	                              ▼            ▼            ▼
//	                         Hub 广播      Hub 广播      Hub 广播
//	                              │            │            │
//	                         客户端1,2     客户端3,4     客户端5,6
type DanmakuPubSubRepo struct {
	client        *RedisClient     // Redis 客户端封装（含 nil 保护）
	subscriptions sync.Map         // map[uint64]*danmakuSub，按 videoID 管理订阅
	logger        *zap.Logger      // 日志记录器
}

// NewDanmakuPubSubRepo 创建弹幕 Pub/Sub 仓储
func NewDanmakuPubSubRepo(client *RedisClient, logger *zap.Logger) *DanmakuPubSubRepo {
	return &DanmakuPubSubRepo{
		client: client,
		logger: logger,
	}
}

// Publish 发布弹幕消息到指定视频的 Redis Pub/Sub 频道
//
// 频道命名：{KeyPrefix}:danmaku:room:{videoID}
// 消息将被所有订阅了该频道的实例收到，实现跨实例弹幕同步。
//
// 注意：调用方应通过熔断器包装此调用，Redis 不可用时快速失败
func (r *DanmakuPubSubRepo) Publish(ctx context.Context, videoID uint64, msg []byte) error {
	// Redis 客户端 nil 保护：Client 为 nil 时返回 ErrRedisUnavailable
	// 避免在 nil Client 上调用 Publish 导致 panic
	if r.client == nil || r.client.Client == nil {
		return ErrRedisUnavailable
	}

	channel := r.client.BuildKey(DanmakuChannelPrefix, strconv.FormatUint(videoID, 10))
	return r.client.Client.Publish(ctx, channel, msg).Err()
}

// Subscribe 订阅指定视频的弹幕频道，返回独立的消息通道
//
// 每个调用者获得自己的 channel，互不影响。
// 底层共享同一个 Redis Pub/Sub 连接，由一个转发 goroutine 将消息分发给所有 listener。
// 当所有 listener 都通过 RemoveListener 移除后，转发 goroutine 自动退出并清理资源。
//
// 重连机制：
//   - Redis 连接断开时，转发 goroutine 检测到 sub.Channel() 关闭
//   - 启动后台重连 goroutine，每 5 秒尝试重新订阅
//   - 重连成功后，转发 goroutine 继续工作
//   - 重连期间消息可能丢失（弹幕是弱一致性数据，可接受）
func (r *DanmakuPubSubRepo) Subscribe(ctx context.Context, videoID uint64) (<-chan []byte, error) {
	// Redis 客户端 nil 保护
	if r.client == nil || r.client.Client == nil {
		return nil, ErrRedisUnavailable
	}

	channel := r.client.BuildKey(DanmakuChannelPrefix, strconv.FormatUint(videoID, 10))

	// 为当前调用者创建独立的消息通道（缓冲 256 条，避免慢客户端阻塞转发）
	msgCh := make(chan []byte, 256)

	// 获取或创建该 videoID 的共享订阅
	val, _ := r.subscriptions.LoadOrStore(videoID, &danmakuSub{
		done: make(chan struct{}),
	})
	ds := val.(*danmakuSub)

	ds.mu.Lock()
	// 如果还没有 Redis 订阅，创建一个并启动转发 goroutine
	if ds.sub == nil {
		sub := r.client.Client.Subscribe(ctx, channel)
		ds.sub = sub

		// 创建可取消的 context 用于控制重连 goroutine
		reconnectCtx, cancelReconnect := context.WithCancel(context.Background())
		ds.cancelReconnect = cancelReconnect

		// 启动转发 goroutine：从 Redis Pub/Sub 读取消息，分发给所有 listener
		go r.forwardMessages(ds, videoID, channel, reconnectCtx)
	}

	// 将当前调用者的 channel 加入 listener 列表
	ds.listeners = append(ds.listeners, msgCh)
	ds.mu.Unlock()

	return msgCh, nil
}

// forwardMessages 从 Redis Pub/Sub 读取消息并分发给所有 listener
//
// 退出条件：
//   - done channel 被关闭（所有 listener 都离开了）
//   - Redis Pub/Sub 连接关闭（触发重连）
//
// 重连机制：
//   - 检测到 sub.Channel() 关闭后，启动后台重连 goroutine
//   - 重连成功后重新开始转发
//   - 重连期间消息丢失（弹幕弱一致性，可接受）
func (r *DanmakuPubSubRepo) forwardMessages(ds *danmakuSub, videoID uint64, channel string, reconnectCtx context.Context) {
	for {
		select {
		case msg, ok := <-ds.sub.Channel():
			if !ok {
				// Redis Pub/Sub 连接关闭，启动重连
				r.logger.Warn("Redis Pub/Sub channel closed, starting reconnect",
					zap.Uint64("video_id", videoID))
				if r.reconnect(ds, videoID, channel, reconnectCtx) {
					// 重连成功，继续转发
					continue
				}
				// 重连失败（context 取消），退出
				return
			}
			payload := []byte(msg.Payload)
			ds.mu.Lock()
			snapshot := make([]chan []byte, len(ds.listeners))
			copy(snapshot, ds.listeners)
			ds.mu.Unlock()
			for _, ch := range snapshot {
				select {
				case ch <- payload:
				default:
					// listener 缓冲区满则跳过，避免阻塞转发 goroutine
					// 弹幕是弱一致性数据，丢失少量消息可接受
				}
			}

		case <-ds.done:
			// 所有 listener 都离开了，退出转发 goroutine
			return
		}
	}
}

// reconnect 尝试重新建立 Redis Pub/Sub 连接
//
// 策略：每 5 秒重试一次，直到成功或 context 被取消
// 返回 true 表示重连成功，false 表示 context 已取消（客户端已断开）
func (r *DanmakuPubSubRepo) reconnect(ds *danmakuSub, videoID uint64, channel string, ctx context.Context) bool {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// 客户端已断开，停止重连
			return false
		case <-ticker.C:
			// 检查是否还有 listener，没有则停止重连
			ds.mu.Lock()
			listenerCount := len(ds.listeners)
			ds.mu.Unlock()
			if listenerCount == 0 {
				return false
			}

			// 尝试重新订阅
			sub := r.client.Client.Subscribe(ctx, channel)
			if err := sub.Ping(ctx); err != nil {
				r.logger.Warn("Redis Pub/Sub reconnect failed, retrying",
					zap.Uint64("video_id", videoID), zap.Error(err))
				sub.Close()
				continue
			}

			// 重连成功，更新订阅
			ds.mu.Lock()
			ds.sub = sub
			ds.mu.Unlock()

			r.logger.Info("Redis Pub/Sub reconnected",
				zap.Uint64("video_id", videoID))
			return true
		}
	}
}

// Unsubscribe 取消订阅，关闭所有 listener 的消息通道
//
// 当所有 listener 都离开后，关闭底层 Redis Pub/Sub 连接。
// 通常在服务关闭或视频房间销毁时调用。
func (r *DanmakuPubSubRepo) Unsubscribe(ctx context.Context, videoID uint64) error {
	channel := r.client.BuildKey(DanmakuChannelPrefix, strconv.FormatUint(videoID, 10))

	val, ok := r.subscriptions.Load(videoID)
	if !ok {
		return nil
	}

	ds := val.(*danmakuSub)
	ds.mu.Lock()
	defer ds.mu.Unlock()

	// 关闭所有 listener 的 channel
	for _, ch := range ds.listeners {
		close(ch)
	}
	ds.listeners = nil

	// 通知转发 goroutine 退出
	ds.closeOnce.Do(func() { close(ds.done) })

	// 取消重连 goroutine
	if ds.cancelReconnect != nil {
		ds.cancelReconnect()
	}

	// 关闭底层 Redis 订阅
	if ds.sub != nil {
		if err := ds.sub.Unsubscribe(ctx, channel); err != nil {
			return fmt.Errorf("unsubscribe danmaku channel %s: %w", channel, err)
		}
		ds.sub = nil
	}

	// 从缓存中移除
	r.subscriptions.Delete(videoID)
	return nil
}

// RemoveListener 移除指定 listener 的 channel（客户端断开时调用）
//
// 当所有 listener 都移除后：
//  1. 关闭 done channel，通知转发 goroutine 退出（防止 goroutine 泄漏）
//  2. 取消重连 goroutine
//  3. 关闭底层 Redis Pub/Sub 连接
//  4. 从 subscriptions 缓存中移除
func (r *DanmakuPubSubRepo) RemoveListener(videoID uint64, targetCh <-chan []byte) error {
	val, ok := r.subscriptions.Load(videoID)
	if !ok {
		return nil
	}

	ds := val.(*danmakuSub)
	ds.mu.Lock()
	defer ds.mu.Unlock()

	// 从 listener 列表中移除并关闭目标 channel
	for i, ch := range ds.listeners {
		if ch == targetCh {
			close(ch)
			ds.listeners = append(ds.listeners[:i], ds.listeners[i+1:]...)
			break
		}
	}

	// 所有 listener 都离开了，清理底层订阅和转发 goroutine
	if len(ds.listeners) == 0 {
		// 通知转发 goroutine 退出，防止 goroutine 泄漏
		ds.closeOnce.Do(func() { close(ds.done) })

		// 取消重连 goroutine
		if ds.cancelReconnect != nil {
			ds.cancelReconnect()
		}

		if ds.sub != nil {
			channel := r.client.BuildKey(DanmakuChannelPrefix, strconv.FormatUint(videoID, 10))
			_ = ds.sub.Unsubscribe(context.Background(), channel)
			ds.sub = nil
		}
		r.subscriptions.Delete(videoID)
	}

	return nil
}

