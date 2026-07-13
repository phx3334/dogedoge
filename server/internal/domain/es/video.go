package es

import (
	"encoding/json"
	"strconv"

	"fake_tiktok/internal/domain/database"

	"github.com/elastic/go-elasticsearch/v8/typedapi/types"
)

// CreatedAtFormat ES 中 created_at 字段的日期格式（ES 格式语法）
// 与 CreatedAtGoLayout 保持一致，修改时必须同步更新
const CreatedAtFormat = "yyyy-MM-dd HH:mm"

// CreatedAtGoLayout Go time.Parse 使用的布局字符串
// 与 CreatedAtFormat 保持一致，修改时必须同步更新
const CreatedAtGoLayout = "2006-01-02 15:04"

// VideoDocument 视频文档结构体
type VideoDocument struct {
	ID             string   `json:"id"`              // 视频ID (ES文档ID和视频ID一致)
	Title          string   `json:"title"`           // 视频标题
	Description    string   `json:"description"`     // 视频描述
	Tags           []string `json:"tags"`            // 视频标签
	AuthorUsername string   `json:"author_username"` // 作者用户名
	AuthorID       string   `json:"author_id"`       // 作者ID
	CoverURL       string   `json:"cover_url"`       // 封面地址
	CreatedAt      string   `json:"created_at"`      // 创建时间
	LikesCount     int64    `json:"likes_count"`     // 点赞数
	CommentsCount  int64    `json:"comments_count"`  // 评论数
	Popularity     int64    `json:"popularity"`      // 热度
}

// VideoIndex 视频 ES 索引
func GetVideoIndex() string {
	return "videos"
}

// ptr 辅助函数，创建字符串指针
func ptr(s string) *string {
	return &s
}

// VideoMapping 定义 Elasticsearch 中视频索引的映射结构
// 作用：告诉 Elasticsearch 如何索引和存储视频的各个字段，优化搜索和排序性能
// 返回值：
//
//	*types.TypeMapping: 视频索引的映射定义
func GetVideoMapping() *types.TypeMapping {
	return &types.TypeMapping{
		Properties: map[string]types.Property{
			// 视频ID，关键字类型，用于精确匹配
			"id": types.KeywordProperty{},
			// 视频标题，文本类型，使用ik分词器，支持中文全文搜索
			// ik_max_word：索引时细粒度分词；ik_smart：搜索时粗粒度分词，提升召回率
			"title": types.TextProperty{
				Analyzer:       ptr("standard"),
				SearchAnalyzer: ptr("standard"),
				Fields: map[string]types.Property{
					"keyword": types.KeywordProperty{},
				},
			},
			// 视频描述，文本类型，使用ik分词器，支持中文全文搜索
			// ik_max_word：索引时细粒度分词；ik_smart：搜索时粗粒度分词，提升召回率
			"description": types.TextProperty{
				Analyzer:       ptr("standard"),
				SearchAnalyzer: ptr("standard"),
				Fields: map[string]types.Property{
					"keyword": types.KeywordProperty{},
				},
			},
			// 视频标签，文本类型，使用ik分词器，搜索权重最高
			// 添加 keyword 子字段支持精确匹配（如按标签筛选）和聚合统计
			// 标签虽然是离散词，但保留 ik 分词器以支持标签内包含短语时的分词搜索
			"tags": types.TextProperty{
				Analyzer:       ptr("standard"),
				SearchAnalyzer: ptr("standard"),
				Fields: map[string]types.Property{
					"keyword": types.KeywordProperty{},
				},
			},
			// 作者用户名，文本类型，支持全文搜索
			"author_username": types.TextProperty{},
			// 作者ID，关键字类型，用于精确匹配
			"author_id": types.KeywordProperty{},
			// 封面地址，关键字类型，用于精确匹配
			"cover_url": types.KeywordProperty{},
			// 创建时间，日期类型
			// 使用 CreatedAtFormat 常量，确保与 Go 解析布局 CreatedAtGoLayout 保持一致
			"created_at": types.DateProperty{
				NullValue: nil,
				Format:    ptr(CreatedAtFormat),
			},
			// 点赞数，整数类型，用于排序和统计
			"likes_count": types.IntegerNumberProperty{},
			// 评论数，整数类型，用于排序和统计
			"comments_count": types.IntegerNumberProperty{},
			// 热度，整数类型，用于排序和统计
		"popularity": types.IntegerNumberProperty{},
		},
	}
}

// NewVideoDocumentFromVideo 把 MySQL Video 实体转换为 ES 文档。
// authorUsername 需要调用方额外查表传入（Video 实体只有 AuthorID，没有 username）。
// TagsJSON（JSON 字符串）会被反序列化为 []string；解析失败时 tags 为空切片。
func NewVideoDocumentFromVideo(v database.Video, authorUsername string) *VideoDocument {
	var tags []string
	if v.TagsJSON != "" {
		_ = json.Unmarshal([]byte(v.TagsJSON), &tags)
	}
	if tags == nil {
		tags = []string{}
	}
	return &VideoDocument{
		ID:             strconv.FormatUint(uint64(v.ID), 10),
		Title:          v.Title,
		Description:    v.Description,
		Tags:           tags,
		AuthorUsername: authorUsername,
		AuthorID:       v.AuthorID,
		CoverURL:       v.CoverURL,
		CreatedAt:      v.CreatedAt.Format(CreatedAtGoLayout),
		LikesCount:     v.LikesCount,
		CommentsCount:  v.CommentsCount,
		Popularity:     v.Popularity,
	}
}
