package interfaces

import "context"

type InteractionStatus struct {
	IsLiked     bool
	IsFavorited bool
	CoinCount   int64
	IsFollowed  bool
}

type InteractionRepository interface {
	GetUserVideoInteraction(ctx context.Context, userID string, videoID uint) (*InteractionStatus, error)
	GetFansCount(ctx context.Context, userID string) (int64, error)
	GetFollowingCount(ctx context.Context, userID string) (int64, error)
	IsUserFollowed(ctx context.Context, followerID, followeeID string) (bool, error)
	// IsMutualFollow 判断 a、b 是否互相关注（双向都已关注）
	IsMutualFollow(ctx context.Context, a, b string) (bool, error)
	// CreateVideoLike 同步写入用户点赞记录到 MySQL VideoLike 表
	// 返回值 created=true 表示真正新建了记录，created=false 表示记录已存在（幂等）
	// 调用方根据 created 决定是否发布 MQ 增量，避免 SET 过期后重复点赞导致 likes_count 虚高
	CreateVideoLike(ctx context.Context, userID string, videoID uint) (created bool, err error)

	// DeleteVideoLike 删除用户点赞记录（取消点赞）
	// 返回值 deleted=true 表示真正删除了记录，deleted=false 表示原本就没有点赞（幂等）
	DeleteVideoLike(ctx context.Context, userID string, videoID uint) (deleted bool, err error)

	// CreateFollow 创建关注关系
	// 返回值 created=true 表示真正新建，false 表示已存在（幂等）
	CreateFollow(ctx context.Context, followerID, followeeID string) (created bool, err error)
	// DeleteFollow 删除关注关系
	// 返回值 deleted=true 表示真正删除，false 表示原本未关注（幂等）
	DeleteFollow(ctx context.Context, followerID, followeeID string) (deleted bool, err error)

	// ListFollowers 分页查询某用户的粉丝列表
	ListFollowers(ctx context.Context, userID string, page, pageSize int) ([]string, int64, error)
	// ListFollowing 分页查询某用户关注的用户列表
	ListFollowing(ctx context.Context, userID string, page, pageSize int) ([]string, int64, error)
}

// FavoriteRepository 视频收藏数据接口（独立于 FavoriteFolderRepository 的文件夹管理）
type FavoriteRepository interface {
	// AddFavorite 收藏视频到指定收藏夹
	// 返回值 created=true 表示真正新增收藏，false 表示已存在（幂等）
	AddFavorite(ctx context.Context, userID string, videoID uint, folderID uint64) (created bool, err error)
	// RemoveFavorite 从所有收藏夹移除该视频
	// 返回值 deleted=true 表示真正删除，false 表示原本未收藏（幂等）
	RemoveFavorite(ctx context.Context, userID string, videoID uint) (deleted bool, err error)
	// RemoveFromFolder 从指定收藏夹移除该视频（保留其他收藏夹中的）
	// 返回值 deleted=true 表示真正删除，false 表示该收藏夹中原本没有此视频（幂等）
	RemoveFromFolder(ctx context.Context, userID string, videoID uint, folderID uint64) (deleted bool, err error)
	// ListFavoritesByFolder 分页查询收藏夹中的视频
	ListFavoritesByFolder(ctx context.Context, userID string, folderID uint64, page, pageSize int) ([]uint, int64, error)
	// EnsureDefaultFolder 确保用户有默认收藏夹，返回默认收藏夹 ID
	EnsureDefaultFolder(ctx context.Context, userID string) (uint64, error)
}
