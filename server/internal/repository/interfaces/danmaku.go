package interfaces

import (
	"context"
	"fake_tiktok/internal/domain/database"
)

type DanmakuRepository interface {
	FindByVideoID(ctx context.Context, videoID uint64) ([]database.Danmaku, error)
	Create(ctx context.Context, danmaku *database.Danmaku) error
}
