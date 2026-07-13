package logic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"fake_tiktok/internal/breaker"
	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/dto/response"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ArticleLogic 专栏文章业务逻辑层
//
// 负责文章草稿保存、发布与详情查询。
// 所有 MySQL 操作通过 Breakers.MySQL 熔断器包装，读路径额外加 MySQLReadSem 信号量。
type ArticleLogic struct {
	deps *LogicDeps
}

// NewArticleLogic 创建 ArticleLogic 实例。
func NewArticleLogic(deps *LogicDeps) *ArticleLogic {
	return &ArticleLogic{deps: deps}
}

// SaveDraft 保存文章草稿。
//
// 流程：
//  1. 将 tags 列表序列化为 JSON 字符串存入 TagsJSON 字段
//  2. 构造 database.Article{Status:"draft", ...}
//  3. 通过 Breakers.MySQL.Execute 包装 ArticleRepo.Create
//
// 返回新创建文章的 ID。
func (l *ArticleLogic) SaveDraft(ctx context.Context, userID string, req request.ArticleDraftReq) (uint64, error) {
	tagsJSON, err := json.Marshal(req.Tags)
	if err != nil {
		return 0, fmt.Errorf("marshal tags failed: %w", err)
	}

	// 序列化图片列表（最多 9 张）
	images := req.Images
	if len(images) > 9 {
		images = images[:9]
	}
	imagesJSON, err := json.Marshal(images)
	if err != nil {
		return 0, fmt.Errorf("marshal images failed: %w", err)
	}

	article := &database.Article{
		UserID:     userID,
		Title:      req.Title,
		BodyMD:     req.BodyMD,
		CoverURL:   req.CoverURL,
		TagsJSON:   string(tagsJSON),
		ImagesJSON: string(imagesJSON),
		Status:     "draft",
	}

	if err := l.deps.Breakers.MySQL.Execute(func() error {
		return l.deps.ArticleRepo.Create(ctx, article)
	}); err != nil {
		if errors.Is(err, breaker.ErrCircuitOpen) {
			l.deps.Logger.Warn("MySQL circuit open during SaveDraft", zap.String("user_id", userID))
		}
		return 0, err
	}

	return article.ID, nil
}

// PublishArticle 发布文章。
//
// 校验：
//   - 文章归属：article.UserID == userID，否则返回 "无权限"
//   - 文章状态：article.Status == "draft"，否则返回 "文章状态不允许发布"
//
// 校验通过后调用 ArticleRepo.UpdateStatus(id, "published")，
// UpdateStatus 内部会同步写入 published_at = NOW()。
func (l *ArticleLogic) PublishArticle(ctx context.Context, userID string, req request.ArticlePublishReq) error {
	// 先查文章做归属与状态校验（读路径加信号量 + 熔断）
	if err := l.deps.Breakers.MySQLReadSem.Acquire(ctx, 1); err != nil {
		l.deps.Logger.Warn("MySQL read semaphore acquire failed during PublishArticle", zap.Error(err))
		return fmt.Errorf("server busy, please try again later")
	}
	defer l.deps.Breakers.MySQLReadSem.Release(1)

	var article *database.Article
	if err := l.deps.Breakers.MySQL.Execute(func() error {
		var err error
		article, err = l.deps.ArticleRepo.FindByID(ctx, req.ArticleID)
		return err
	}); err != nil {
		if errors.Is(err, breaker.ErrCircuitOpen) {
			l.deps.Logger.Warn("MySQL circuit open during PublishArticle FindByID",
				zap.Uint64("article_id", req.ArticleID))
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("文章不存在")
		}
		return err
	}

	if article.UserID != userID {
		return errors.New("无权限")
	}
	if article.Status != "draft" {
		return errors.New("文章状态不允许发布")
	}

	// 通过熔断器包装写操作
	if err := l.deps.Breakers.MySQL.Execute(func() error {
		return l.deps.ArticleRepo.UpdateStatus(ctx, req.ArticleID, "published")
	}); err != nil {
		if errors.Is(err, breaker.ErrCircuitOpen) {
			l.deps.Logger.Warn("MySQL circuit open during PublishArticle UpdateStatus",
				zap.Uint64("article_id", req.ArticleID))
		}
		return err
	}
	return nil
}

// GetArticleDetail 查询已发布文章详情。
//
// 流程：
//  1. 查询文章（Breakers.MySQL + MySQLReadSem）
//  2. 文章不存在或 status != "published" 时返回 "文章不存在"
//  3. 查询作者信息（AccountRepo.FindByID），同样走熔断 + 信号量
//  4. 异步自增 view_count，不阻塞响应
//  5. 解析 TagsJSON，组装 ArticleDetailResp 返回
func (l *ArticleLogic) GetArticleDetail(ctx context.Context, articleID uint64) (*response.ArticleDetailResp, error) {
	// ---- 1. 查询文章 ----
	if err := l.deps.Breakers.MySQLReadSem.Acquire(ctx, 1); err != nil {
		l.deps.Logger.Warn("MySQL read semaphore acquire failed during GetArticleDetail", zap.Error(err))
		return nil, fmt.Errorf("server busy, please try again later")
	}
	var article *database.Article
	if err := l.deps.Breakers.MySQL.Execute(func() error {
		var err error
		article, err = l.deps.ArticleRepo.FindByID(ctx, articleID)
		return err
	}); err != nil {
		l.deps.Breakers.MySQLReadSem.Release(1)
		if errors.Is(err, breaker.ErrCircuitOpen) {
			l.deps.Logger.Warn("MySQL circuit open during GetArticleDetail FindByID",
				zap.Uint64("article_id", articleID))
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("文章不存在")
		}
		return nil, err
	}
	l.deps.Breakers.MySQLReadSem.Release(1)

	// ---- 2. 状态校验：仅 published 文章对外可见 ----
	if article == nil || article.Status != "published" {
		return nil, errors.New("文章不存在")
	}

	// ---- 3. 查询作者信息 ----
	// 单作者查询用 AccountRepo.FindByID 即可，无需走批量回源路径。
	var author *database.Account
	if article.UserID != "" {
		if err := l.deps.Breakers.MySQLReadSem.Acquire(ctx, 1); err != nil {
			l.deps.Logger.Warn("MySQL read semaphore acquire failed during GetArticleDetail author lookup",
				zap.Error(err))
			return nil, fmt.Errorf("server busy, please try again later")
		}
		if err := l.deps.Breakers.MySQL.Execute(func() error {
			var err error
			author, err = l.deps.AccountRepo.FindByID(ctx, article.UserID)
			return err
		}); err != nil {
			l.deps.Breakers.MySQLReadSem.Release(1)
			if errors.Is(err, breaker.ErrCircuitOpen) {
				l.deps.Logger.Warn("MySQL circuit open during GetArticleDetail FindByID author",
					zap.String("user_id", article.UserID))
			}
			// 作者查询失败不阻塞响应：返回不带作者信息的结果
			author = nil
		} else {
			l.deps.Breakers.MySQLReadSem.Release(1)
		}
	}

	// ---- 4. 异步自增 view_count ----
	// 使用独立 context.Background() 派生的子 context，避免请求结束后 ctx 被 cancel 导致更新失败。
	// view_count 是非关键计数，失败仅记录日志，不影响响应。
	go func(articleID uint64) {
		incCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := l.deps.Breakers.MySQL.Execute(func() error {
			return l.deps.ArticleRepo.IncrementViewCount(incCtx, articleID)
		}); err != nil {
			if errors.Is(err, breaker.ErrCircuitOpen) {
				l.deps.Logger.Warn("MySQL circuit open during IncrementViewCount",
					zap.Uint64("article_id", articleID))
			} else {
				l.deps.Logger.Warn("IncrementViewCount failed",
					zap.Uint64("article_id", articleID), zap.Error(err))
			}
		}
	}(articleID)

	// ---- 5. 解析 tags JSON ----
	var tags []string
	if article.TagsJSON != "" {
		if err := json.Unmarshal([]byte(article.TagsJSON), &tags); err != nil {
			l.deps.Logger.Warn("unmarshal article tags failed",
				zap.Uint64("article_id", articleID), zap.Error(err))
			tags = nil
		}
	}

	// ---- 6. 组装响应 ----
	authorCard := response.UserCard{}
	if author != nil {
		authorCard = response.UserCard{
			ID:        author.ID,
			Username:  author.Username,
			AvatarURL: author.AvatarURL,
		}
	}

	return &response.ArticleDetailResp{
		ID:           article.ID,
		Title:        article.Title,
		CoverURL:     article.CoverURL,
		BodyMD:       article.BodyMD,
		Tags:         tags,
		ImagesJSON:   article.ImagesJSON,
		ViewCount:    article.ViewCount,
		CommentCount: article.CommentCount,
		CreatedAt:    article.CreatedAt,
		Author:       authorCard,
	}, nil
}
