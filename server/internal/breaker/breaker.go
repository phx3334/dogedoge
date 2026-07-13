// Package breaker 提供极简三态熔断器
//
// 状态机：
//   - Closed（关闭）：正常调用，每次失败计数 +N；连续 N 次失败转 Open
//   - Open（断开）：直接返回 ErrCircuitOpen，不调用 fn
//   - Half-Open（半开）：冷却期过后，放行 1 个探测；成功转 Closed，失败转 Open
//
// 线程安全：所有状态读取/修改都在 mu 保护下。
package breaker

import (
	"errors"
	"sync"
	"time"
)

// ErrCircuitOpen 表示熔断器处于 Open 状态
//
// 当熔断器处于 Open 或 Half-Open 探测数已满时，Execute 会立即返回此错误，
// 业务层可以据此走降级路径（如直接返回空值 / 默认值）。
var ErrCircuitOpen = errors.New("circuit breaker is open")

// State 表示熔断器状态
//
// 状态机流转图：
//
//	                   连续失败 N 次
//	  Closed  ───────────────────────▶  Open
//	    ▲                              │
//	    │  探测成功                     │ OpenDuration 到期
//	    │                              ▼
//	    └──────────────────  Half-Open
//	                            │
//	                            │ 探测失败
//	                            ▼
//	                           Open
type State int

const (
	// StateClosed 关闭状态：所有请求正常通过，失败计数累计
	StateClosed State = iota
	// StateOpen 断开状态：所有请求直接拒绝（返回 ErrCircuitOpen），不调用 fn
	StateOpen
	// StateHalfOpen 半开状态：放行有限个探测（默认 1 个）验证下游是否恢复
	StateHalfOpen
)

// String 返回状态的字符串表示（仅用于日志 / metrics）
func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half_open"
	default:
		return "unknown"
	}
}

// Config 熔断器配置
type Config struct {
	// FailureThreshold 连续失败次数阈值
	// 在 Closed 状态下，连续失败次数达到此值时进入 Open
	FailureThreshold int

	// OpenDuration Open 状态持续时间（冷却期）
	// Open 状态持续此时间后转为 Half-Open；探测期间超出 HalfOpenMaxProbes 的请求仍被拒绝
	OpenDuration time.Duration

	// HalfOpenMaxProbes Half-Open 状态允许放行的探测数（通常 1）
	// 仅允许指定数量的探测请求通过，验证下游是否真正恢复；
	// 探测期间其他请求被拒绝，避免雪崩重演
	HalfOpenMaxProbes int
}

// Breaker 单个熔断器
//
// 设计目标：极简、零依赖、易于注入到任何回调（fn func() error）。
// 不维护失败时间窗口（sliding window），仅追踪 consecutive failures，
// 适合"瞬时抖动恢复"场景；如需更复杂的统计可替换实现。
type Breaker struct {
	cfg Config
	mu               sync.Mutex
	state            State
	consecutiveFail  int
	openedAt         time.Time
	halfOpenInFlight int
}

// New 创建一个熔断器
//
// 对配置做最小化兜底：FailureThreshold / OpenDuration / HalfOpenMaxProbes 为 0 时
// 退化为合理默认值（5 / 30s / 1），避免无意义的"立即熔断"或"永不熔断"。
func New(cfg Config) *Breaker {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 5
	}
	if cfg.OpenDuration <= 0 {
		cfg.OpenDuration = 30 * time.Second
	}
	if cfg.HalfOpenMaxProbes <= 0 {
		cfg.HalfOpenMaxProbes = 1
	}
	return &Breaker{cfg: cfg, state: StateClosed}
}

// Execute 执行 fn，受熔断器状态约束
//
// 决策流程（在 mu 保护下）：
//  1. Closed：直接放行
//  2. Open：若仍在冷却期，返回 ErrCircuitOpen；否则转为 Half-Open 并放行 1 个探测
//  3. Half-Open：若探测数已满（>= HalfOpenMaxProbes），返回 ErrCircuitOpen；否则递增 in-flight 计数
//
// 注意：fn 真正执行时**不持锁**（fn 可能是慢 IO / 远程调用），仅在结果返回后
// 重新加锁更新状态——这是熔断器不阻塞热路径的关键。
func (b *Breaker) Execute(fn func() error) error {
	b.mu.Lock()

	// 状态转换 + 准入决策
	switch b.state {
	case StateClosed:
		// pass through
	case StateOpen:
		if time.Since(b.openedAt) < b.cfg.OpenDuration {
			b.mu.Unlock()
			return ErrCircuitOpen
		}
		// 冷却期结束 → Half-Open
		b.state = StateHalfOpen
		b.halfOpenInFlight = 0
		fallthrough
	case StateHalfOpen:
		if b.halfOpenInFlight >= b.cfg.HalfOpenMaxProbes {
			b.mu.Unlock()
			return ErrCircuitOpen
		}
		b.halfOpenInFlight++
	}

	b.mu.Unlock()

	// 真正执行 fn（不持锁，因为 fn 可能慢）
	err := fn()

	b.mu.Lock()
	defer b.mu.Unlock()

	if err != nil {
		b.onFailure()
	} else {
		b.onSuccess()
	}
	return err
}

// onSuccess 成功回调（在 mu 保护下调用）
//
// 行为：
//   - 重置 consecutiveFail
//   - 若处于 Half-Open：递减 in-flight 计数；归零时转为 Closed
func (b *Breaker) onSuccess() {
	b.consecutiveFail = 0
	if b.state == StateHalfOpen {
		b.halfOpenInFlight--
		if b.halfOpenInFlight == 0 {
			b.state = StateClosed
		}
	}
}

// onFailure 失败回调（在 mu 保护下调用）
//
// 行为：
//   - Half-Open 中失败：立即转 Open（不重置 in-flight，避免新探测）
//   - Closed 中失败：累加 consecutiveFail；达到阈值时转 Open
func (b *Breaker) onFailure() {
	if b.state == StateHalfOpen {
		b.halfOpenInFlight--
		b.state = StateOpen
		b.openedAt = time.Now()
		return
	}
	b.consecutiveFail++
	if b.consecutiveFail >= b.cfg.FailureThreshold {
		b.state = StateOpen
		b.openedAt = time.Now()
	}
}

// State 返回当前状态（仅用于 metrics/log）
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}
