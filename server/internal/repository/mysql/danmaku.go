package mysql

import (
	"context"
	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/repository/interfaces"
	"gorm.io/gorm"
	"time"
)

var _ interfaces.DanmakuRepository = (*DanmakuRepo)(nil)

type DanmakuRepo struct {
	db *gorm.DB
}

func NewDanmakuRepo(db *gorm.DB) *DanmakuRepo {
	return &DanmakuRepo{db: db}
}

func (r *DanmakuRepo) FindByVideoID(ctx context.Context, videoID uint64) ([]database.Danmaku, error) {
	var danmakus []database.Danmaku
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := r.db.WithContext(ctx).Where("video_id = ?", videoID).Find(&danmakus).Error; err != nil {
		return nil, err
	}
	return danmakus, nil
}

func (r *DanmakuRepo) Create(ctx context.Context, danmaku *database.Danmaku) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return r.db.WithContext(ctx).Create(danmaku).Error
}
