package mysql

import (
	"context"
	"errors"

	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/repository/interfaces"

	"gorm.io/gorm"
)

var (
	_ interfaces.VideoCoinRepository   = (*VideoCoinRepo)(nil)
	_ interfaces.CoinLedgerRepository = (*CoinLedgerRepo)(nil)
)

// ---------------------------------------------------------------------------
// VideoCoinRepo —— 视频投币
// ---------------------------------------------------------------------------

type VideoCoinRepo struct {
	db *gorm.DB
}

func NewVideoCoinRepo(db *gorm.DB) *VideoCoinRepo {
	return &VideoCoinRepo{db: db}
}

// FindByUserVideo 查询用户对某视频的投币记录
func (r *VideoCoinRepo) FindByUserVideo(ctx context.Context, userID string, videoID uint) (*database.VideoCoin, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var coin database.VideoCoin
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND video_id = ?", userID, videoID).
		First(&coin).Error; err != nil {
		return nil, err
	}
	return &coin, nil
}

// Upsert 创建或更新投币记录
//   - 不存在：插入新记录，added = amount
//   - 已存在：累加 amount，但上限为 2；added = max(0, 2 - 当前 Amount)
//
// amount 必须 ∈ {1, 2}（由调用方校验）
func (r *VideoCoinRepo) Upsert(ctx context.Context, userID string, videoID uint, amount int) (int, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	// 先查现有记录
	var existing database.VideoCoin
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND video_id = ?", userID, videoID).
		First(&existing).Error

	if err == nil {
		// 已有记录，累加但上限 2
		newAmount := existing.Amount + amount
		if newAmount > 2 {
			newAmount = 2
		}
		added := newAmount - existing.Amount
		if added <= 0 {
			return 0, nil
		}
		err = r.db.WithContext(ctx).Model(&database.VideoCoin{}).
			Where("id = ?", existing.ID).
			Update("amount", newAmount).Error
		if err != nil {
			return 0, err
		}
		return added, nil
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}

	// 不存在则插入
	coin := database.VideoCoin{
		UserID:  userID,
		VideoID: uint64(videoID),
		Amount:  amount,
	}
	if err := r.db.WithContext(ctx).Create(&coin).Error; err != nil {
		// 并发情况下可能被其他请求先插入，回退到更新路径
		var existing2 database.VideoCoin
		if err2 := r.db.WithContext(ctx).
			Where("user_id = ? AND video_id = ?", userID, videoID).
			First(&existing2).Error; err2 == nil {
			newAmount := existing2.Amount + amount
			if newAmount > 2 {
				newAmount = 2
			}
			added := newAmount - existing2.Amount
			if added <= 0 {
				return 0, nil
			}
			if err2 := r.db.WithContext(ctx).Model(&database.VideoCoin{}).
				Where("id = ?", existing2.ID).
				Update("amount", newAmount).Error; err2 != nil {
				return 0, err2
			}
			return added, nil
		}
		return 0, err
	}
	return amount, nil
}

// ---------------------------------------------------------------------------
// CoinLedgerRepo —— 硬币流水
// ---------------------------------------------------------------------------

type CoinLedgerRepo struct {
	db *gorm.DB
}

func NewCoinLedgerRepo(db *gorm.DB) *CoinLedgerRepo {
	return &CoinLedgerRepo{db: db}
}

// Create 写入一条流水
func (r *CoinLedgerRepo) Create(ctx context.Context, l *database.CoinLedger) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Create(l).Error
}

// ListByUser 分页查询用户流水（按时间倒序）
func (r *CoinLedgerRepo) ListByUser(ctx context.Context, userID uint64, reasonType string, page, pageSize int) ([]database.CoinLedger, int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	query := r.db.WithContext(ctx).Model(&database.CoinLedger{}).Where("user_id = ?", userID)
	if reasonType != "" {
		query = query.Where("reason_type = ?", reasonType)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 || page < 1 || pageSize < 1 {
		return nil, total, nil
	}
	offset := (page - 1) * pageSize
	var ledgers []database.CoinLedger
	if err := query.Order("created_at DESC").
		Offset(offset).Limit(pageSize).
		Find(&ledgers).Error; err != nil {
		return nil, total, err
	}
	return ledgers, total, nil
}
