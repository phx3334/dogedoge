package interfaces

import (
	"context"
	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/domain/other"
)

// LoginRepository 登录记录数据仓储接口
//
// 所有方法都接收 context.Context 参数，用于超时控制和请求取消传播。
type LoginRepository interface {
	Create(ctx context.Context, login *database.Login) error
	Paginate(ctx context.Context, option other.MySQLOption, dest interface{}) (int64, error)
}