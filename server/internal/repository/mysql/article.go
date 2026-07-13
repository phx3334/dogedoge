package mysql

import (
	"context"
	"time"

	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/repository/interfaces"

	"gorm.io/gorm"
)

// 编译期接口校验
var _ interfaces.ArticleRepository = (*ArticleRepo)(nil)

// ArticleRepo 专栏文章的 MySQL 数据存储，实现 ArticleRepository 接口。
type ArticleRepo struct {
	db *gorm.DB
}

// NewArticleRepo 创建 ArticleRepo 实例。
func NewArticleRepo(db *gorm.DB) *ArticleRepo {
	return &ArticleRepo{db: db}
}

// Create 插入一条文章记录。
func (r *ArticleRepo) Create(ctx context.Context, a *database.Article) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Create(a).Error
}

// FindByID 根据文章 ID 查询，命中 idx_article_status / 主键索引。
func (r *ArticleRepo) FindByID(ctx context.Context, id uint64) (*database.Article, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var article database.Article
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&article).Error; err != nil {
		return nil, err
	}
	return &article, nil
}

// UpdateStatus 更新文章状态。
//
// 当 status == "published" 时同步写入 published_at = NOW()，
// 使用 Updates + map 避免结构体 Updates 自动忽略零值字段
//（time.Time 的零值会被 GORM 视为零值而忽略，无法显式置 NOW）。
//
// updated_at = NOW() 也通过 map 显式写入，保证索引 idx_article_created
// 的排序字段随状态变更而推进。
func (r *ArticleRepo) UpdateStatus(ctx context.Context, id uint64, status string) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	updates := map[string]interface{}{
		"status":     status,
		"updated_at": time.Now(),
	}
	if status == "published" {
		now := time.Now()
		updates["published_at"] = &now
	}

	return r.db.WithContext(ctx).
		Model(&database.Article{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// IncrementViewCount 原子自增文章浏览量，避免并发读改写丢失更新。
func (r *ArticleRepo) IncrementViewCount(ctx context.Context, id uint64) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	return r.db.WithContext(ctx).
		Model(&database.Article{}).
		Where("id = ?", id).
		UpdateColumn("view_count", gorm.Expr("view_count + 1")).Error
}

// FindPublishedArticlesByAuthorIDs 批量查询指定作者列表的已发布文章，按发布时间倒序
func (r *ArticleRepo) FindPublishedArticlesByAuthorIDs(ctx context.Context, authorIDs []string, limit, offset int) ([]database.Article, error) {
	if len(authorIDs) == 0 {
		return nil, nil
	}
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var articles []database.Article
	err := r.db.WithContext(ctx).
		Select("id, user_id, title, cover_url, view_count, comment_count, created_at, published_at").
		Where("user_id IN ? AND status = ?", authorIDs, "published").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&articles).Error
	return articles, err
}
