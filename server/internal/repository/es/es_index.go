package es_repo

import (
	"context"
	"fake_tiktok/internal/repository/interfaces"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/typedapi/types"
)

var _ interfaces.SearchIndexRepository = (*SearchIndexRepo)(nil)

type SearchIndexRepo struct {
	client *elasticsearch.TypedClient
}

func NewSearchIndexRepo(client *elasticsearch.TypedClient) *SearchIndexRepo {
	return &SearchIndexRepo{client: client}
}

// CreateIndex 创建 Elasticsearch 索引。
// 使用调用方传入的 context 支持超时控制和链路追踪，
// 而非使用 context.TODO() 导致操作无法被取消。
func (r *SearchIndexRepo) CreateIndex(ctx context.Context, indexName string, mapping *types.TypeMapping) error {
	_, err := r.client.Indices.Create(indexName).Mappings(mapping).Do(ctx)
	return err
}

// DeleteIndex 删除 Elasticsearch 索引。
func (r *SearchIndexRepo) DeleteIndex(ctx context.Context, indexName string) error {
	_, err := r.client.Indices.Delete(indexName).Do(ctx)
	return err
}

// IndexExists 检查 Elasticsearch 索引是否存在。
func (r *SearchIndexRepo) IndexExists(ctx context.Context, indexName string) (bool, error) {
	exists, err := r.client.Indices.Exists(indexName).Do(ctx)
	return exists, err
}
