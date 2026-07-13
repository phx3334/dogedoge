package mysql

import (
	"context"
	"time"

	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/repository/interfaces"

	"gorm.io/gorm"
)

var _ interfaces.FavoriteFolderRepository = (*FavoriteFolderRepo)(nil)

type FavoriteFolderRepo struct {
	db *gorm.DB
}

func NewFavoriteFolderRepo(db *gorm.DB) *FavoriteFolderRepo {
	return &FavoriteFolderRepo{db: db}
}

// FindByUserID 查询用户的所有收藏夹（按创建时间正序，默认收藏夹置顶）
func (r *FavoriteFolderRepo) FindByUserID(ctx context.Context, userID string) ([]database.FavoriteFolder, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var folders []database.FavoriteFolder
	err := r.db.WithContext(ctx).
		Select("id, user_id, title, cover_url, is_default, created_at").
		Where("user_id = ?", userID).
		Order("is_default DESC, created_at ASC").
		Find(&folders).Error
	return folders, err
}

// FindByID 按收藏夹 ID 查询
func (r *FavoriteFolderRepo) FindByID(ctx context.Context, folderID uint64) (*database.FavoriteFolder, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var folder database.FavoriteFolder
	if err := r.db.WithContext(ctx).Where("id = ?", folderID).First(&folder).Error; err != nil {
		return nil, err
	}
	return &folder, nil
}

// CreateFolder 创建收藏夹
// 自动设置 CreatedAt/UpdatedAt
func (r *FavoriteFolderRepo) CreateFolder(ctx context.Context, folder *database.FavoriteFolder) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return r.db.WithContext(ctx).Create(folder).Error
}

// UpdateFolder 更新收藏夹标题/封面
// 使用 Updates + map 确保零值字段（如空 cover_url）也会被写入
func (r *FavoriteFolderRepo) UpdateFolder(ctx context.Context, folderID uint64, title, coverURL string) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return r.db.WithContext(ctx).
		Model(&database.FavoriteFolder{}).
		Where("id = ?", folderID).
		Updates(map[string]interface{}{
			"title":      title,
			"cover_url":  coverURL,
			"updated_at": time.Now(),
		}).Error
}

// DeleteFolder 删除收藏夹（仅当非默认且属于 userID）
// 不允许删除默认收藏夹：默认收藏夹是用户基本资源
// 通过 RowsAffected 判断是否真正删除：1=已删除，0=默认收藏夹或不属于该用户
func (r *FavoriteFolderRepo) DeleteFolder(ctx context.Context, userID string, folderID uint64) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	result := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ? AND is_default = ?", folderID, userID, false).
		Delete(&database.FavoriteFolder{})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// CountVideosInFolder 统计收藏夹中视频数
func (r *FavoriteFolderRepo) CountVideosInFolder(ctx context.Context, folderID uint64) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var count int64
	err := r.db.WithContext(ctx).Model(&database.VideoFavorite{}).
		Where("folder_id = ?", folderID).
		Count(&count).Error
	return count, err
}
