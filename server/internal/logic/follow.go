package logic

import (
	"context"
	"fmt"

	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/dto/response"
)

// FollowLogic 关注/粉丝列表业务逻辑
type FollowLogic struct {
	deps *LogicDeps
}

func NewFollowLogic(deps *LogicDeps) *FollowLogic {
	return &FollowLogic{deps: deps}
}

// ListFollowers 分页查询某用户的粉丝列表
func (l *FollowLogic) ListFollowers(ctx context.Context, req request.ListFollowersReq) (*response.PaginatedResp[response.FollowUserItem], error) {
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

	var followerIDs []string
	var total int64
	if err := l.deps.Breakers.MySQL.Execute(func() error {
		var err error
		followerIDs, total, err = l.deps.InteractionRepo.ListFollowers(ctx, req.UserID, req.Page, req.PageSize)
		return err
	}); err != nil {
		return nil, fmt.Errorf("查询粉丝列表失败")
	}

	return l.buildFollowListResp(ctx, followerIDs, total, req.Page, req.PageSize)
}

// ListFollowing 分页查询某用户的关注列表
func (l *FollowLogic) ListFollowing(ctx context.Context, req request.ListFollowingReq) (*response.PaginatedResp[response.FollowUserItem], error) {
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

	var followeeIDs []string
	var total int64
	if err := l.deps.Breakers.MySQL.Execute(func() error {
		var err error
		followeeIDs, total, err = l.deps.InteractionRepo.ListFollowing(ctx, req.UserID, req.Page, req.PageSize)
		return err
	}); err != nil {
		return nil, fmt.Errorf("查询关注列表失败")
	}

	return l.buildFollowListResp(ctx, followeeIDs, total, req.Page, req.PageSize)
}

// buildFollowListResp 批量查询用户信息构造响应
func (l *FollowLogic) buildFollowListResp(ctx context.Context, userIDs []string, total int64, page, pageSize int) (*response.PaginatedResp[response.FollowUserItem], error) {
	if len(userIDs) == 0 {
		return &response.PaginatedResp[response.FollowUserItem]{
			List:     []response.FollowUserItem{},
			Total:    total,
			Page:     page,
			PageSize: pageSize,
		}, nil
	}

	users, err := l.deps.AccountRepo.FindByIDs(ctx, userIDs)
	if err != nil {
		// 降级：仅返回 ID 列表
		items := make([]response.FollowUserItem, 0, len(userIDs))
		for _, id := range userIDs {
			items = append(items, response.FollowUserItem{ID: id})
		}
		return &response.PaginatedResp[response.FollowUserItem]{
			List:     items,
			Total:    total,
			Page:     page,
			PageSize: pageSize,
		}, nil
	}

	// 构建 ID → Account 映射，保证顺序与 userIDs 一致
	userMapIdx := make(map[string]int, len(users))
	for i, u := range users {
		userMapIdx[u.ID] = i
	}

	items := make([]response.FollowUserItem, 0, len(userIDs))
	for _, id := range userIDs {
		if idx, ok := userMapIdx[id]; ok {
			u := users[idx]
			items = append(items, response.FollowUserItem{
				ID:        u.ID,
				Username:  u.Username,
				AvatarURL: u.AvatarURL,
				Signature: u.Signature,
			})
		} else {
			items = append(items, response.FollowUserItem{ID: id})
		}
	}

	return &response.PaginatedResp[response.FollowUserItem]{
		List:     items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}
