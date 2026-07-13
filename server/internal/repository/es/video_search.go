package es_repo

import (
	"context"
	"fmt"

	es "fake_tiktok/internal/domain/es"
	"fake_tiktok/internal/repository/interfaces"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v8/typedapi/types"
	"github.com/elastic/go-elasticsearch/v8/typedapi/types/enums/sortorder"
	"go.uber.org/zap"
)

var _ interfaces.VideoSearchRepository = (*VideoSearchRepo)(nil)

type VideoSearchRepo struct {
	client *elasticsearch.TypedClient
	logger *zap.Logger
}

func NewVideoSearchRepo(client *elasticsearch.TypedClient, logger *zap.Logger) *VideoSearchRepo {
	return &VideoSearchRepo{client: client, logger: logger}
}

// IndexVideo 索引一个视频文档到 ES
func (r *VideoSearchRepo) IndexVideo(ctx context.Context, doc *es.VideoDocument) error {
	_, err := r.client.Index(es.GetVideoIndex()).Id(doc.ID).Document(doc).Do(ctx)
	if err != nil {
		r.logger.Error("ES 索引视频文档失败", zap.String("video_id", doc.ID), zap.Error(err))
		return fmt.Errorf("index video failed: %w", err)
	}
	return nil
}

// DeleteVideo 从 ES 中删除一个视频文档
func (r *VideoSearchRepo) DeleteVideo(ctx context.Context, videoID string) error {
	_, err := r.client.Delete(es.GetVideoIndex(), videoID).Do(ctx)
	if err != nil {
		r.logger.Error("ES 删除视频文档失败", zap.String("video_id", videoID), zap.Error(err))
		return fmt.Errorf("delete video failed: %w", err)
	}
	return nil
}

// SearchVideos 按关键词搜索视频，使用加权评分和游标分页
// 加权策略：tags(5.0) > description(2.0) > title(1.0)
// 排序：_score desc, id asc（保证游标分页稳定性）
//
// 搜索流程第 1 步：ES 只负责关键词匹配，返回视频 ID 列表和分页游标。
// 视频详情由调用方通过 Redis 缓存批量组装（第 2 步），
// Redis 未命中时走 MySQL 回源（第 3 步）。
// 这样 ES 不承担详情组装职责，各组件职责清晰。
func (r *VideoSearchRepo) SearchVideos(ctx context.Context, query string, cursor []types.FieldValue, limit int) ([]string, []types.FieldValue, error) {
	if limit <= 0 || limit > 30 {
		limit = 10
	}

	boost5 := float32(5.0)
	boost2 := float32(2.0)
	boost1 := float32(1.0)
	minShould := "1"
	sortDesc := sortorder.Desc
	sortAsc := sortorder.Asc

	req := &search.Request{
		Query: &types.Query{
			Bool: &types.BoolQuery{
				Should: []types.Query{
					{
						Match: map[string]types.MatchQuery{
							"tags": {Query: query, Boost: &boost5},
						},
					},
					{
						Match: map[string]types.MatchQuery{
							"description": {Query: query, Boost: &boost2},
						},
					},
					{
						Match: map[string]types.MatchQuery{
							"title": {Query: query, Boost: &boost1},
						},
					},
				},
				MinimumShouldMatch: &minShould,
			},
		},
		Sort: []types.SortCombinations{
			types.SortOptions{
				Score_: &types.ScoreSort{Order: &sortDesc},
			},
			types.SortOptions{
				SortOptions: map[string]types.FieldSort{
					"id": {Order: &sortAsc},
				},
			},
		},
		// 禁用 _source 返回：只返回文档 ID，不需要 _source 字段
		// ES 跳过 _source 提取和传输，减少网络 I/O 和序列化开销
		Source_: false,
		Size:    &limit,
	}

	if len(cursor) > 0 {
		req.SearchAfter = cursor
	}

	resp, err := r.client.Search().Index(es.GetVideoIndex()).Request(req).Do(ctx)
	if err != nil {
		r.logger.Error("ES 搜索视频失败", zap.String("query", query), zap.Error(err))
		return nil, nil, fmt.Errorf("search videos failed: %w", err)
	}

	// 从 ES 响应的最后一项 hit 取 Sort 值作为下一页游标
	// 必须在循环外取最后一项的 Sort，而非依赖最后一个成功解析的 hit，
	// 否则当最后一条记录解析失败时 nextCursor 会丢失，导致分页中断
	var nextCursor []types.FieldValue
	if hitsLen := len(resp.Hits.Hits); hitsLen > 0 && len(resp.Hits.Hits[hitsLen-1].Sort) > 0 {
		nextCursor = resp.Hits.Hits[hitsLen-1].Sort
	}

	// 只提取文档 ID，详情由 Redis/MySQL 层组装
	ids := make([]string, 0, len(resp.Hits.Hits))
	for _, hit := range resp.Hits.Hits {
		if hit.Id_ != nil && *hit.Id_ != "" {
			ids = append(ids, *hit.Id_)
		}
	}

	return ids, nextCursor, nil
}
