package interfaces

import (
	"context"

	"fake_tiktok/internal/dto/cache"
)

type UserCacheRepository interface {
	GetUserCache(ctx context.Context, userID string) (*cache.UserCacheData, error)
	BatchWriteUserCache(ctx context.Context, items []cache.UserCacheData)
	IncrementTotalPlayCount(ctx context.Context, userID string) error
	IncrementFansCount(ctx context.Context, userID string) error
	IncrementFollowingCount(ctx context.Context, userID string) error
	IncrementExperience(ctx context.Context, userID string, delta int64) error
	DeleteUserCache(ctx context.Context, userID string) error
}
