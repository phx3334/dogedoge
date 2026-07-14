package mysql

import (
	"context"
	"errors"
	"strconv"

	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/repository/interfaces"

	"gorm.io/gorm"
)

// 编译期接口校验
var (
	_ interfaces.VideoCommentRepository   = (*VideoCommentRepo)(nil)
	_ interfaces.ArticleCommentRepository = (*ArticleCommentRepo)(nil)
	_ interfaces.DynamicCommentRepository = (*DynamicCommentRepo)(nil)
	_ interfaces.CommentLikeRepository    = (*VideoCommentLikeRepo)(nil)
	_ interfaces.CommentLikeRepository    = (*ArticleCommentLikeRepo)(nil)
	_ interfaces.CommentLikeRepository    = (*DynamicCommentLikeRepo)(nil)
	_ interfaces.NotificationRepository   = (*NotificationRepo)(nil)
)

// ---------------------------------------------------------------------------
// VideoCommentRepo —— 视频评论
// ---------------------------------------------------------------------------

// VideoCommentRepo 视频评论表的 MySQL 数据存储
type VideoCommentRepo struct {
	db *gorm.DB
}

func NewVideoCommentRepo(db *gorm.DB) *VideoCommentRepo {
	return &VideoCommentRepo{db: db}
}

// Create 插入一条视频评论
func (r *VideoCommentRepo) Create(ctx context.Context, c *database.Comment) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Create(c).Error
}

// FindByVideoID 分页查询指定视频的顶级评论（Level=1, Approved=true）
// 排序：pinned DESC, like_count DESC, created_at DESC；同时返回总数
func (r *VideoCommentRepo) FindByVideoID(ctx context.Context, videoID uint, page, pageSize int) ([]database.Comment, int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var total int64
	if err := r.db.WithContext(ctx).Model(&database.Comment{}).
		Where("video_id = ? AND level = ? AND approved = ?", videoID, 1, true).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var comments []database.Comment
	err := r.db.WithContext(ctx).
		Where("video_id = ? AND level = ? AND approved = ?", videoID, 1, true).
		Order("pinned DESC, like_count DESC, created_at DESC").
		Limit(pageSize).Offset((page - 1) * pageSize).
		Find(&comments).Error
	return comments, total, err
}

// FindReplies 分页查询指定父评论的回复
// 排序：created_at ASC；同时返回总数
func (r *VideoCommentRepo) FindReplies(ctx context.Context, parentID uint64, page, pageSize int) ([]database.Comment, int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var total int64
	if err := r.db.WithContext(ctx).Model(&database.Comment{}).
		Where("parent_id = ? AND approved = ?", parentID, true).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var comments []database.Comment
	err := r.db.WithContext(ctx).
		Where("parent_id = ? AND approved = ?", parentID, true).
		Order("created_at ASC").
		Limit(pageSize).Offset((page - 1) * pageSize).
		Find(&comments).Error
	return comments, total, err
}

// FindRepliesByParentIDs 批量查询多个父评论的子回复（用于递归查楼中楼）
func (r *VideoCommentRepo) FindRepliesByParentIDs(ctx context.Context, parentIDs []uint64, limit int) ([]database.Comment, int64, error) {
	if len(parentIDs) == 0 {
		return nil, 0, nil
	}
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var total int64
	if err := r.db.WithContext(ctx).Model(&database.Comment{}).
		Where("parent_id IN ? AND approved = ?", parentIDs, true).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var comments []database.Comment
	err := r.db.WithContext(ctx).
		Where("parent_id IN ? AND approved = ?", parentIDs, true).
		Order("created_at ASC").
		Limit(limit).
		Find(&comments).Error
	return comments, total, err
}

// FindByID 根据 ID 查询单条评论
func (r *VideoCommentRepo) FindByID(ctx context.Context, id uint64) (*database.Comment, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var c database.Comment
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// Delete 硬删除一条评论
func (r *VideoCommentRepo) Delete(ctx context.Context, id uint64) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&database.Comment{}).Error
}

// IncrementLikeCount 原子更新评论点赞数（delta 可正可负）
func (r *VideoCommentRepo) IncrementLikeCount(ctx context.Context, id uint64, delta int) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Model(&database.Comment{}).
		Where("id = ?", id).
		UpdateColumn("like_count", gorm.Expr("like_count + ?", delta)).Error
}

// IncrementVideoCommentCount 原子更新视频评论计数（delta 可正可负）
func (r *VideoCommentRepo) IncrementVideoCommentCount(ctx context.Context, videoID uint, delta int) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Model(&database.Video{}).
		Where("id = ?", videoID).
		UpdateColumn("comments_count", gorm.Expr("comments_count + ?", delta)).Error
}

// CountReplies 统计指定父评论的回复数
func (r *VideoCommentRepo) CountReplies(ctx context.Context, parentID uint64) (int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var count int64
	err := r.db.WithContext(ctx).Model(&database.Comment{}).
		Where("parent_id = ?", parentID).
		Count(&count).Error
	return count, err
}

// FindVideoTargetInfo 查询视频的评论相关元信息
func (r *VideoCommentRepo) FindVideoTargetInfo(ctx context.Context, videoID uint) (*interfaces.CommentTargetInfo, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var v database.Video
	if err := r.db.WithContext(ctx).
		Select("id, author_id, comments_closed").
		Where("id = ?", videoID).First(&v).Error; err != nil {
		return nil, err
	}
	return &interfaces.CommentTargetInfo{
		CommentsClosed:  v.CommentsClosed,
		CommentsCurated: false, // 视频无精选模式
		AuthorID:        v.AuthorID,
	}, nil
}

// ---------------------------------------------------------------------------
// ArticleCommentRepo —— 文章评论
// ---------------------------------------------------------------------------

// ArticleCommentRepo 文章评论表的 MySQL 数据存储
type ArticleCommentRepo struct {
	db *gorm.DB
}

func NewArticleCommentRepo(db *gorm.DB) *ArticleCommentRepo {
	return &ArticleCommentRepo{db: db}
}

func (r *ArticleCommentRepo) Create(ctx context.Context, c *database.ArticleComment) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Create(c).Error
}

func (r *ArticleCommentRepo) FindByArticleID(ctx context.Context, articleID uint64, page, pageSize int) ([]database.ArticleComment, int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var total int64
	if err := r.db.WithContext(ctx).Model(&database.ArticleComment{}).
		Where("article_id = ? AND level = ? AND approved = ?", articleID, 1, true).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var comments []database.ArticleComment
	err := r.db.WithContext(ctx).
		Where("article_id = ? AND level = ? AND approved = ?", articleID, 1, true).
		Order("pinned DESC, like_count DESC, created_at DESC").
		Limit(pageSize).Offset((page - 1) * pageSize).
		Find(&comments).Error
	return comments, total, err
}

func (r *ArticleCommentRepo) FindReplies(ctx context.Context, parentID uint64, page, pageSize int) ([]database.ArticleComment, int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var total int64
	if err := r.db.WithContext(ctx).Model(&database.ArticleComment{}).
		Where("parent_id = ? AND approved = ?", parentID, true).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var comments []database.ArticleComment
	err := r.db.WithContext(ctx).
		Where("parent_id = ? AND approved = ?", parentID, true).
		Order("created_at ASC").
		Limit(pageSize).Offset((page - 1) * pageSize).
		Find(&comments).Error
	return comments, total, err
}

func (r *ArticleCommentRepo) FindRepliesByParentIDs(ctx context.Context, parentIDs []uint64, limit int) ([]database.ArticleComment, int64, error) {
	if len(parentIDs) == 0 {
		return nil, 0, nil
	}
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var total int64
	if err := r.db.WithContext(ctx).Model(&database.ArticleComment{}).
		Where("parent_id IN ? AND approved = ?", parentIDs, true).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var comments []database.ArticleComment
	err := r.db.WithContext(ctx).
		Where("parent_id IN ? AND approved = ?", parentIDs, true).
		Order("created_at ASC").
		Limit(limit).
		Find(&comments).Error
	return comments, total, err
}

func (r *ArticleCommentRepo) FindByID(ctx context.Context, id uint64) (*database.ArticleComment, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var c database.ArticleComment
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *ArticleCommentRepo) Delete(ctx context.Context, id uint64) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&database.ArticleComment{}).Error
}

func (r *ArticleCommentRepo) IncrementLikeCount(ctx context.Context, id uint64, delta int) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Model(&database.ArticleComment{}).
		Where("id = ?", id).
		UpdateColumn("like_count", gorm.Expr("like_count + ?", delta)).Error
}

func (r *ArticleCommentRepo) IncrementArticleCommentCount(ctx context.Context, articleID uint64, delta int) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Model(&database.Article{}).
		Where("id = ?", articleID).
		UpdateColumn("comment_count", gorm.Expr("comment_count + ?", delta)).Error
}

func (r *ArticleCommentRepo) CountReplies(ctx context.Context, parentID uint64) (int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var count int64
	err := r.db.WithContext(ctx).Model(&database.ArticleComment{}).
		Where("parent_id = ?", parentID).
		Count(&count).Error
	return count, err
}

// FindArticleTargetInfo 查询文章的评论相关元信息
func (r *ArticleCommentRepo) FindArticleTargetInfo(ctx context.Context, articleID uint64) (*interfaces.CommentTargetInfo, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var a database.Article
	if err := r.db.WithContext(ctx).
		Select("id, user_id, comments_closed, comments_curated").
		Where("id = ?", articleID).First(&a).Error; err != nil {
		return nil, err
	}
	return &interfaces.CommentTargetInfo{
		CommentsClosed:  a.CommentsClosed,
		CommentsCurated: a.CommentsCurated,
		AuthorID:        a.UserID,
	}, nil
}

// ---------------------------------------------------------------------------
// DynamicCommentRepo —— 动态评论
// ---------------------------------------------------------------------------

// DynamicCommentRepo 动态评论表的 MySQL 数据存储
//
// 注意：DynamicComment.UserID 和 UserDynamicText.UserID 均为 uint64，
// 与 Video/Article 评论的 string 类型不同。Like 操作的 userID 入参为 string，
// 内部通过 strconv.ParseUint 转换。
type DynamicCommentRepo struct {
	db *gorm.DB
}

func NewDynamicCommentRepo(db *gorm.DB) *DynamicCommentRepo {
	return &DynamicCommentRepo{db: db}
}

func (r *DynamicCommentRepo) Create(ctx context.Context, c *database.DynamicComment) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Create(c).Error
}

func (r *DynamicCommentRepo) FindByDynamicID(ctx context.Context, dynamicID uint64, page, pageSize int) ([]database.DynamicComment, int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var total int64
	if err := r.db.WithContext(ctx).Model(&database.DynamicComment{}).
		Where("dynamic_id = ? AND level = ?", dynamicID, 1).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var comments []database.DynamicComment
	err := r.db.WithContext(ctx).
		Where("dynamic_id = ? AND level = ?", dynamicID, 1).
		Order("pinned DESC, like_count DESC, created_at DESC").
		Limit(pageSize).Offset((page - 1) * pageSize).
		Find(&comments).Error
	return comments, total, err
}

func (r *DynamicCommentRepo) FindReplies(ctx context.Context, parentID uint64, page, pageSize int) ([]database.DynamicComment, int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var total int64
	if err := r.db.WithContext(ctx).Model(&database.DynamicComment{}).
		Where("parent_id = ?", parentID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var comments []database.DynamicComment
	err := r.db.WithContext(ctx).
		Where("parent_id = ?", parentID).
		Order("created_at ASC").
		Limit(pageSize).Offset((page - 1) * pageSize).
		Find(&comments).Error
	return comments, total, err
}

func (r *DynamicCommentRepo) FindRepliesByParentIDs(ctx context.Context, parentIDs []uint64, limit int) ([]database.DynamicComment, int64, error) {
	if len(parentIDs) == 0 {
		return nil, 0, nil
	}
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var total int64
	if err := r.db.WithContext(ctx).Model(&database.DynamicComment{}).
		Where("parent_id IN ?", parentIDs).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var comments []database.DynamicComment
	err := r.db.WithContext(ctx).
		Where("parent_id IN ?", parentIDs).
		Order("created_at ASC").
		Limit(limit).
		Find(&comments).Error
	return comments, total, err
}

func (r *DynamicCommentRepo) FindByID(ctx context.Context, id uint64) (*database.DynamicComment, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var c database.DynamicComment
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *DynamicCommentRepo) Delete(ctx context.Context, id uint64) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&database.DynamicComment{}).Error
}

func (r *DynamicCommentRepo) IncrementLikeCount(ctx context.Context, id uint64, delta int) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Model(&database.DynamicComment{}).
		Where("id = ?", id).
		UpdateColumn("like_count", gorm.Expr("like_count + ?", delta)).Error
}

func (r *DynamicCommentRepo) IncrementDynamicCommentCount(ctx context.Context, dynamicID uint64, delta int) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Model(&database.UserDynamicText{}).
		Where("id = ?", dynamicID).
		UpdateColumn("comment_count", gorm.Expr("comment_count + ?", delta)).Error
}

func (r *DynamicCommentRepo) CountReplies(ctx context.Context, parentID uint64) (int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var count int64
	err := r.db.WithContext(ctx).Model(&database.DynamicComment{}).
		Where("parent_id = ?", parentID).
		Count(&count).Error
	return count, err
}

// FindDynamicTargetInfo 查询动态的评论相关元信息
func (r *DynamicCommentRepo) FindDynamicTargetInfo(ctx context.Context, dynamicID uint64) (*interfaces.CommentTargetInfo, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var d database.UserDynamicText
	if err := r.db.WithContext(ctx).
		Select("id, user_id, comments_closed").
		Where("id = ?", dynamicID).First(&d).Error; err != nil {
		return nil, err
	}
	return &interfaces.CommentTargetInfo{
		CommentsClosed:  d.CommentsClosed,
		CommentsCurated: false, // 动态无精选模式
		AuthorID:        strconv.FormatUint(d.UserID, 10),
	}, nil
}

// ---------------------------------------------------------------------------
// VideoCommentLikeRepo —— 视频评论点赞
// ---------------------------------------------------------------------------

// VideoCommentLikeRepo 视频评论点赞表的 MySQL 数据存储
type VideoCommentLikeRepo struct {
	db *gorm.DB
}

func NewVideoCommentLikeRepo(db *gorm.DB) *VideoCommentLikeRepo {
	return &VideoCommentLikeRepo{db: db}
}

// CreateLike 尝试插入点赞记录。created=true 表示首次新增（用于幂等判断）。
// 通过唯一索引 idx_like_user_comment 保证幂等：重复点赞时 FirstOrCreate 不报错，
// 但 RowsAffected=0 表示记录已存在。
func (r *VideoCommentLikeRepo) CreateLike(ctx context.Context, userID string, commentID uint64) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	like := database.CommentLike{
		UserID:    userID,
		CommentID: commentID,
	}
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND comment_id = ?", userID, commentID).
		FirstOrCreate(&like)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (r *VideoCommentLikeRepo) DeleteLike(ctx context.Context, userID string, commentID uint64) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).
		Where("user_id = ? AND comment_id = ?", userID, commentID).
		Delete(&database.CommentLike{}).Error
}

func (r *VideoCommentLikeRepo) ExistsLike(ctx context.Context, userID string, commentID uint64) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var count int64
	err := r.db.WithContext(ctx).Model(&database.CommentLike{}).
		Where("user_id = ? AND comment_id = ?", userID, commentID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ---------------------------------------------------------------------------
// ArticleCommentLikeRepo —— 文章评论点赞
// ---------------------------------------------------------------------------

type ArticleCommentLikeRepo struct {
	db *gorm.DB
}

func NewArticleCommentLikeRepo(db *gorm.DB) *ArticleCommentLikeRepo {
	return &ArticleCommentLikeRepo{db: db}
}

func (r *ArticleCommentLikeRepo) CreateLike(ctx context.Context, userID string, commentID uint64) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	like := database.ArticleCommentLike{
		UserID:    userID,
		CommentID: commentID,
	}
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND comment_id = ?", userID, commentID).
		FirstOrCreate(&like)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (r *ArticleCommentLikeRepo) DeleteLike(ctx context.Context, userID string, commentID uint64) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).
		Where("user_id = ? AND comment_id = ?", userID, commentID).
		Delete(&database.ArticleCommentLike{}).Error
}

func (r *ArticleCommentLikeRepo) ExistsLike(ctx context.Context, userID string, commentID uint64) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var count int64
	err := r.db.WithContext(ctx).Model(&database.ArticleCommentLike{}).
		Where("user_id = ? AND comment_id = ?", userID, commentID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ---------------------------------------------------------------------------
// DynamicCommentLikeRepo —— 动态评论点赞
// ---------------------------------------------------------------------------

// DynamicCommentLikeRepo 动态评论点赞表的 MySQL 数据存储
//
// 注意：DynamicCommentLike.UserID 为 uint64，接口入参 userID 为 string，
// 内部通过 strconv.ParseUint 转换。
type DynamicCommentLikeRepo struct {
	db *gorm.DB
}

func NewDynamicCommentLikeRepo(db *gorm.DB) *DynamicCommentLikeRepo {
	return &DynamicCommentLikeRepo{db: db}
}

func (r *DynamicCommentLikeRepo) CreateLike(ctx context.Context, userID string, commentID uint64) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	uid, err := strconv.ParseUint(userID, 10, 64)
	if err != nil {
		return false, err
	}

	like := database.DynamicCommentLike{
		UserID:    uid,
		CommentID: commentID,
	}
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND comment_id = ?", uid, commentID).
		FirstOrCreate(&like)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (r *DynamicCommentLikeRepo) DeleteLike(ctx context.Context, userID string, commentID uint64) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	uid, err := strconv.ParseUint(userID, 10, 64)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).
		Where("user_id = ? AND comment_id = ?", uid, commentID).
		Delete(&database.DynamicCommentLike{}).Error
}

func (r *DynamicCommentLikeRepo) ExistsLike(ctx context.Context, userID string, commentID uint64) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	uid, err := strconv.ParseUint(userID, 10, 64)
	if err != nil {
		return false, err
	}

	var count int64
	err = r.db.WithContext(ctx).Model(&database.DynamicCommentLike{}).
		Where("user_id = ? AND comment_id = ?", uid, commentID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ---------------------------------------------------------------------------
// NotificationRepo —— 通知
// ---------------------------------------------------------------------------

// NotificationRepo 通知表的 MySQL 数据存储
type NotificationRepo struct {
	db *gorm.DB
}

func NewNotificationRepo(db *gorm.DB) *NotificationRepo {
	return &NotificationRepo{db: db}
}

// Create 插入一条通知记录
func (r *NotificationRepo) Create(ctx context.Context, n *database.Notification) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Create(n).Error
}

// CreateBatch 批量插入通知（作者发布后通知其所有粉丝）。
// 单条插入在粉丝量大时过慢，故批量写入。
func (r *NotificationRepo) CreateBatch(ctx context.Context, ns []database.Notification) error {
	if len(ns) == 0 {
		return nil
	}
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Create(&ns).Error
}

// ListByRecipient 分页查询收件人的通知列表（按 CreatedAt 倒序）
func (r *NotificationRepo) ListByRecipient(ctx context.Context, recipientID string, filterType string, onlyUnread bool, page, pageSize int) ([]database.Notification, int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	query := r.db.WithContext(ctx).Model(&database.Notification{}).Where("recipient_id = ?", recipientID)
	if filterType != "" {
		query = query.Where("type = ?", filterType)
	}
	if onlyUnread {
		query = query.Where("is_read = ?", false)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 || page < 1 || pageSize < 1 {
		return nil, total, nil
	}
	offset := (page - 1) * pageSize
	var items []database.Notification
	if err := query.Order("created_at DESC").
		Offset(offset).Limit(pageSize).
		Find(&items).Error; err != nil {
		return nil, total, err
	}
	return items, total, nil
}

// CountUnread 统计未读通知数
func (r *NotificationRepo) CountUnread(ctx context.Context, recipientID string) (int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var count int64
	err := r.db.WithContext(ctx).Model(&database.Notification{}).
		Where("recipient_id = ? AND is_read = ?", recipientID, false).
		Count(&count).Error
	return count, err
}

// MarkRead 标记单条通知为已读（仅当属于 recipientID）
func (r *NotificationRepo) MarkRead(ctx context.Context, recipientID string, notifID uint64) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	result := r.db.WithContext(ctx).Model(&database.Notification{}).
		Where("id = ? AND recipient_id = ? AND is_read = ?", notifID, recipientID, false).
		Update("is_read", true)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// MarkAllRead 标记收件人所有未读通知为已读
func (r *NotificationRepo) MarkAllRead(ctx context.Context, recipientID string) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	return r.db.WithContext(ctx).Model(&database.Notification{}).
		Where("recipient_id = ? AND is_read = ?", recipientID, false).
		Update("is_read", true).Error
}

// MuteLike 静默某条评论的点赞通知
func (r *NotificationRepo) MuteLike(ctx context.Context, recipientID string, commentID uint64) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	mute := database.LikeNotifMute{
		RecipientID: recipientID,
		CommentID:   commentID,
	}
	// FirstOrCreate 保证幂等：已存在不报错
	return r.db.WithContext(ctx).
		Where("recipient_id = ? AND comment_id = ?", recipientID, commentID).
		FirstOrCreate(&mute).Error
}

// IsMutedLike 查询某条评论的点赞通知是否被静默
func (r *NotificationRepo) IsMutedLike(ctx context.Context, recipientID string, commentID uint64) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var mute database.LikeNotifMute
	err := r.db.WithContext(ctx).
		Where("recipient_id = ? AND comment_id = ?", recipientID, commentID).
		First(&mute).Error
	if err == nil {
		return true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return false, err
}

// Delete 删除单条通知（仅当属于 recipientID）
func (r *NotificationRepo) Delete(ctx context.Context, recipientID string, notifID uint64) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	result := r.db.WithContext(ctx).
		Where("id = ? AND recipient_id = ?", notifID, recipientID).
		Delete(&database.Notification{})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}
