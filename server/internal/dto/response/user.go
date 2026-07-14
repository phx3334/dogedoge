package response

import (
	"fake_tiktok/internal/domain/database"
)

type Login struct {
	Account           database.Account `json:"account"`
	AccessToken       string           `json:"access_token"`
	AccessTokenExpire int              `json:"access_token_expire"`
}

// UserHomeResp 用户主页响应
type UserHomeResp struct {
	ID                 string               `json:"id"`
	AvatarURL          string               `json:"avatar_url"`
	Signature          string               `json:"signature"`
	Username           string               `json:"username"`
	Address            string               `json:"address"`
	VideoCount         int64                `json:"video_count"`
	Birthday           string               `json:"birthday"`
	Gender             string               `json:"gender"`
	TotalLikesReceived int64                `json:"total_likes_received"`
	TotalPlayCount     int64                `json:"total_play_count"`
	Experience         uint64               `json:"experience"`
	FansCount          int64                `json:"fans_count"`
	FollowingCount     int64                `json:"following_count"`
	// IsFollowed 当前登录用户是否已关注该主页用户（用于前端"关注/已关注"按钮正确回显）。
	// 未登录 / 查看自己主页时为 false。
	IsFollowed        bool                 `json:"is_followed"`
	FavoriteFolders    []FavoriteFolderInfo `json:"favorite_folders"`
	Videos             []HomeVideoInfo      `json:"videos"`
}

// FavoriteFolderInfo 收藏夹信息
type FavoriteFolderInfo struct {
	ID        uint64 `json:"id"`
	Title     string `json:"title"`
	CoverURL  string `json:"cover_url"`
	IsDefault bool   `json:"is_default"`
}

// UserCard 用户卡片信息（共享结构）。
//
// 用于评论列表、文章详情等场景的作者展示。
// Phase 3 评论模块与本文件共用此结构，避免在 comment.go / article.go 中重复定义导致编译冲突。
type UserCard struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
	Level     int    `json:"level"`
}

// UserBriefResp 用户简档（供私信入口按 user_id 拉取对端资料，无需手动输入 ID）
type UserBriefResp struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
}
