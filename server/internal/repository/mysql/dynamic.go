package mysql

import (
	"context"
	"errors"

	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/repository/interfaces"

	"gorm.io/gorm"
)

var (
	_ interfaces.UserDynamicRepository     = (*UserDynamicRepo)(nil)
	_ interfaces.UserDynamicLikeRepository = (*UserDynamicLikeRepo)(nil)
)

// ---------------------------------------------------------------------------
// UserDynamicRepo —— 用户动态
// ---------------------------------------------------------------------------

type UserDynamicRepo struct {
	db *gorm.DB
}

func NewUserDynamicRepo(db *gorm.DB) *UserDynamicRepo {
	return &UserDynamicRepo{db: db}
}

// Create 发布动态
func (r *UserDynamicRepo) Create(ctx context.Context, d *database.UserDynamicText) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Create(d).Error
}

// FindByID 查询单条动态
func (r *UserDynamicRepo) FindByID(ctx context.Context, id uint64) (*database.UserDynamicText, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var d database.UserDynamicText
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&d).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

// ListByUser 分页查询指定用户的动态（按 CreatedAt 倒序）
func (r *UserDynamicRepo) ListByUser(ctx context.Context, userID uint64, page, pageSize int) ([]database.UserDynamicText, int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var total int64
	if err := r.db.WithContext(ctx).Model(&database.UserDynamicText{}).
		Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 || page < 1 || pageSize < 1 {
		return nil, total, nil
	}
	offset := (page - 1) * pageSize
	var items []database.UserDynamicText
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Offset(offset).Limit(pageSize).
		Find(&items).Error; err != nil {
		return nil, total, err
	}
	return items, total, nil
}

// ListFeed 分页查询关注用户的最新动态
func (r *UserDynamicRepo) ListFeed(ctx context.Context, userIDs []string, page, pageSize int) ([]database.UserDynamicText, int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	if len(userIDs) == 0 {
		return nil, 0, nil
	}

	// 转换为 []uint64 用于查询
	ids := make([]uint64, 0, len(userIDs))
	for _, idStr := range userIDs {
		var id uint64
		for _, ch := range idStr {
			if ch < '0' || ch > '9' {
				break
			}
			id = id*10 + uint64(ch-'0')
		}
		if id > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, 0, nil
	}

	var total int64
	if err := r.db.WithContext(ctx).Model(&database.UserDynamicText{}).
		Where("user_id IN ?", ids).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 || page < 1 || pageSize < 1 {
		return nil, total, nil
	}
	offset := (page - 1) * pageSize
	var items []database.UserDynamicText
	if err := r.db.WithContext(ctx).
		Where("user_id IN ?", ids).
		Order("created_at DESC").
		Offset(offset).Limit(pageSize).
		Find(&items).Error; err != nil {
		return nil, total, err
	}
	return items, total, nil
}

// IncrementLikeCount 动态点赞数 +delta
func (r *UserDynamicRepo) IncrementLikeCount(ctx context.Context, id uint64, delta int) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Model(&database.UserDynamicText{}).
		Where("id = ?", id).
		UpdateColumn("like_count", gorm.Expr("GREATEST(0, like_count + ?)", delta)).Error
}

// IncrementCommentCount 动态评论数 +delta
func (r *UserDynamicRepo) IncrementCommentCount(ctx context.Context, id uint64, delta int) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Model(&database.UserDynamicText{}).
		Where("id = ?", id).
		UpdateColumn("comment_count", gorm.Expr("GREATEST(0, comment_count + ?)", delta)).Error
}

// ---------------------------------------------------------------------------
// UserDynamicLikeRepo —— 动态点赞
// ---------------------------------------------------------------------------

type UserDynamicLikeRepo struct {
	db *gorm.DB
}

func NewUserDynamicLikeRepo(db *gorm.DB) *UserDynamicLikeRepo {
	return &UserDynamicLikeRepo{db: db}
}

// CreateLike 点赞动态
// 通过唯一索引 idx_dyn_like_user_dyn 保证幂等：已存在则 FirstOrCreate 返回 RowsAffected=0
func (r *UserDynamicLikeRepo) CreateLike(ctx context.Context, userID uint64, dynamicID uint64) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	like := database.UserDynamicLike{
		UserID:    userID,
		DynamicID: dynamicID,
	}
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND dynamic_id = ?", userID, dynamicID).
		FirstOrCreate(&like)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// DeleteLike 取消点赞
func (r *UserDynamicLikeRepo) DeleteLike(ctx context.Context, userID uint64, dynamicID uint64) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	result := r.db.WithContext(ctx).
		Where("user_id = ? AND dynamic_id = ?", userID, dynamicID).
		Delete(&database.UserDynamicLike{})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// IsLiked 查询是否已点赞
func (r *UserDynamicLikeRepo) IsLiked(ctx context.Context, userID uint64, dynamicID uint64) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var like database.UserDynamicLike
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND dynamic_id = ?", userID, dynamicID).
		First(&like).Error
	if err == nil {
		return true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return false, err
}

// ListLikedByUser 分页查询用户点赞过的动态 ID
func (r *UserDynamicLikeRepo) ListLikedByUser(ctx context.Context, userID uint64, page, pageSize int) ([]uint64, int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var total int64
	if err := r.db.WithContext(ctx).Model(&database.UserDynamicLike{}).
		Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 || page < 1 || pageSize < 1 {
		return nil, total, nil
	}
	offset := (page - 1) * pageSize
	var likes []database.UserDynamicLike
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Offset(offset).Limit(pageSize).
		Find(&likes).Error; err != nil {
		return nil, total, err
	}
	ids := make([]uint64, 0, len(likes))
	for _, l := range likes {
		ids = append(ids, l.DynamicID)
	}
	return ids, total, nil
}
