package interfaces

import (
	"context"

	"fake_tiktok/internal/domain/database"
)

// UserDynamicRepository 用户动态（图文）接口
type UserDynamicRepository interface {
	// Create 发布动态
	Create(ctx context.Context, d *database.UserDynamicText) error

	// FindByID 查询单条动态
	FindByID(ctx context.Context, id uint64) (*database.UserDynamicText, error)

	// ListByUser 分页查询指定用户的动态（按 CreatedAt 倒序）
	ListByUser(ctx context.Context, userID uint64, page, pageSize int) ([]database.UserDynamicText, int64, error)

	// ListFeed 分页查询关注用户的最新动态（按 CreatedAt 倒序）
	// userIDs 为空时返回空列表
	ListFeed(ctx context.Context, userIDs []string, page, pageSize int) ([]database.UserDynamicText, int64, error)

	// IncrementLikeCount 动态点赞数 +delta
	IncrementLikeCount(ctx context.Context, id uint64, delta int) error
	// IncrementCommentCount 动态评论数 +delta
	IncrementCommentCount(ctx context.Context, id uint64, delta int) error
}

// UserDynamicLikeRepository 动态点赞数据接口
type UserDynamicLikeRepository interface {
	// CreateLike 点赞动态；返回 created=true 表示真正新增
	CreateLike(ctx context.Context, userID uint64, dynamicID uint64) (bool, error)

	// DeleteLike 取消点赞动态；返回 deleted=true 表示真正删除
	DeleteLike(ctx context.Context, userID uint64, dynamicID uint64) (bool, error)

	// IsLiked 查询是否已点赞
	IsLiked(ctx context.Context, userID uint64, dynamicID uint64) (bool, error)

	// ListLikedByUser 分页查询用户点赞过的动态 ID
	ListLikedByUser(ctx context.Context, userID uint64, page, pageSize int) ([]uint64, int64, error)
}
