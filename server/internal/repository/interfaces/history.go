package interfaces

import (
	"context"

	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/dto/response"
)

// VideoViewHistoryRepository 视频观看历史接口
type VideoViewHistoryRepository interface {
	// Upsert 创建或更新观看历史
	// 同一 user+video 唯一索引保证幂等：存在则更新 ProgressSec/ViewedAt，不存在则插入
	Upsert(ctx context.Context, h *database.VideoViewHistory) error

	// ListByUserWithVideo 分页查询用户观看历史并 JOIN 视频表和作者表
	ListByUserWithVideo(ctx context.Context, userID uint64, page, pageSize int) ([]response.VideoHistoryItem, int64, error)

	// Delete 删除单条观看历史（仅当属于 userID）
	// 返回 deleted=true 表示真正删除
	Delete(ctx context.Context, userID uint64, videoID uint64) (bool, error)

	// ClearAll 清空用户所有观看历史
	ClearAll(ctx context.Context, userID uint64) error
}

// ArticleViewHistoryRepository 文章阅读历史接口
type ArticleViewHistoryRepository interface {
	// Upsert 创建或更新阅读历史
	Upsert(ctx context.Context, h *database.ArticleViewHistory) error

	// ListByUserWithArticle 分页查询文章阅读历史并 JOIN 文章表
	ListByUserWithArticle(ctx context.Context, userID uint64, page, pageSize int) ([]response.ArticleHistoryItem, int64, error)

	// Delete 删除单条阅读历史
	Delete(ctx context.Context, userID uint64, articleID uint64) (bool, error)

	// ClearAll 清空用户所有阅读历史
	ClearAll(ctx context.Context, userID uint64) error
}

// UserSearchHistoryRepository 用户搜索历史接口
type UserSearchHistoryRepository interface {
	// Upsert 创建或更新搜索历史（同 keyword_norm 则更新时间）
	Upsert(ctx context.Context, h *database.UserSearchHistory) error

	// ListByUser 查询用户最近 N 条搜索历史（按 UpdatedAt 倒序）
	ListByUser(ctx context.Context, userID uint64, limit int) ([]database.UserSearchHistory, error)

	// Delete 删除单条搜索历史
	Delete(ctx context.Context, userID uint64, keyword string) (bool, error)

	// ClearAll 清空用户所有搜索历史
	ClearAll(ctx context.Context, userID uint64) error
}
