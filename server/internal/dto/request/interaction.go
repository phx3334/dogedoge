package request

type InteractionVideo struct {
	UserID  string `json:"user_id"`           // 从 JWT 提取，不由客户端传入
	VideoID uint   `json:"video_id" binding:"required"`
}

// FavoriteVideoReq 收藏视频请求
type FavoriteVideoReq struct {
	VideoID  uint   `json:"video_id" binding:"required"`
	FolderID uint64 `json:"folder_id"` // 0 表示默认收藏夹
}

// UnfavoriteVideoReq 取消收藏请求
type UnfavoriteVideoReq struct {
	VideoID  uint   `json:"video_id" binding:"required"`
	FolderID uint64 `json:"folder_id"` // 0 表示从所有收藏夹移除
}

// FollowUserReq 关注/取关请求
type FollowUserReq struct {
	TargetUserID string `json:"target_user_id" binding:"required"`
}

// ListFollowReq 粉丝/关注列表请求
type ListFollowReq struct {
	UserID   string `form:"user_id" binding:"required"`
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
}
