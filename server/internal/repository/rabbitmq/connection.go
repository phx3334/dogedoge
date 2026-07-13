// Package rabbitmq 提供 RabbitMQ 弹性连接抽象：
//   - 自动重连（指数退避 1s→2s→4s→…→30s）
//   - NotifyClose 监听：连接被动断开时自动触发重连
//   - ready 状态原子标记：发布/消费方可无锁探测连接可用性
//   - 线程安全：所有 channel 访问都走 RWMutex 保护
//
// 典型使用：
//   conn := rabbitmq.NewConnection(&cfg.RabbitMQ, logger)
//   go conn.Run(ctx)              // 启动后台重连循环（必须异步）
//   ch, _ := conn.WaitReady(ctx)  // 阻塞直到连接可用
//   ch.PublishWithContext(...)
//   ...
//   conn.Close()                  // 主动停机：取消 ctx + 关 channel/conn
package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"fake_tiktok/internal/config"

	amqp091 "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

// ErrNotReady 表示连接尚未就绪（首次连接尚未成功 / 正在重连中 / 已主动关闭）。
// 调用方收到该错误后应当回退到"重试 / 入缓冲 / 等待重订阅"路径，**不应**作为致命错误。
var ErrNotReady = errors.New("rabbitmq connection not ready")

// 默认退避时间常量
const (
	// defaultInitialBackoff 首次重连失败后的等待时间；后续每次翻倍
	defaultInitialBackoff = 1 * time.Second
	// defaultMaxBackoff 退避上限；防止长时间不可用时把退避拉到几分钟
	defaultMaxBackoff = 30 * time.Second
)

// Connection 是 amqp091.Connection 的线程安全封装。
//
// 它把"建连 + 重连 + 暴露 channel"的职责集中起来，让上层 publish / consume
// 不用关心底层连接何时重建。所有字段对外只读；修改只能通过内部方法。
type Connection struct {
	// cfg RabbitMQ 连接配置（host/port/username/password）。
	// 创建后只读，无并发风险。
	cfg *config.RabbitMQConfig

	// logger 结构化日志器；记录连接建立 / 断开 / 重连等待。
	logger *zap.Logger

	// mu 保护 conn 和 channel 这两个 amqp 对象的并发读写。
	// Channel() / setChannel() / Close() 都会用到；用 RWMutex 是因为
	// "读 channel"的频率远高于"重连后写入新 channel"的频率。
	mu      sync.RWMutex
	conn    *amqp091.Connection
	channel *amqp091.Channel

	// ready 原子布尔：true 表示当前 channel 可用。
	// 与 mu 一起形成双重检查：ready=true 仅作为快速路径，channel 仍可能
	// 介于"已就绪"和"正在重连"之间的极小窗口返回 nil，因此 Channel() 仍
	// 会再做 nil 检查。
	ready atomic.Bool

	// closed 原子布尔：标记是否调用过 Close()。
	// Run 循环每轮迭代都会检查该字段以实现"主动退出"；
	// 用 atomic.Bool.Swap 实现 Close 的幂等性。
	closed atomic.Bool

	// initialBackoff 首次重连等待时间
	initialBackoff time.Duration
	// maxBackoff 退避上限
	maxBackoff time.Duration

	// runCtx / runCancel Run 启动时基于调用方 ctx 派生的内部 ctx。
	// Close() 通过 cancel() 主动终止重连循环。
	runCtx    context.Context
	runCancel context.CancelFunc

	// done Run 返回时关闭；Close() 用它等待 Run 完全退出再返回。
	done chan struct{}
}

// NewConnection 构造一个 Connection 实例。
//
// **不会**立即建连——真正的连接由 Run 在 goroutine 中发起。
// 这样 API 启动时即使 RabbitMQ 暂时不可用，进程也能正常起来，
// 由重连循环异步恢复（符合 spec "启动时 MQ 不可用 → 不阻塞启动"）。
func NewConnection(cfg *config.RabbitMQConfig, logger *zap.Logger) *Connection {
	return &Connection{
		cfg:            cfg,
		logger:         logger,
		initialBackoff: defaultInitialBackoff,
		maxBackoff:     defaultMaxBackoff,
		done:           make(chan struct{}),
		// 修复：预初始化 runCtx，避免 drainLoop 在 Run() 设置 runCtx 前访问 nil context 导致 panic。
		// context.Background().Done() 返回 nil，在 select 中 nil channel 永不触发，安全。
		// Run() 启动后会用 context.WithCancel(ctx) 覆盖此值，drainLoop 下一轮迭代会读到新的 runCtx。
		runCtx: context.Background(),
	}
}

// Run 是重连循环的入口。
//
// 流程：
//  1. 用调用方 ctx 派生内部 ctx（信号触发 ctx 取消时本协程也会退出）
//  2. 循环：dial → 成功则监听 NotifyClose → 失败则指数退避
//  3. 当 closed=true（Close 被调用）或 ctx 取消时退出并 close(c.done)
//
// 为什么用指数退避：
//   - RabbitMQ 短暂抖动时（重启、扩缩容）立即重试很容易失败
//   - 退避避免对刚启动的 broker 形成"重连风暴"
//   - 上限 30s 保证长时间不可用时仍能周期性重试
//
// 为什么监听 NotifyClose 而不是依赖心跳：
//   - amqp091-go 在连接断开时会主动回调 NotifyClose 通道
//   - 这样不需要额外的心跳探测，能最快感知连接丢失
func (c *Connection) Run(ctx context.Context) {
	c.runCtx, c.runCancel = context.WithCancel(ctx)
	defer close(c.done)

	backoff := c.initialBackoff
	for {
		// 主动关闭优先：Close() 把 closed 置 true 后，本协程下一轮直接退出
		if c.closed.Load() {
			c.logger.Info("rabbitmq connection closed by user")
			return
		}
		// ctx 取消（信号触发、进程级超时）也立即退出
		if err := c.runCtx.Err(); err != nil {
			c.logger.Info("rabbitmq connection context done", zap.Error(err))
			return
		}

		// 尝试建立连接 + 打开一个 channel
		conn, ch, err := c.dialAndSetup()
		if err != nil {
			// 拨号失败：标记为不可用，按 backoff 等待后继续
			c.logger.Warn("rabbitmq dial failed, will retry",
				zap.Error(err), zap.Duration("backoff", backoff))
			c.setReady(false)
			select {
			case <-time.After(backoff):
			case <-c.runCtx.Done():
				return
			}
			// 指数退避：翻倍，封顶在 maxBackoff
			backoff *= 2
			if backoff > c.maxBackoff {
				backoff = c.maxBackoff
			}
			continue
		}

		// 拨号成功：发布新 channel + 标记 ready + 重置 backoff
		c.setReady(true)
		c.setChannel(conn, ch)
		c.logger.Info("rabbitmq connection established")
		backoff = c.initialBackoff

		// 阻塞等待 broker 主动通知连接关闭
		// 容量 1 的 channel 是 amqp091-go 的要求模式
		closeErr := <-conn.NotifyClose(make(chan *amqp091.Error, 1))
		c.setReady(false)
		// 释放旧引用（但不调用 Close，因为连接已经断开了）
		c.mu.Lock()
		c.conn = nil
		c.channel = nil
		c.mu.Unlock()
		if closeErr != nil {
			c.logger.Warn("rabbitmq connection lost, will reconnect", zap.Error(closeErr))
		} else {
			c.logger.Info("rabbitmq connection closed gracefully")
		}
		// 进入下一轮：继续 dial
	}
}

// dialAndSetup 完成一次完整的"建连 + 开 channel"动作。
// 任何一步失败都会清理已分配的 amqp 资源，避免泄漏。
func (c *Connection) dialAndSetup() (*amqp091.Connection, *amqp091.Channel, error) {
	// 修复：使用 url.UserPassword 转义用户名和密码中的特殊字符（@、:、/等），
	// 避免包含特殊字符的密码导致 URL 被错误解析
	u := &url.URL{
		Scheme: "amqp",
		Host:   fmt.Sprintf("%s:%d", c.cfg.Host, c.cfg.Port),
		User:   url.UserPassword(c.cfg.Username, c.cfg.Password),
	}
	amqpURL := u.String()
	conn, err := amqp091.Dial(amqpURL)
	if err != nil {
		return nil, nil, fmt.Errorf("dial: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		// 拨号成功但开 channel 失败：手动关 conn 释放 socket
		conn.Close()
		return nil, nil, fmt.Errorf("open channel: %w", err)
	}
	return conn, ch, nil
}

// setChannel 写时加锁：保护 conn / channel 两个字段的赋值原子性。
func (c *Connection) setChannel(conn *amqp091.Connection, ch *amqp091.Channel) {
	c.mu.Lock()
	c.conn = conn
	c.channel = ch
	c.mu.Unlock()
}

// setReady 原子写入 ready 标志。供 Run 在建立/丢失连接时调用。
func (c *Connection) setReady(v bool) { c.ready.Store(v) }

// Channel 返回当前可用的 channel（只读快照）。
//
// 返回的 channel 来自 amqp091-go；调用方**不应**长期持有它，
// 也不应关闭它（关闭由 Connection.Close 统一管理）。
//
// 失败场景：
//   - ready=false（首次连接未完成 / 正在重连）：返回 ErrNotReady
//   - ready=true 但 channel 字段恰好为 nil（极小竞态窗口）：返回 ErrNotReady
func (c *Connection) Channel() (*amqp091.Channel, error) {
	if !c.ready.Load() {
		return nil, ErrNotReady
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.channel == nil {
		return nil, ErrNotReady
	}
	return c.channel, nil
}

// WaitReady 阻塞直到 channel 可用或 ctx 取消。
//
// 典型使用：worker 注册消费者时 / publisher 首次发消息时。
// 每 100ms 轮询一次 channel 可用状态；轮询间隔是一个权衡：
//   - 太快（< 50ms）浪费 CPU
//   - 太慢（> 1s）感知重连不及时
//
// ctx 取消立即返回（用于优雅停机）。
func (c *Connection) WaitReady(ctx context.Context) (*amqp091.Channel, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if ch, err := c.Channel(); err == nil {
			return ch, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// IsReady 提供给外部（监控 / drainLoop）查询连接可用性。
// 注意：返回值仅是即时快照，调用方拿到后 channel 可能立刻失效。
func (c *Connection) IsReady() bool { return c.ready.Load() }

// Close 主动关闭连接并停止重连循环。
//
// 关闭顺序：
//  1. closed.Swap(true) 保证幂等（多次调用安全）
//  2. runCancel() 触发 Run 协程退出
//  3. 关 channel、关 conn（释放 socket）
//  4. <-c.done 等待 Run 完全退出
//
// 为什么用 closed.Swap 防重入：
//   进程退出阶段可能被多处调用（app.Close、defer、panic recover 等），
//   如果不做幂等保护，第二次 Close 会 double-close channel 引发 panic。
func (c *Connection) Close() {
	if c.closed.Swap(true) {
		return
	}
	if c.runCancel != nil {
		c.runCancel()
	}
	c.mu.Lock()
	if c.channel != nil {
		c.channel.Close()
		c.channel = nil
	}
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.mu.Unlock()
	// 修复：使用 select + 超时替代直接 <-c.done，
	// 避免 Run() 从未被调用时 done channel 永远不会被 close 导致 Close() 永久阻塞
	select {
	case <-c.done:
	case <-time.After(5 * time.Second):
		c.logger.Warn("close timeout: Run() may not have been started")
	}
}
