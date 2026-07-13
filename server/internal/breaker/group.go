package breaker

import "golang.org/x/sync/semaphore"

// Group 按域分组的熔断器集合
//
// 设计动机：不同下游依赖的可用性是相互独立的。
// 例如 MySQL 抖动时，Redis 仍然健康；反之亦然。
// 把 Redis 和 MySQL 共用一个熔断器会出现"一个污染另一个"的问题：
// Redis 失败 5 次后，MySQL 还没出问题也被强制熔断，导致不必要的降级。
//
// 通过 Group 持有多个独立 Breaker，业务层按依赖类型选择对应的熔断器。
type Group struct {
	// Redis 缓存层熔断器：包装所有 Redis 调用（如 GetUserCache / GetVideoCache）
	Redis *Breaker
	// MySQL 数据库熔断器：包装所有 MySQL 回源查询（如 BackfillUserCache / BackfillVideoCache）
	MySQL *Breaker
	// ES 搜索引擎熔断器：包装所有 Elasticsearch 调用（如 SearchVideos / IndexVideo）
	// ES 不可用时搜索功能快速失败，不影响视频列表等非搜索功能
	ES *Breaker

	// 全局并发信号量：限制对数据库的并发请求数，与连接池 MaxOpenConns 对齐
	// 信号量作用于全局，不是单个请求——单个请求的缓存击穿防穿透应使用 singleflight
	MySQLReadSem  *semaphore.Weighted
	MySQLWriteSem *semaphore.Weighted
}

// SemaphoreConfig 信号量配置，应与数据库连接池参数对齐
type SemaphoreConfig struct {
	// MySQLReadLimit 对应数据库 MaxOpenConns 中读连接的份额
	// 建议值：MaxOpenConns * 0.7（读多写少场景）
	MySQLReadLimit int64
	// MySQLWriteLimit 对应数据库 MaxOpenConns 中写连接的份额
	// 建议值：MaxOpenConns * 0.3
	MySQLWriteLimit int64
}

// NewGroup 构造默认配置的熔断器组
//
// 配置：FailureThreshold=5 次连续失败、OpenDuration=30s（30 * 1e9 纳秒）。
// 使用纳秒字面量避免额外 import time，编译期与 time.Duration(30*time.Second) 等价。
//
// 信号量限制应与数据库连接池 MaxOpenConns 对齐，防止超出连接池容量的请求排队等待。
func NewGroup(semCfg SemaphoreConfig) *Group {
	return &Group{
		Redis: New(Config{FailureThreshold: 3, OpenDuration: 10 * 1e9}), // 10s, Redis 故障通常是瞬时网络抖动，恢复快
		MySQL: New(Config{FailureThreshold: 5, OpenDuration: 30 * 1e9}), // 30s
		ES:    New(Config{FailureThreshold: 5, OpenDuration: 30 * 1e9}),

		MySQLReadSem:  semaphore.NewWeighted(semCfg.MySQLReadLimit),
		MySQLWriteSem: semaphore.NewWeighted(semCfg.MySQLWriteLimit),
	}
}
