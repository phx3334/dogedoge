// Package rabbitmq 内的 publish_buffer.go：提供"连接断开时不丢消息"的有界缓冲发布能力。
//
// 设计动机：
//   - 直连 publish：MQ 抖动时调用方会拿到错误，可能选择丢弃 / 退避重试，但都
//     会带来业务方代码的复杂度。
//   - 缓冲 publish：publish 调用方永远拿到一个 nil 或 buffered 错误，业务
//     路径不变；MQ 恢复时由后台 drainLoop 自动重放。
//
// 丢最旧策略的取舍：
//   - 播放量增量是"可累加"的弱一致性数据；偶尔丢 1 条旧消息对业务影响很小。
//   - 阻塞 publish 路径会拖慢整个 API 响应时间；阻塞 + 无限增长又可能 OOM。
//   - 因此选"丢最旧"：保证 publish 永不阻塞、内存使用有上限、丢失的总是最旧数据。
package rabbitmq

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	amqp091 "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

// defaultBufferCapacity 是环形缓冲的容量上限。
// 10000 条消息对播放量增量场景够用约 30 分钟（按 5 msg/s 计）；
// 同时 10000 * 一条消息约 1KB ≈ 10MB 内存，对单机进程完全可接受。
const defaultBufferCapacity = 10000

// bufferedItem 缓冲内的一条待发消息。
type bufferedItem struct {
	exchange   string          // 目标 exchange
	routingKey string          // 路由键
	msg        amqp091.Publishing // amqp 消息体（内容 + headers + content-type 等）
	enqueuedAt time.Time       // 入队时间；用于后续分析延迟 / 调试
}

// ringBuffer 是有界 FIFO 缓冲；满时丢最旧（drop-oldest）。
//
// 为什么用 slice + drop-oldest 而不用 container/list：
//   - 简单的 FIFO 场景，slice 性能更好（连续内存）
//   - 容量固定后只需要 append / [1:] 操作
//   - dropped 计数用 atomic.Int64 暴露给监控
type ringBuffer struct {
	mu       sync.Mutex
	items    []bufferedItem
	capacity int
	// dropped 累计被丢弃的条数；publish 路径访问无需持锁
	dropped atomic.Int64
}

// newRingBuffer 构造一个指定容量的环形缓冲。
func newRingBuffer(capacity int) *ringBuffer {
	return &ringBuffer{capacity: capacity}
}

// push 推入一条新消息。返回 true 表示正常入队；false 表示缓冲已满，丢了最旧一条。
func (b *ringBuffer) push(it bufferedItem) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.items) >= b.capacity {
		// 满了：丢最旧（index 0）然后把新消息追加到末尾
		// [1:] 复制会 O(n)，但容量小（10000）代价可接受
		b.items = append(b.items[1:], it)
		b.dropped.Add(1)
		return false
	}
	b.items = append(b.items, it)
	return true
}

// drainAll 一次性取出所有消息并清空缓冲。
// 返回的 slice 所有权转移给调用方。
//
// 返回 nil 表示空；调用方应当 len(items)==0 判断而非判断 nil。
func (b *ringBuffer) drainAll() []bufferedItem {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.items) == 0 {
		return nil
	}
	items := b.items
	b.items = nil
	return items
}

// requeue 把 drain 出来但发不出去的消息重新塞回缓冲头部。
//
// 用 append(items, b.items...) 把未消费的部分拼接到当前缓冲的前面，
// 保证 FIFO 顺序：旧的先发、新的后发。
func (b *ringBuffer) requeue(items []bufferedItem) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.items = append(items, b.items...)
	// 修复：检查容量上限，避免极端场景下（如 broker 反复断连-重连）缓冲无限增长导致 OOM
	if len(b.items) > b.capacity {
		dropped := len(b.items) - b.capacity
		b.dropped.Add(int64(dropped))
		b.items = b.items[dropped:]
	}
}

// depth 返回当前缓冲中消息数（监控用）。
func (b *ringBuffer) depth() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.items)
}

// PublishBuffer 把 Connection 包装为"自动缓冲 + 自动重放"的发布器。
//
// 关键特性：
//   - Publish() 永不阻塞（最坏情况：丢最旧）
//   - 后台 drainLoop 每秒尝试重放缓冲
//   - Connection 关闭时 drainLoop 自动退出
type PublishBuffer struct {
	conn   *Connection
	buf    *ringBuffer
	logger *zap.Logger

	// drainDone drainLoop 退出时关闭；供测试或未来 Close 等待
	drainDone chan struct{}
}

// NewPublishBuffer 构造一个缓冲发布器（未启动 drainLoop）。
// 调用方需在合适时机调用 Start(ctx)。
func NewPublishBuffer(conn *Connection, logger *zap.Logger) *PublishBuffer {
	return &PublishBuffer{
		conn:      conn,
		buf:       newRingBuffer(defaultBufferCapacity),
		logger:    logger,
		drainDone: make(chan struct{}),
	}
}

// Start 启动后台 drainLoop。drainLoop 会在以下情况退出：
//   - ctx 取消
//   - Connection 的 runCtx 取消（用户主动 Close）
//
// 注意：drainLoop 是 fire-and-forget 的；如果它 panic，
// 当前实现不会重启——这是有意为之，避免在 broker 持续不可用时
// 进入"频繁 panic → 频繁重启"的恶性循环。
func (p *PublishBuffer) Start(ctx context.Context) {
	go p.drainLoop(ctx)
}

// Publish 尝试立刻发送消息；连接不可用时入缓冲。
//
// 行为（修复后）：
//  1. 先尝试非阻塞 Channel() 拿到 channel
//  2. 拿到后 PublishWithContext 成功 → 返回 nil
//  3. 拿不到或发送失败 → 入缓冲（缓冲满则丢最旧 + Warn）
//
// 修复说明：
//   - 旧版使用 WaitReady(ctx) 阻塞等待连接可用，在 API 请求路径中
//     如果 MQ 宕机，WaitReady 会持续轮询直到 ctx 超时，直接阻塞 API 响应
//   - 新版改为非阻塞 Channel()：连接不可用时立即入缓冲，不阻塞调用方
//   - 这与 PublishBuffer 的设计初衷一致——"Publish() 永不阻塞"
//   - drainLoop 中已经使用 IsReady() + Channel() 的非阻塞模式
//
// 返回值：永远返回入缓冲前的错误（如果有），调用方可以感知到
// "刚才是因为 MQ 断开才入的缓冲"——便于上层做指标上报。
func (p *PublishBuffer) Publish(ctx context.Context, exchange, routingKey string, msg amqp091.Publishing) error {
	// 非阻塞尝试获取 channel：如果连接不可用，立即走缓冲路径
	// 不使用 WaitReady 避免在 API 请求路径中阻塞
	ch, err := p.conn.Channel()
	if err == nil {
		if pubErr := ch.PublishWithContext(ctx, exchange, routingKey, false, false, msg); pubErr == nil {
			return nil
		} else {
			// PublishWithContext 失败（连接可能在发送过程中断开）
			err = pubErr
		}
	}
	// 入缓冲
	it := bufferedItem{exchange: exchange, routingKey: routingKey, msg: msg, enqueuedAt: time.Now()}
	if !p.buf.push(it) {
		// 满了：丢最旧；用 Warn 提示运维关注（QPS 异常 / broker 长时间不可用）
		p.logger.Warn("publish buffer full, dropped oldest",
			zap.Int("depth", p.buf.depth()),
			zap.Int64("total_dropped", p.buf.dropped.Load()))
	}
	return err
}

// DeclareQueue 在 broker 端幂等声明一个 durable 队列。
//
// 为什么发布方也要声明队列：
//   - 历史实现只在 worker（消费者）启动时声明 mini_bili_transcode 等队列
//   - 若 API 在 worker 尚未声明队列时就直发消息（默认 exchange "" + routingKey=队列名），
//     RabbitMQ 会"静默丢弃"该消息（路由不到任何队列）
//   - 结果：视频草稿永远停在 status=draft，前端轮询永远转圈、上传"一直加载"
//   - 发布方主动声明（durable + 与 worker 一致的参数）即可消除这个竞态窗口
//
// 调用时机：在每个需要保证可达的 Publish 之前调用一次。
//   - 连接未就绪时直接返回（消息会进入缓冲，由 drainLoop 重放；
//     届时 worker 通常已声明队列，或由本方法在重放前再次声明兜底）
//   - 声明是幂等的，重复调用无副作用，开销仅一次 RTT
func (p *PublishBuffer) DeclareQueue(queueName string) {
	ch, err := p.conn.Channel()
	if err != nil {
		// 连接未就绪：交给 drainLoop 在 broker 恢复后重放，无需在此阻塞
		return
	}
	// durable=true 与 worker 端 QueueDeclare 参数保持一致，避免属性冲突
	if _, err := ch.QueueDeclare(queueName, true, false, false, false, nil); err != nil {
		p.logger.Warn("publish buffer: declare queue failed",
			zap.String("queue", queueName), zap.Error(err))
	}
}

// drainLoop 是后台重放协程，每 1s 尝试把缓冲里的消息发出去。
//
// 退出条件：
//   - ctx 取消（业务整体退场）
//   - p.conn.runCtx 取消（Connection.Close）
//
// 为什么每 1s 而不是立刻持续重试：
//   - 持续重试在 broker 不可用时会浪费 CPU
//   - 1s 间隔在"消息可容忍最多 1s 延迟"和"CPU 节约"之间平衡
//
// 为什么用 Connection.IsReady() 而不直接 WaitReady：
//   - drainLoop 是周期任务，需要"非阻塞地"判断状态
//   - 用 IsReady 避免在 broker 不可用时阻塞 drainLoop 协程
func (p *PublishBuffer) drainLoop(ctx context.Context) {
	defer close(p.drainDone)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.conn.runCtx.Done():
			// Connection 已关闭，drainLoop 没必要继续
			return
		case <-ticker.C:
			// broker 还不可用：跳过本轮，等下一 tick
			if !p.conn.IsReady() {
				continue
			}
			items := p.buf.drainAll()
			if len(items) == 0 {
				continue
			}
			// 逐条重放：任意一条失败就把剩余的 requeue
			for i, it := range items {
				ch, err := p.conn.Channel()
				if err != nil {
					// broker 刚断开：剩余的全量 requeue 等下轮
					p.buf.requeue(items[i:])
					break
				}
				if err := ch.PublishWithContext(ctx, it.exchange, it.routingKey, false, false, it.msg); err != nil {
					p.logger.Warn("buffer drain publish failed, requeue", zap.Error(err))
					p.buf.requeue(items[i:])
					break
				}
			}
		}
	}
}
