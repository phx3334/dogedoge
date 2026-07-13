package mysql

import (
	"context"
	"time"

	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/repository/interfaces"

	"gorm.io/gorm"
)

// 编译期接口校验
var (
	_ interfaces.AccountRepository = (*AccountRepo)(nil)
	_ interfaces.LoginRepository   = (*LoginRepo)(nil)
)

// defaultDBTimeout 是 MySQL 操作的默认超时时间。
// 当调用方传入的 context 没有设置 deadline 时，使用此默认值。
// 行业标准：读操作 3~5s，写操作 5~10s；这里取 5s 作为通用默认值，
// 关键写操作（如 Register）可由调用方设置更长的超时。
const defaultDBTimeout = 5 * time.Second

// AccountRepo 用户账户数据仓储，实现 AccountRepository 接口。
type AccountRepo struct {
	db *gorm.DB
}

func NewAccountRepo(db *gorm.DB) *AccountRepo {
	return &AccountRepo{db: db}
}

// withTimeout 为 context 添加超时控制。
// 若 ctx 已有 deadline，则直接返回原 ctx（透传模式），
// 避免覆盖调用方设置的更短超时时间。
// 若 ctx 无 deadline，则创建带 defaultDBTimeout 超时的新 context。
func withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, defaultDBTimeout)
}

// Create 创建用户账户记录。
// 通过 context 超时控制防止 MySQL 慢查询阻塞 goroutine。
func (r *AccountRepo) Create(ctx context.Context, account *database.Account) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Create(account).Error
}

// FindByEmail 根据邮箱查询用户。
func (r *AccountRepo) FindByEmail(ctx context.Context, email string) (*database.Account, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var user database.Account
	if err := r.db.WithContext(ctx).Where("email=?", email).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByID 根据 ID 查询用户。
func (r *AccountRepo) FindByID(ctx context.Context, id string) (*database.Account, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var user database.Account
	if err := r.db.WithContext(ctx).Where("id=?", id).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByIDs 根据 ID 列表批量查询用户。
func (r *AccountRepo) FindByIDs(ctx context.Context, ids []string) ([]*database.Account, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var users []*database.Account
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// Save 保存用户（全字段更新或插入）。
func (r *AccountRepo) Save(ctx context.Context, user *database.Account) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Save(user).Error
}

// Updates 更新用户指定字段。
func (r *AccountRepo) Updates(ctx context.Context, id string, updates map[string]interface{}) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Model(&database.Account{}).Where("id=?", id).Updates(updates).Error
}

// UpdateColumn 更新用户单个字段（不触发 GORM 钩子）。
func (r *AccountRepo) UpdateColumn(ctx context.Context, id string, column string, value interface{}) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Model(&database.Account{}).Where("id=?", id).Update(column, value).Error
}

// AddExperience 原子加经验值
// 用 GREATEST(0, experience + delta) 兜底防止负数；
// delta 可为负（取款经验）也可为正（任务奖励）
func (r *AccountRepo) AddExperience(ctx context.Context, id string, delta int64) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).
		Model(&database.Account{}).
		Where("id = ?", id).
		UpdateColumn("experience", gorm.Expr("GREATEST(0, experience + ?)", delta)).Error
}

// AddCoinBalanceTenths 原子调整用户硬币余额（0.1 硬币单位）
// deltaTenths 可为正（充值/退款）或负（投币消费）；GREATEST(0, ...) 防止透支
func (r *AccountRepo) AddCoinBalanceTenths(ctx context.Context, id string, deltaTenths int64) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).
		Model(&database.Account{}).
		Where("id = ?", id).
		UpdateColumn("coin_balance_tenths", gorm.Expr("GREATEST(0, coin_balance_tenths + ?)", deltaTenths)).Error
}
