package interfaces

import (
	"context"

	"fake_tiktok/internal/domain/database"
)

// ArticleRepository 专栏文章数据接口
//
// 所有方法接收 context.Context 用于超时控制与请求取消，
// 与项目其他 repository 接口保持一致。
type ArticleRepository interface {
	// Create 插入一条文章记录（草稿或发布）
	Create(ctx context.Context, a *database.Article) error

	// FindByID 按主键查询文章
	FindByID(ctx context.Context, id uint64) (*database.Article, error)

	// UpdateStatus 更新文章状态；status 为 "published" 时同步写入 published_at=NOW()
	UpdateStatus(ctx context.Context, id uint64, status string) error

	// IncrementViewCount 文章浏览量原子 +1
	IncrementViewCount(ctx context.Context, id uint64) error

	// FindPublishedArticlesByAuthorIDs 批量查询指定作者列表的已发布文章，按发布时间倒序
	// 用于动态 Feed 流
	FindPublishedArticlesByAuthorIDs(ctx context.Context, authorIDs []string, limit, offset int) ([]database.Article, error)
}
