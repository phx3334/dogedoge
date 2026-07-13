package interfaces

import (
	"context"

	"fake_tiktok/internal/domain/database"
)

// CommentTargetType 评论目标类型
type CommentTargetType string

const (
	CommentTargetVideo   CommentTargetType = "video"
	CommentTargetArticle CommentTargetType = "article"
	CommentTargetDynamic CommentTargetType = "dynamic"
)

// CommentTargetInfo 评论目标（视频/文章/动态）的评论相关元信息
//
// 用于 CreateComment 校验评论区是否关闭、是否精选模式，以及 DeleteComment
// 校验调用者是否为目标 UP 主。将目标查询内聚到评论 repo，避免 logic 层
// 依赖尚未存在的 ArticleRepo / DynamicRepo。
type CommentTargetInfo struct {
	CommentsClosed  bool
	CommentsCurated bool
	AuthorID        string
}

// VideoCommentRepository 视频评论数据接口
type VideoCommentRepository interface {
	Create(ctx context.Context, c *database.Comment) error
	FindByVideoID(ctx context.Context, videoID uint, page, pageSize int) ([]database.Comment, int64, error)
	FindReplies(ctx context.Context, parentID uint64, page, pageSize int) ([]database.Comment, int64, error)
	// FindRepliesByParentIDs 批量查询多个父评论的子回复（用于递归查楼中楼）
	FindRepliesByParentIDs(ctx context.Context, parentIDs []uint64, limit int) ([]database.Comment, int64, error)
	FindByID(ctx context.Context, id uint64) (*database.Comment, error)
	Delete(ctx context.Context, id uint64) error
	IncrementLikeCount(ctx context.Context, id uint64, delta int) error
	IncrementVideoCommentCount(ctx context.Context, videoID uint, delta int) error
	CountReplies(ctx context.Context, parentID uint64) (int64, error)
	// FindVideoTargetInfo 查询视频的评论相关元信息（评论区开关、精选模式、作者）
	FindVideoTargetInfo(ctx context.Context, videoID uint) (*CommentTargetInfo, error)
}

// ArticleCommentRepository 文章评论数据接口（方法签名与 VideoCommentRepository 一致但操作 article_comments 表）
type ArticleCommentRepository interface {
	Create(ctx context.Context, c *database.ArticleComment) error
	FindByArticleID(ctx context.Context, articleID uint64, page, pageSize int) ([]database.ArticleComment, int64, error)
	FindReplies(ctx context.Context, parentID uint64, page, pageSize int) ([]database.ArticleComment, int64, error)
	// FindRepliesByParentIDs 批量查询多个父评论的子回复（用于递归查楼中楼）
	FindRepliesByParentIDs(ctx context.Context, parentIDs []uint64, limit int) ([]database.ArticleComment, int64, error)
	FindByID(ctx context.Context, id uint64) (*database.ArticleComment, error)
	Delete(ctx context.Context, id uint64) error
	IncrementLikeCount(ctx context.Context, id uint64, delta int) error
	IncrementArticleCommentCount(ctx context.Context, articleID uint64, delta int) error
	CountReplies(ctx context.Context, parentID uint64) (int64, error)
	// FindArticleTargetInfo 查询文章的评论相关元信息
	FindArticleTargetInfo(ctx context.Context, articleID uint64) (*CommentTargetInfo, error)
}

// DynamicCommentRepository 动态评论数据接口
type DynamicCommentRepository interface {
	Create(ctx context.Context, c *database.DynamicComment) error
	FindByDynamicID(ctx context.Context, dynamicID uint64, page, pageSize int) ([]database.DynamicComment, int64, error)
	FindReplies(ctx context.Context, parentID uint64, page, pageSize int) ([]database.DynamicComment, int64, error)
	// FindRepliesByParentIDs 批量查询多个父评论的子回复（用于递归查楼中楼）
	FindRepliesByParentIDs(ctx context.Context, parentIDs []uint64, limit int) ([]database.DynamicComment, int64, error)
	FindByID(ctx context.Context, id uint64) (*database.DynamicComment, error)
	Delete(ctx context.Context, id uint64) error
	IncrementLikeCount(ctx context.Context, id uint64, delta int) error
	IncrementDynamicCommentCount(ctx context.Context, dynamicID uint64, delta int) error
	CountReplies(ctx context.Context, parentID uint64) (int64, error)
	// FindDynamicTargetInfo 查询动态的评论相关元信息
	FindDynamicTargetInfo(ctx context.Context, dynamicID uint64) (*CommentTargetInfo, error)
}

// CommentLikeRepository 评论点赞接口（统一三套表的点赞操作）
// CreateLike 返回 created=true 表示真正新增了点赞记录（用于幂等判断）
type CommentLikeRepository interface {
	CreateLike(ctx context.Context, userID string, commentID uint64) (created bool, err error)
	DeleteLike(ctx context.Context, userID string, commentID uint64) error
	ExistsLike(ctx context.Context, userID string, commentID uint64) (bool, error)
}

// NotificationRepository 通知数据接口
//
// 用于评论回复通知的写入。复用现有 Notification 表，类型 reply_received。
type NotificationRepository interface {
	Create(ctx context.Context, n *database.Notification) error

	// ListByRecipient 分页查询收件人的通知列表
	// filterType 为空时查所有类型；非空时只查指定类型
	// 仅UnreadOnly=true 时只返回未读
	ListByRecipient(ctx context.Context, recipientID string, filterType string, onlyUnread bool, page, pageSize int) ([]database.Notification, int64, error)

	// CountUnread 统计未读通知数
	CountUnread(ctx context.Context, recipientID string) (int64, error)

	// MarkRead 标记单条通知为已读（仅当属于 recipientID）
	// 返回 updated=true 表示真正更新
	MarkRead(ctx context.Context, recipientID string, notifID uint64) (bool, error)

	// MarkAllRead 标记收件人所有未读通知为已读
	MarkAllRead(ctx context.Context, recipientID string) error

	// MuteLike 静默某条评论的点赞通知（recipient + comment_id）
	MuteLike(ctx context.Context, recipientID string, commentID uint64) error
	// IsMutedLike 查询某条评论的点赞通知是否被静默
	IsMutedLike(ctx context.Context, recipientID string, commentID uint64) (bool, error)

	// Delete 删除单条通知（仅当属于 recipientID）
	// 返回 deleted=true 表示真正删除；false 表示该通知不存在/不属于该用户（幂等）
	Delete(ctx context.Context, recipientID string, notifID uint64) (deleted bool, err error)
}
