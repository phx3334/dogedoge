package mysql

import (
	"context"

	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/domain/other"
	"gorm.io/gorm"
)

// LoginRepo 登录记录数据存储，实现 LoginRepository 接口。
type LoginRepo struct {
	db *gorm.DB
}

func NewLoginRepo(db *gorm.DB) *LoginRepo {
	return &LoginRepo{db: db}
}

// Create 创建登录记录。
// 通过 context 超时控制防止 MySQL 慢查询阻塞 goroutine。
func (r *LoginRepo) Create(ctx context.Context, login *database.Login) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Create(login).Error
}

// Paginate 分页查询登录记录。
// 通过 context 超时控制防止 MySQL 慢查询阻塞 goroutine。
func (r *LoginRepo) Paginate(ctx context.Context, option other.MySQLOption, dest interface{}) (int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	// 修复：分页参数校验，防止非法值导致 SQL 异常或全表扫描
	if option.Page <= 0 {
		option.Page = 1
	}
	if option.PageSize <= 0 {
		option.PageSize = 10
	}
	if option.PageSize > 100 {
		option.PageSize = 100
	}

	db := r.db.WithContext(ctx).Model(&database.Login{})
	for k, v := range option.Filters {
		db = db.Where(k+"=?", v)
	}
	if option.Order != "" {
		db = db.Order(option.Order)
	}
	for _, p := range option.Preload {
		db = db.Preload(p)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return 0, err
	}
	if err := db.Offset((option.Page - 1) * option.PageSize).Limit(option.PageSize).Find(dest).Error; err != nil {
		return 0, err
	}
	return total, nil
}
