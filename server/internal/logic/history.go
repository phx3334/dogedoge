package logic

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/dto/response"
)

// HistoryLogic 用户历史业务逻辑
type HistoryLogic struct {
	deps *LogicDeps
}

func NewHistoryLogic(deps *LogicDeps) *HistoryLogic {
	return &HistoryLogic{deps: deps}
}

// =============================================================================
// 视频观看历史
// =============================================================================

// RecordVideoView 记录视频观看进度
func (l *HistoryLogic) RecordVideoView(ctx context.Context, userID string, req request.RecordVideoViewReq) error {
	userIDUint, _ := strconv.ParseUint(userID, 10, 64)
	if userIDUint == 0 {
		return fmt.Errorf("invalid user id")
	}

	// 检查用户是否暂停了观看历史
	account, err := l.deps.AccountRepo.FindByID(ctx, userID)
	if err == nil && account.ViewHistoryPaused {
		return nil // 暂停记录，幂等返回
	}

	history := &database.VideoViewHistory{
		UserID:      userIDUint,
		VideoID:     uint64(req.VideoID),
		ProgressSec: req.ProgressSec,
		DurationSec: req.DurationSec,
		Device:      req.Device,
	}
	if history.Device == "" {
		history.Device = "web"
	}

	if err := l.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); err != nil {
		return fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLWriteSem.Release(1)

	return l.deps.VideoViewHistoryRepo.Upsert(ctx, history)
}

// ListVideoHistory 分页查询视频观看历史（JOIN 视频表返回标题/封面/UP主名）
func (l *HistoryLogic) ListVideoHistory(ctx context.Context, userID string, req request.ListVideoHistoryReq) (*response.PaginatedResp[response.VideoHistoryItem], error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 || req.PageSize > 100 {
		req.PageSize = 20
	}

	userIDUint, _ := strconv.ParseUint(userID, 10, 64)

	if err := l.deps.Breakers.MySQLReadSem.Acquire(ctx, 1); err != nil {
		return nil, fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLReadSem.Release(1)

	items, total, err := l.deps.VideoViewHistoryRepo.ListByUserWithVideo(ctx, userIDUint, req.Page, req.PageSize)
	if err != nil {
		return nil, fmt.Errorf("查询观看历史失败")
	}

	return &response.PaginatedResp[response.VideoHistoryItem]{
		List:     items,
		Total:    total,
		Page:     req.Page,
		PageSize: req.PageSize,
	}, nil
}

// DeleteVideoHistory 删除单条视频观看历史
func (l *HistoryLogic) DeleteVideoHistory(ctx context.Context, userID string, req request.DeleteVideoHistoryReq) error {
	userIDUint, _ := strconv.ParseUint(userID, 10, 64)

	if err := l.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); err != nil {
		return fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLWriteSem.Release(1)

	_, err := l.deps.VideoViewHistoryRepo.Delete(ctx, userIDUint, req.VideoID)
	if err != nil {
		return fmt.Errorf("删除失败")
	}
	return nil
}

// ClearVideoHistory 清空视频观看历史
func (l *HistoryLogic) ClearVideoHistory(ctx context.Context, userID string) error {
	userIDUint, _ := strconv.ParseUint(userID, 10, 64)

	if err := l.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); err != nil {
		return fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLWriteSem.Release(1)

	return l.deps.VideoViewHistoryRepo.ClearAll(ctx, userIDUint)
}

// =============================================================================
// 文章阅读历史
// =============================================================================

// ListArticleHistory 分页查询文章阅读历史（JOIN 文章表返回标题/封面）
func (l *HistoryLogic) ListArticleHistory(ctx context.Context, userID string, req request.ListArticleHistoryReq) (*response.PaginatedResp[response.ArticleHistoryItem], error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 || req.PageSize > 100 {
		req.PageSize = 20
	}

	userIDUint, _ := strconv.ParseUint(userID, 10, 64)

	if err := l.deps.Breakers.MySQLReadSem.Acquire(ctx, 1); err != nil {
		return nil, fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLReadSem.Release(1)

	items, total, err := l.deps.ArticleViewHistoryRepo.ListByUserWithArticle(ctx, userIDUint, req.Page, req.PageSize)
	if err != nil {
		return nil, fmt.Errorf("查询阅读历史失败")
	}

	return &response.PaginatedResp[response.ArticleHistoryItem]{
		List:     items,
		Total:    total,
		Page:     req.Page,
		PageSize: req.PageSize,
	}, nil
}

// DeleteArticleHistory 删除单条文章阅读历史
func (l *HistoryLogic) DeleteArticleHistory(ctx context.Context, userID string, req request.DeleteArticleHistoryReq) error {
	userIDUint, _ := strconv.ParseUint(userID, 10, 64)

	if err := l.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); err != nil {
		return fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLWriteSem.Release(1)

	_, err := l.deps.ArticleViewHistoryRepo.Delete(ctx, userIDUint, req.ArticleID)
	if err != nil {
		return fmt.Errorf("删除失败")
	}
	return nil
}

// =============================================================================
// 搜索历史
// =============================================================================

// SaveSearchHistory 保存搜索历史
// keyword_norm 用于去重（同关键词只保留最新一条）
func (l *HistoryLogic) SaveSearchHistory(ctx context.Context, userID string, req request.SaveSearchHistoryReq) error {
	userIDUint, _ := strconv.ParseUint(userID, 10, 64)

	history := &database.UserSearchHistory{
		UserID:      userIDUint,
		Keyword:     req.Keyword,
		KeywordNorm: normalizeKeyword(req.Keyword),
	}

	if err := l.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); err != nil {
		return fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLWriteSem.Release(1)

	return l.deps.UserSearchHistoryRepo.Upsert(ctx, history)
}

// ListSearchHistory 查询搜索历史（默认最近 20 条）
func (l *HistoryLogic) ListSearchHistory(ctx context.Context, userID string, limit int) ([]response.SearchHistoryItem, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	userIDUint, _ := strconv.ParseUint(userID, 10, 64)

	items, err := l.deps.UserSearchHistoryRepo.ListByUser(ctx, userIDUint, limit)
	if err != nil {
		return nil, fmt.Errorf("查询搜索历史失败")
	}
	result := make([]response.SearchHistoryItem, 0, len(items))
	for _, h := range items {
		result = append(result, response.SearchHistoryItem{
			Keyword:   h.Keyword,
			UpdatedAt: h.UpdatedAt,
		})
	}
	return result, nil
}

// DeleteSearchHistory 删除单条搜索历史
func (l *HistoryLogic) DeleteSearchHistory(ctx context.Context, userID string, req request.DeleteSearchHistoryReq) error {
	userIDUint, _ := strconv.ParseUint(userID, 10, 64)

	if err := l.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); err != nil {
		return fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLWriteSem.Release(1)

	_, err := l.deps.UserSearchHistoryRepo.Delete(ctx, userIDUint, normalizeKeyword(req.Keyword))
	if err != nil {
		return fmt.Errorf("删除失败")
	}
	return nil
}

// ClearSearchHistory 清空搜索历史
func (l *HistoryLogic) ClearSearchHistory(ctx context.Context, userID string) error {
	userIDUint, _ := strconv.ParseUint(userID, 10, 64)

	if err := l.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); err != nil {
		return fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLWriteSem.Release(1)

	return l.deps.UserSearchHistoryRepo.ClearAll(ctx, userIDUint)
}

// normalizeKeyword 关键词规范化（去除首尾空格、转小写）
// 用于去重：用户搜索"  Vue " 和 "vue" 视为同一条历史
func normalizeKeyword(kw string) string {
	// 简单 trim + lower；中文字符不受 lower 影响
	kw = strings.TrimSpace(kw)
	return strings.ToLower(kw)
}
