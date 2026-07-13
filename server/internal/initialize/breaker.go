package initialize

import (
	"fake_tiktok/internal/breaker"
	"fake_tiktok/internal/config"

	"go.uber.org/zap"
)

// NewBreakerGroup 创建全局熔断器组
//
// 返回一个包含 Redis / MySQL 两个独立熔断器的 Group。
// 业务层根据调用类型选择对应熔断器：
//   - 缓存读 / 写（UserCacheRepo / VideoCacheRepo / InteractionCacheRepo 等）→ g.Redis.Execute
//   - MySQL 回源（BackfillUserCache / BackfillVideoCache 等）→ g.MySQL.Execute
//
// 信号量限制与数据库连接池 MaxOpenConns 对齐：
//   - 读信号量 = MaxOpenConns * 0.7（读多写少场景）
//   - 写信号量 = MaxOpenConns * 0.3
// 防止超出连接池容量的请求排队等待连接，导致 goroutine 堆积。
func NewBreakerGroup(cfg *config.Config, logger *zap.Logger) *breaker.Group {
	maxOpen := int64(cfg.Database.MaxOpenConns)
	if maxOpen <= 0 {
		maxOpen = 100 // 默认值，与 config.yaml 一致
	}
	g := breaker.NewGroup(breaker.SemaphoreConfig{
		MySQLReadLimit:  int64(float64(maxOpen) * 0.7),
		MySQLWriteLimit: int64(float64(maxOpen) * 0.3),
	})
	logger.Info("circuit breaker group initialized",
		zap.String("redis_state", g.Redis.State().String()),
		zap.String("mysql_state", g.MySQL.State().String()),
		zap.Int64("mysql_read_sem_limit", int64(float64(maxOpen)*0.7)),
		zap.Int64("mysql_write_sem_limit", int64(float64(maxOpen)*0.3)),
	)
	return g
}
