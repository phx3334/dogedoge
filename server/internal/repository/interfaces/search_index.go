package interfaces

import (
	"context"

	es "fake_tiktok/internal/domain/es"

	"github.com/elastic/go-elasticsearch/v8/typedapi/types"
)

// SearchIndexRepository 定义 Elasticsearch 索引管理接口。
// 所有方法接收 context.Context 以支持超时控制和链路追踪，
// 与其它 Repository 接口风格保持一致。
type SearchIndexRepository interface {
	CreateIndex(ctx context.Context, indexName string, mapping *types.TypeMapping) error
	DeleteIndex(ctx context.Context, indexName string) error
	IndexExists(ctx context.Context, indexName string) (bool, error)
}

// VideoSearchRepository 定义视频搜索接口。
// 提供视频文档的索引、删除和搜索功能，支持加权评分和游标分页。
//
// 搜索流程职责分离：
//   - 第 1 步（ES）：SearchVideos 只返回匹配的视频 ID 列表，不返回详情
//   - 第 2 步（Redis）：调用方通过 VideoCacheRepo 批量组装详情
//   - 第 3 步（MySQL）：Redis 未命中时通过 BackfillRepo 回源
type VideoSearchRepository interface {
	// IndexVideo 索引一个视频文档到 ES
	IndexVideo(ctx context.Context, doc *es.VideoDocument) error
	// DeleteVideo 从 ES 中删除一个视频文档
	DeleteVideo(ctx context.Context, videoID string) error
	// SearchVideos 按关键词搜索视频，只返回匹配的视频 ID 列表和分页游标
	// 视频详情由调用方通过 Redis 缓存批量组装，ES 不承担详情组装职责
	SearchVideos(ctx context.Context, query string, cursor []types.FieldValue, limit int) ([]string, []types.FieldValue, error)
}
