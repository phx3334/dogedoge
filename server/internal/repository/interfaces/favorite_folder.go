package interfaces

import (
	"context"
	"fake_tiktok/internal/domain/database"
)

// FavoriteFolderRepository 收藏夹数据仓储接口
type FavoriteFolderRepository interface {
	// FindByUserID 查询用户的所有收藏夹
	FindByUserID(ctx context.Context, userID string) ([]database.FavoriteFolder, error)

	// FindByID 按收藏夹 ID 查询；不存在返回 gorm.ErrRecordNotFound
	FindByID(ctx context.Context, folderID uint64) (*database.FavoriteFolder, error)

	// CreateFolder 创建收藏夹
	// 返回新收藏夹 ID（folder.ID 由 GORM 回填）
	CreateFolder(ctx context.Context, folder *database.FavoriteFolder) error

	// UpdateFolder 更新收藏夹标题/封面
	UpdateFolder(ctx context.Context, folderID uint64, title, coverURL string) error

	// DeleteFolder 删除收藏夹（仅当非默认且属于 userID）
	// 返回 deleted=true 表示真正删除；调用方据此决定是否清理 video_favorites
	DeleteFolder(ctx context.Context, userID string, folderID uint64) (deleted bool, err error)

	// CountVideosInFolder 统计收藏夹中视频数
	CountVideosInFolder(ctx context.Context, folderID uint64) (int64, error)
}
