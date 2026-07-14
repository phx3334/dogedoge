package mysql

import (
	"context"
	"database/sql"
	"errors"

	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/repository/interfaces"

	"gorm.io/gorm"
)

var _ interfaces.InteractionRepository = (*InteractionRepo)(nil)

type InteractionRepo struct {
	db *gorm.DB
}

func NewInteractionRepo(db *gorm.DB) *InteractionRepo {
	return &InteractionRepo{db: db}
}

func (r *InteractionRepo) GetUserVideoInteraction(ctx context.Context, userID string, videoID uint) (*interfaces.InteractionStatus, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	// 修复：使用只读事务保证三次查询的快照一致性
	// 并发写入场景下，三次独立查询之间数据可能变化，导致返回的状态不是某一时刻的一致快照
	var status interfaces.InteractionStatus
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ? AND video_id = ?", userID, videoID).First(&database.VideoLike{}).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		} else {
			status.IsLiked = true
		}

		var fav database.VideoFavorite
		if err := tx.Joins("JOIN favorite_folders ON favorite_folders.id = video_favorites.folder_id").
			Where("favorite_folders.user_id = ? AND video_favorites.video_id = ?", userID, videoID).
			First(&fav).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		} else {
			status.IsFavorited = true
		}

		var coin database.VideoCoin
		if err := tx.Where("user_id = ? AND video_id = ?", userID, videoID).First(&coin).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		} else {
			status.CoinCount = int64(coin.Amount)
		}

		return nil
	}, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	return &status, nil
}

func (r *InteractionRepo) GetFansCount(ctx context.Context, userID string) (int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var count int64
	err := r.db.WithContext(ctx).Model(&database.UserFollow{}).Where("followee_id = ?", userID).Count(&count).Error
	return count, err
}

func (r *InteractionRepo) GetFollowingCount(ctx context.Context, userID string) (int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var count int64
	err := r.db.WithContext(ctx).Model(&database.UserFollow{}).Where("follower_id = ?", userID).Count(&count).Error
	return count, err
}

func (r *InteractionRepo) IsUserFollowed(ctx context.Context, followerID, followeeID string) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var follow database.UserFollow
	err := r.db.WithContext(ctx).
		Where("follower_id = ? AND followee_id = ?", followerID, followeeID).
		First(&follow).Error
	if err == gorm.ErrRecordNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// IsMutualFollow 判断 a、b 是否互相关注（两个方向都已关注）。
// 先查 a→b，未关注直接返回 false（短路，省一次查询）。
func (r *InteractionRepo) IsMutualFollow(ctx context.Context, a, b string) (bool, error) {
	fwd, err := r.IsUserFollowed(ctx, a, b)
	if err != nil {
		return false, err
	}
	if !fwd {
		return false, nil
	}
	return r.IsUserFollowed(ctx, b, a)
}

// CreateVideoLike 同步写入用户点赞记录到 MySQL VideoLike 表
// 使用 唯一索引 idx_video_like_user_video 保证幂等：记录已存在时不报错
//
// 返回值：
//   - created=true：真正新建了记录（首次点赞），调用方应发布 MQ 增量
//   - created=false：记录已存在（SET 过期后重复点赞），调用方不应发布 MQ 增量
//   - err：数据库错误（非唯一索引冲突）
func (r *InteractionRepo) CreateVideoLike(ctx context.Context, userID string, videoID uint) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	like := database.VideoLike{
		UserID:  userID,
		VideoID: uint64(videoID),
	}
	// FirstOrCreate：记录不存在则创建，已存在则查询返回
	// 通过 RowsAffected 判断：1=新建，0=已存在
	result := r.db.WithContext(ctx).Where("user_id = ? AND video_id = ?", userID, videoID).
		FirstOrCreate(&like)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// DeleteVideoLike 删除用户点赞记录（取消点赞）
// 通过 RowsAffected 判断是否真正删除：1=已删除，0=原本不存在
func (r *InteractionRepo) DeleteVideoLike(ctx context.Context, userID string, videoID uint) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	result := r.db.WithContext(ctx).
		Where("user_id = ? AND video_id = ?", userID, videoID).
		Delete(&database.VideoLike{})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// CreateFollow 创建关注关系
// 通过唯一索引 idx_user_follow_pair 保证幂等：已存在不报错
func (r *InteractionRepo) CreateFollow(ctx context.Context, followerID, followeeID string) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	// 不允许关注自己
	if followerID == followeeID {
		return false, nil
	}

	follow := database.UserFollow{
		FollowerID: followerID,
		FolloweeID: followeeID,
	}
	result := r.db.WithContext(ctx).
		Where("follower_id = ? AND followee_id = ?", followerID, followeeID).
		FirstOrCreate(&follow)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// DeleteFollow 删除关注关系
func (r *InteractionRepo) DeleteFollow(ctx context.Context, followerID, followeeID string) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	result := r.db.WithContext(ctx).
		Where("follower_id = ? AND followee_id = ?", followerID, followeeID).
		Delete(&database.UserFollow{})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// ListFollowers 分页查询某用户的粉丝列表（followerID 列表）
func (r *InteractionRepo) ListFollowers(ctx context.Context, userID string, page, pageSize int) ([]string, int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var total int64
	if err := r.db.WithContext(ctx).Model(&database.UserFollow{}).
		Where("followee_id = ?", userID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 || page < 1 || pageSize < 1 {
		return nil, total, nil
	}
	offset := (page - 1) * pageSize
	var follows []database.UserFollow
	if err := r.db.WithContext(ctx).
		Where("followee_id = ?", userID).
		Order("created_at DESC").
		Offset(offset).Limit(pageSize).
		Find(&follows).Error; err != nil {
		return nil, total, err
	}
	ids := make([]string, 0, len(follows))
	for _, f := range follows {
		ids = append(ids, f.FollowerID)
	}
	return ids, total, nil
}

// ListFollowing 分页查询某用户关注的列表（followeeID 列表）
func (r *InteractionRepo) ListFollowing(ctx context.Context, userID string, page, pageSize int) ([]string, int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var total int64
	if err := r.db.WithContext(ctx).Model(&database.UserFollow{}).
		Where("follower_id = ?", userID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 || page < 1 || pageSize < 1 {
		return nil, total, nil
	}
	offset := (page - 1) * pageSize
	var follows []database.UserFollow
	if err := r.db.WithContext(ctx).
		Where("follower_id = ?", userID).
		Order("created_at DESC").
		Offset(offset).Limit(pageSize).
		Find(&follows).Error; err != nil {
		return nil, total, err
	}
	ids := make([]string, 0, len(follows))
	for _, f := range follows {
		ids = append(ids, f.FolloweeID)
	}
	return ids, total, nil
}

// -----------------------------------------------------------------------------
// FavoriteRepo 实现 FavoriteRepository 接口
// -----------------------------------------------------------------------------

var _ interfaces.FavoriteRepository = (*FavoriteRepo)(nil)

// FavoriteRepo 视频收藏数据仓储
type FavoriteRepo struct {
	db *gorm.DB
}

func NewFavoriteRepo(db *gorm.DB) *FavoriteRepo {
	return &FavoriteRepo{db: db}
}

// EnsureDefaultFolder 确保用户有默认收藏夹，返回默认收藏夹 ID
// 没有时自动创建一个名为「默认收藏夹」的文件夹
func (r *FavoriteRepo) EnsureDefaultFolder(ctx context.Context, userID string) (uint64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var folder database.FavoriteFolder
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND is_default = ?", userID, true).
		First(&folder).Error
	if err == nil {
		return folder.ID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}
	// 不存在则创建
	folder = database.FavoriteFolder{
		UserID:    userID,
		Title:     "默认收藏夹",
		IsDefault: true,
	}
	if err := r.db.WithContext(ctx).Create(&folder).Error; err != nil {
		return 0, err
	}
	return folder.ID, nil
}

// AddFavorite 收藏视频到指定收藏夹
// folderID=0 时使用默认收藏夹
// 通过唯一索引 idx_video_fav_user_video_folder 保证幂等
func (r *FavoriteRepo) AddFavorite(ctx context.Context, userID string, videoID uint, folderID uint64) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	// folderID=0 → 取/建默认收藏夹
	if folderID == 0 {
		var err error
		folderID, err = r.EnsureDefaultFolder(ctx, userID)
		if err != nil {
			return false, err
		}
	}

	fav := database.VideoFavorite{
		UserID:   userID,
		VideoID:  uint64(videoID),
		FolderID: folderID,
	}
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND video_id = ? AND folder_id = ?", userID, videoID, folderID).
		FirstOrCreate(&fav)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// RemoveFavorite 从所有收藏夹移除该视频
func (r *FavoriteRepo) RemoveFavorite(ctx context.Context, userID string, videoID uint) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	result := r.db.WithContext(ctx).
		Where("user_id = ? AND video_id = ?", userID, videoID).
		Delete(&database.VideoFavorite{})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// RemoveFromFolder 从指定收藏夹移除该视频
func (r *FavoriteRepo) RemoveFromFolder(ctx context.Context, userID string, videoID uint, folderID uint64) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	result := r.db.WithContext(ctx).
		Where("user_id = ? AND video_id = ? AND folder_id = ?", userID, videoID, folderID).
		Delete(&database.VideoFavorite{})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// ListFavoritesByFolder 分页查询收藏夹中的视频 ID 列表
// folderID=0 时查询默认收藏夹
func (r *FavoriteRepo) ListFavoritesByFolder(ctx context.Context, userID string, folderID uint64, page, pageSize int) ([]uint, int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	if folderID == 0 {
		var err error
		folderID, err = r.EnsureDefaultFolder(ctx, userID)
		if err != nil {
			return nil, 0, err
		}
	}

	var total int64
	if err := r.db.WithContext(ctx).Model(&database.VideoFavorite{}).
		Where("user_id = ? AND folder_id = ?", userID, folderID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 || page < 1 || pageSize < 1 {
		return nil, total, nil
	}
	offset := (page - 1) * pageSize
	var favs []database.VideoFavorite
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND folder_id = ?", userID, folderID).
		Order("created_at DESC").
		Offset(offset).Limit(pageSize).
		Find(&favs).Error; err != nil {
		return nil, total, err
	}
	videoIDs := make([]uint, 0, len(favs))
	for _, f := range favs {
		videoIDs = append(videoIDs, uint(f.VideoID))
	}
	return videoIDs, total, nil
}
