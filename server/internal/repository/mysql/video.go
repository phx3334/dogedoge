package mysql

import (
	"context"
	"time"

	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/repository/interfaces"

	"gorm.io/gorm"
)

// 编译期接口校验
var (
	_ interfaces.VideoRepository      = (*VideoRepo)(nil)
	_ interfaces.VideoDraftRepository = (*VideoRepo)(nil)
)

// VideoRepo 视频表的 MySQL 数据存储，实现 VideoRepository 接口。
type VideoRepo struct {
	db *gorm.DB
}

// NewVideoRepo 创建 VideoRepo 实例。
func NewVideoRepo(db *gorm.DB) *VideoRepo {
	return &VideoRepo{db: db}
}

// FindPublishedVideos 返回已发布视频的基础查询（未执行）。
// 返回 *gorm.DB 供 PaginateRepo 进一步添加分页条件后执行。
func (v *VideoRepo) FindPublishedVideos() *gorm.DB {
	// 修复：每次返回新 Session，避免多 goroutine 共享同一个 *gorm.DB 对象导致竞态
	return v.db.Session(&gorm.Session{}).Model(&database.Video{}).Where("status = ?", "published")
}

// FindPublishedVideosWithZone 返回指定分区已发布视频的基础查询。
func (v *VideoRepo) FindPublishedVideosWithZone(zone string) *gorm.DB {
	return v.FindPublishedVideos().Where("zone = ?", zone)
}

// FindPublishedVideosByIDs 根据视频 ID 列表批量查询已发布的视频。
// 用于 Redis 缓存未命中时批量回源 MySQL。
// ids 为空时直接返回空切片，避免 IN () 语法错误。
// 只 Select 缓存回填所需的字段，减少不必要的数据传输。
func (v *VideoRepo) FindPublishedVideosByIDs(ctx context.Context, ids []uint) ([]database.Video, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	if len(ids) == 0 {
		return nil, nil
	}
	var videos []database.Video
	err := v.db.WithContext(ctx).
		Select("id, author_id, duration_sec, title, description, zone, play_url, cover_url, play_count, comments_count, likes_count, fav_count, coin_count, danmaku_count, comments_closed, danmaku_closed, created_at").
		Where("id IN ? AND status = ?", ids, "published").
		Find(&videos).Error
	return videos, err
}

// FindAllPublishedVideoIDs 查询所有已发布视频的关键字段。
// Select 包含热度计算、ZSet 重建和 Hash 预热所需的全部字段，减少数据传输量。
// 用于定时任务全量重建 ZSet 和缓存 Hash。
func (v *VideoRepo) FindAllPublishedVideoIDs(ctx context.Context) ([]database.Video, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var videos []database.Video
	err := v.db.WithContext(ctx).
		Select("id, author_id, play_count, comments_count, likes_count, fav_count, coin_count, popularity, created_at, cover_url, duration_sec, title").
		Where("status = ?", "published").
		Find(&videos).Error
	return videos, err
}

// UpdatePopularity 更新视频的 popularity 热度字段。
// 在定时任务重建 ZSet 后，将计算出的热度分同步回 MySQL 的 popularity 列，
// 保证 MySQL 中的游标分页（降级路径）也使用一致的热度排序。
func (v *VideoRepo) UpdatePopularity(ctx context.Context, id uint, popularity int64) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	return v.db.WithContext(ctx).Model(&database.Video{}).Where("id = ?", id).Update("popularity", popularity).Error
}

// FindPublishedVideosByAuthorID 查询指定作者的已发布视频，按创建时间倒序
// 使用 idx_author_time 复合索引（author_id DESC, created_at DESC）
func (v *VideoRepo) FindPublishedVideosByAuthorID(ctx context.Context, authorID string, limit, offset int) ([]database.Video, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var videos []database.Video
	err := v.db.WithContext(ctx).
		Select("id, author_id, duration_sec, title, cover_url, play_count, comments_count, fav_count, created_at").
		Where("author_id = ? AND status = ?", authorID, "published").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&videos).Error
	return videos, err
}

// FindPublishedVideosByAuthorIDs 批量查询指定作者列表的已发布视频，按创建时间倒序
func (v *VideoRepo) FindPublishedVideosByAuthorIDs(ctx context.Context, authorIDs []string, limit, offset int) ([]database.Video, error) {
	if len(authorIDs) == 0 {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var videos []database.Video
	err := v.db.WithContext(ctx).
		Select("id, author_id, duration_sec, title, cover_url, play_count, comments_count, fav_count, created_at").
		Where("author_id IN ? AND status = ?", authorIDs, "published").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&videos).Error
	return videos, err
}

func (v *VideoRepo) FindVideoByID(ctx context.Context, id uint) (*database.Video, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var video database.Video
	if err := v.db.WithContext(ctx).Where("id = ?", id).First(&video).Error; err != nil {
		return nil, err
	}
	return &video, nil
}

// DeleteVideo 软删除视频（设置 deleted_at）。
// 依赖 database.Video 上的 gorm.DeletedAt 字段：GORM 在 Delete 时不会物理删除，
// 而是写入 deleted_at；同时所有 Find/First 查询自动过滤已删除记录。
func (v *VideoRepo) DeleteVideo(ctx context.Context, id uint) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return v.db.WithContext(ctx).Delete(&database.Video{}, id).Error
}

// CreateDraft 插入一条 status=draft 的 Video 记录。
// 调用方负责设置 Status / DraftRawPath / DraftCoverPath / Title / Description / Zone / TagsJSON / AuthorID，
// GORM 在 Create 成功后会回填 v.ID。
func (v *VideoRepo) CreateDraft(ctx context.Context, video *database.Video) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return v.db.WithContext(ctx).Create(video).Error
}

// UpdateTranscodeResult 转码成功后回写 video_url / cover_url / duration_sec / status。
// 使用 Updates(map[...]) 而不是 Updates(struct)，确保零值字段（如空 cover_url）也会被写入。
func (v *VideoRepo) UpdateTranscodeResult(ctx context.Context, videoID uint, videoURL, coverURL string, duration float64, status string) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return v.db.WithContext(ctx).
		Model(&database.Video{}).
		Where("id = ?", videoID).
		Updates(map[string]interface{}{
			"play_url":     videoURL,
			"cover_url":    coverURL,
			"duration_sec": duration,
			"status":       status,
		}).Error
}

// UpdateTranscodeFailure 转码失败时将 status 置为 failed 并写入 fail_reason。
// fail_reason 由调用方截断至 2000 字符以内（对应 database.Video.FailReason 的 gorm size:2000）。
func (v *VideoRepo) UpdateTranscodeFailure(ctx context.Context, videoID uint, failReason string) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return v.db.WithContext(ctx).
		Model(&database.Video{}).
		Where("id = ?", videoID).
		Updates(map[string]interface{}{
			"status":      "failed",
			"fail_reason": failReason,
		}).Error
}

// FindDraftByID 按 ID 查询 Video 草稿记录，未命中时返回 gorm.ErrRecordNotFound。
// 用于前端轮询 status 接口和 worker 转码前读取 DraftRawPath / DraftCoverPath。
func (v *VideoRepo) FindDraftByID(ctx context.Context, id uint) (*database.Video, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var video database.Video
	if err := v.db.WithContext(ctx).Where("id = ?", id).First(&video).Error; err != nil {
		return nil, err
	}
	return &video, nil
}

// IncrementFavCount 视频 fav_count +delta
// 用 gorm.Expr 让 SQL 端做加法，避免读-改-写竞态
func (v *VideoRepo) IncrementFavCount(ctx context.Context, videoID uint, delta int) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return v.db.WithContext(ctx).Model(&database.Video{}).
		Where("id = ?", videoID).
		UpdateColumn("fav_count", gorm.Expr("fav_count + ?", delta)).Error
}

// DecrementFavCount 视频 fav_count -delta，不会减到负数
// 用 GREATEST(fav_count - ?, 0) 兜底，避免取消收藏竞态导致 -1
func (v *VideoRepo) DecrementFavCount(ctx context.Context, videoID uint, delta int) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return v.db.WithContext(ctx).Model(&database.Video{}).
		Where("id = ?", videoID).
		UpdateColumn("fav_count", gorm.Expr("GREATEST(fav_count - ?, 0)", delta)).Error
}

// IncrementCoinCount 视频 coin_count +delta（投币时调用）
func (v *VideoRepo) IncrementCoinCount(ctx context.Context, videoID uint, delta int) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return v.db.WithContext(ctx).Model(&database.Video{}).
		Where("id = ?", videoID).
		UpdateColumn("coin_count", gorm.Expr("coin_count + ?", delta)).Error
}

// IncrementDanmakuCount 视频 danmaku_count +delta（发送弹幕时调用）
func (v *VideoRepo) IncrementDanmakuCount(ctx context.Context, videoID uint, delta int) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return v.db.WithContext(ctx).Model(&database.Video{}).
		Where("id = ?", videoID).
		UpdateColumn("danmaku_count", gorm.Expr("danmaku_count + ?", delta)).Error
}
