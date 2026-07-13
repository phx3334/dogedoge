package interfaces

import (
	"context"
	"fake_tiktok/internal/domain/database"
)

// AccountRepository 用户账户数据仓储接口
//
// 所有方法都接收 context.Context 参数，用于：
//   - 超时控制：通过 context.WithTimeout 设置查询/写入的最大执行时间，
//     防止 MySQL 慢查询或网络抖动导致 goroutine 长期阻塞
//   - 链路追踪：context 中可携带 trace ID 等可观测性信息
//   - 请求取消：当 HTTP 请求被客户端取消时，context 会被 cancel，
//     数据库操作应立即中断，避免浪费连接池资源
type AccountRepository interface {
	Create(ctx context.Context, account *database.Account) error
	FindByEmail(ctx context.Context, email string) (*database.Account, error)
	FindByID(ctx context.Context, id string) (*database.Account, error)
	FindByIDs(ctx context.Context, ids []string) ([]*database.Account, error)
	Save(ctx context.Context, user *database.Account) error
	Updates(ctx context.Context, id string, updates map[string]interface{}) error
	UpdateColumn(ctx context.Context, id string, column string, value interface{}) error

	// AddExperience 原子加经验值（delta 可为负）
	// 用 SQL 端 +/- 避免读改写竞态；GREATEST(0, exp+delta) 兜底防负数
	AddExperience(ctx context.Context, id string, delta int64) error
	// AddCoinBalanceTenths 原子调整用户硬币余额（0.1 硬币单位，delta 可为负）
	// 用 GREATEST(0, ...) 兜底防负数
	AddCoinBalanceTenths(ctx context.Context, id string, deltaTenths int64) error
}

