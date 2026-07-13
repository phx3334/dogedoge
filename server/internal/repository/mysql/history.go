package mysql

import (
	"context"
	"time"

	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/dto/response"
	"fake_tiktok/internal/repository/interfaces"

	"gorm.io/gorm"
)

var (
	_ interfaces.VideoViewHistoryRepository   = (*VideoViewHistoryRepo)(nil)
	_ interfaces.ArticleViewHistoryRepository = (*ArticleViewHistoryRepo)(nil)
	_ interfaces.UserSearchHistoryRepository  = (*UserSearchHistoryRepo)(nil)
)

// ---------------------------------------------------------------------------
// VideoViewHistoryRepo —— 视频观看历史
// ---------------------------------------------------------------------------

type VideoViewHistoryRepo struct {
	db *gorm.DB
}

func NewVideoViewHistoryRepo(db *gorm.DB) *VideoViewHistoryRepo {
	return &VideoViewHistoryRepo{db: db}
}

// Upsert 创建或更新观看历史
// 同 user+video 唯一索引保证：存在则更新 ProgressSec/ViewedAt，不存在则插入
func (r *VideoViewHistoryRepo) Upsert(ctx context.Context, h *database.VideoViewHistory) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	now := time.Now()
	h.ViewedAt = now
	h.UpdatedAt = now

	// 先尝试插入；冲突则更新
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND video_id = ?", h.UserID, h.VideoID).
		Assign(map[string]interface{}{
			"progress_sec": h.ProgressSec,
			"duration_sec": h.DurationSec,
			"device":       h.Device,
			"viewed_at":    now,
			"updated_at":   now,
		}).
		FirstOrCreate(h)
	return result.Error
}

// ListByUserWithVideo 分页查询用户观看历史并 JOIN 视频表和用户表
// 返回包含视频标题、封面、时长、UP 主名的完整信息
func (r *VideoViewHistoryRepo) ListByUserWithVideo(ctx context.Context, userID uint64, page, pageSize int) ([]response.VideoHistoryItem, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var total int64
	if err := r.db.WithContext(ctx).Model(&database.VideoViewHistory{}).
		Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 || page < 1 || pageSize < 1 {
		return nil, total, nil
	}
	offset := (page - 1) * pageSize

	type joinResult struct {
		VideoID     uint64    `gorm:"column:video_id"`
		ProgressSec float64   `gorm:"column:progress_sec"`
		DurationSec float64   `gorm:"column:duration_sec"`
		Device      string    `gorm:"column:device"`
		ViewedAt    time.Time `gorm:"column:viewed_at"`
		Title       string    `gorm:"column:title"`
		CoverURL    string    `gorm:"column:cover_url"`
		Duration    float64   `gorm:"column:duration"`
		UpName      string    `gorm:"column:up_name"`
	}

	var rows []joinResult
	err := r.db.WithContext(ctx).
		Table("video_view_histories AS h").
		Select("h.video_id, h.progress_sec, h.duration_sec, h.device, h.viewed_at, v.title, v.cover_url, v.duration_sec AS duration, a.username AS up_name").
		Joins("LEFT JOIN videos v ON v.id = h.video_id").
		Joins("LEFT JOIN accounts a ON a.id = v.author_id").
		Where("h.user_id = ?", userID).
		Order("h.viewed_at DESC").
		Offset(offset).Limit(pageSize).
		Scan(&rows).Error
	if err != nil {
		return nil, total, err
	}

	items := make([]response.VideoHistoryItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, response.VideoHistoryItem{
			VideoID:     row.VideoID,
			ProgressSec: row.ProgressSec,
			DurationSec: row.DurationSec,
			Device:      row.Device,
			ViewedAt:    row.ViewedAt,
			Title:       row.Title,
			CoverURL:    row.CoverURL,
			Duration:    row.Duration,
			UpName:      row.UpName,
		})
	}
	return items, total, nil
}

// Delete 删除单条观看历史
func (r *VideoViewHistoryRepo) Delete(ctx context.Context, userID uint64, videoID uint64) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	result := r.db.WithContext(ctx).
		Where("user_id = ? AND video_id = ?", userID, videoID).
		Delete(&database.VideoViewHistory{})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// ClearAll 清空用户所有观看历史
func (r *VideoViewHistoryRepo) ClearAll(ctx context.Context, userID uint64) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&database.VideoViewHistory{}).Error
}

// ---------------------------------------------------------------------------
// ArticleViewHistoryRepo —— 文章阅读历史
// ---------------------------------------------------------------------------

type ArticleViewHistoryRepo struct {
	db *gorm.DB
}

func NewArticleViewHistoryRepo(db *gorm.DB) *ArticleViewHistoryRepo {
	return &ArticleViewHistoryRepo{db: db}
}

func (r *ArticleViewHistoryRepo) Upsert(ctx context.Context, h *database.ArticleViewHistory) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	now := time.Now()
	h.ViewedAt = now
	h.UpdatedAt = now

	result := r.db.WithContext(ctx).
		Where("user_id = ? AND article_id = ?", h.UserID, h.ArticleID).
		Assign(map[string]interface{}{
			"device":     h.Device,
			"viewed_at":  now,
			"updated_at": now,
		}).
		FirstOrCreate(h)
	return result.Error
}

// ListByUserWithArticle 分页查询文章阅读历史并 JOIN 文章表
func (r *ArticleViewHistoryRepo) ListByUserWithArticle(ctx context.Context, userID uint64, page, pageSize int) ([]response.ArticleHistoryItem, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var total int64
	if err := r.db.WithContext(ctx).Model(&database.ArticleViewHistory{}).
		Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 || page < 1 || pageSize < 1 {
		return nil, total, nil
	}
	offset := (page - 1) * pageSize

	type joinResult struct {
		ArticleID uint64    `gorm:"column:article_id"`
		Device    string    `gorm:"column:device"`
		ViewedAt  time.Time `gorm:"column:viewed_at"`
		Title     string    `gorm:"column:title"`
		CoverURL  string    `gorm:"column:cover_url"`
	}

	var rows []joinResult
	err := r.db.WithContext(ctx).
		Table("article_view_histories AS h").
		Select("h.article_id, h.device, h.viewed_at, a.title, a.cover_url").
		Joins("LEFT JOIN articles a ON a.id = h.article_id").
		Where("h.user_id = ?", userID).
		Order("h.viewed_at DESC").
		Offset(offset).Limit(pageSize).
		Scan(&rows).Error
	if err != nil {
		return nil, total, err
	}

	items := make([]response.ArticleHistoryItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, response.ArticleHistoryItem{
			ArticleID: row.ArticleID,
			Device:    row.Device,
			ViewedAt:  row.ViewedAt,
			Title:     row.Title,
			CoverURL:  row.CoverURL,
		})
	}
	return items, total, nil
}

func (r *ArticleViewHistoryRepo) Delete(ctx context.Context, userID uint64, articleID uint64) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	result := r.db.WithContext(ctx).
		Where("user_id = ? AND article_id = ?", userID, articleID).
		Delete(&database.ArticleViewHistory{})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (r *ArticleViewHistoryRepo) ClearAll(ctx context.Context, userID uint64) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&database.ArticleViewHistory{}).Error
}

// ---------------------------------------------------------------------------
// UserSearchHistoryRepo —— 用户搜索历史
// ---------------------------------------------------------------------------

type UserSearchHistoryRepo struct {
	db *gorm.DB
}

func NewUserSearchHistoryRepo(db *gorm.DB) *UserSearchHistoryRepo {
	return &UserSearchHistoryRepo{db: db}
}

// Upsert 创建或更新搜索历史（同 keyword_norm 则更新时间）
func (r *UserSearchHistoryRepo) Upsert(ctx context.Context, h *database.UserSearchHistory) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	now := time.Now()
	h.UpdatedAt = now

	result := r.db.WithContext(ctx).
		Where("user_id = ? AND keyword_norm = ?", h.UserID, h.KeywordNorm).
		Assign(map[string]interface{}{
			"keyword":    h.Keyword,
			"updated_at": now,
		}).
		FirstOrCreate(h)
	return result.Error
}

// ListByUser 查询用户最近 N 条搜索历史（按 UpdatedAt 倒序）
func (r *UserSearchHistoryRepo) ListByUser(ctx context.Context, userID uint64, limit int) ([]database.UserSearchHistory, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if limit <= 0 {
		limit = 20
	}
	var items []database.UserSearchHistory
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("updated_at DESC").
		Limit(limit).
		Find(&items).Error
	return items, err
}

// Delete 删除单条搜索历史
func (r *UserSearchHistoryRepo) Delete(ctx context.Context, userID uint64, keyword string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	result := r.db.WithContext(ctx).
		Where("user_id = ? AND keyword_norm = ?", userID, keyword).
		Delete(&database.UserSearchHistory{})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// ClearAll 清空用户所有搜索历史
func (r *UserSearchHistoryRepo) ClearAll(ctx context.Context, userID uint64) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&database.UserSearchHistory{}).Error
}
