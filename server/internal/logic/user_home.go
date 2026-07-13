package logic

import (
	"context"
	"fmt"

	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/dto/response"
	"fake_tiktok/internal/repository/interfaces"
)

// UserHomeLogic 用户主页相关业务逻辑
type UserHomeLogic struct {
	deps *LogicDeps
}

func NewUserHomeLogic(deps *LogicDeps) *UserHomeLogic {
	return &UserHomeLogic{deps: deps}
}

// ListUserVideos 用户主页视频列表（按 CreatedAt 倒序）
func (l *UserHomeLogic) ListUserVideos(ctx context.Context, req request.ListUserVideosReq) (*response.PaginatedResp[response.HomeVideoInfo], error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 || req.PageSize > 100 {
		req.PageSize = 20
	}

	if err := l.deps.Breakers.MySQLReadSem.Acquire(ctx, 1); err != nil {
		return nil, fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLReadSem.Release(1)

	offset := (req.Page - 1) * req.PageSize
	videos, err := l.deps.VideoRepo.FindPublishedVideosByAuthorID(ctx, req.UserID, req.PageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("查询视频列表失败")
	}

	// 获取作者信息（用户名、头像）
	var authorName, authorAvatar string
	if account, err := l.deps.AccountRepo.FindByID(ctx, req.UserID); err == nil {
		authorName = account.Username
		authorAvatar = account.AvatarURL
	}

	items := make([]response.HomeVideoInfo, 0, len(videos))
	for _, v := range videos {
		items = append(items, response.HomeVideoInfo{
			ID:           v.ID,
			UpName:       authorName,
			UpAvatar:     authorAvatar,
			Title:        v.Title,
			CoverURL:     v.CoverURL,
			PlayCount:    v.PlayCount,
			CommentCount: v.CommentsCount,
			Duration:     v.DurationSec,
			CreatedAt:    v.CreatedAt,
			FavCount:     v.FavCount,
		})
	}

	// 总数：查询用户视频数（account.video_count 已缓存）
	var total int64
	if account, err := l.deps.AccountRepo.FindByID(ctx, req.UserID); err == nil {
		total = account.VideoCount
	}

	return &response.PaginatedResp[response.HomeVideoInfo]{
		List:     items,
		Total:    total,
		Page:     req.Page,
		PageSize: req.PageSize,
	}, nil
}

// GetLevel 获取用户等级信息
func (l *UserHomeLogic) GetLevel(ctx context.Context, userID string) (*response.UserLevelResp, error) {
	account, err := l.deps.AccountRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("查询用户信息失败")
	}
	level := interfaces.CalcLevel(account.Experience)
	resp := &response.UserLevelResp{
		Level:             level,
		Experience:        account.Experience,
		CurrentLevelExp:   interfaces.LevelBaseExp(level),
		MaxLevelExp:       interfaces.LevelThresholds[interfaces.LevelMax-1],
		IsMaxLevel:        level >= interfaces.LevelMax,
	}
	if level < interfaces.LevelMax {
		resp.NextLevelExp = interfaces.LevelThresholds[level-1]
	}
	return resp, nil
}
