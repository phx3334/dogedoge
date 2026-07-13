package interfaces

import (
	"context"
	"fake_tiktok/internal/domain/other"

	"gorm.io/gorm"
)

// PaginateRepository 分页查询数据仓储接口
//
// 所有方法都接收 context.Context 参数，用于超时控制和请求取消传播。
type PaginateRepository interface {
	Paginate(ctx context.Context, option other.MySQLOption, dest interface{}) (int64, error)
	CursorPaginate(ctx context.Context, query *gorm.DB, opt other.CursorOption, dest interface{}) (int64, string, error)
}