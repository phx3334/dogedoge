package request

// =============================================================================
// 投币相关
// =============================================================================

// CoinVideoReq 视频投币请求
// Amount 必须 ∈ {1, 2}：单视频每个用户最多投 2 个硬币
type CoinVideoReq struct {
	VideoID uint `json:"video_id" binding:"required"`
	Amount  int  `json:"amount" binding:"required,oneof=1 2"`
}

// ListCoinLedgerReq 硬币流水查询请求
type ListCoinLedgerReq struct {
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
	Reason   string `form:"reason_type"` // 可选：过滤原因类型
}

// =============================================================================
// 收藏夹管理
// =============================================================================

// CreateFavoriteFolderReq 创建收藏夹请求
type CreateFavoriteFolderReq struct {
	Title    string `json:"title" binding:"required,min=1,max=20"`
	CoverURL string `json:"cover_url"`
}

// UpdateFavoriteFolderReq 更新收藏夹请求
type UpdateFavoriteFolderReq struct {
	FolderID uint64 `json:"folder_id" binding:"required"`
	Title    string `json:"title" binding:"required,min=1,max=20"`
	CoverURL string `json:"cover_url"`
}

// DeleteFavoriteFolderReq 删除收藏夹请求
type DeleteFavoriteFolderReq struct {
	FolderID uint64 `json:"folder_id" binding:"required"`
}

// ListFolderVideosReq 收藏夹视频列表请求
type ListFolderVideosReq struct {
	FolderID uint64 `form:"folder_id"` // 0 表示默认收藏夹
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
}

// MoveFavoriteReq 移动收藏到指定收藏夹
type MoveFavoriteReq struct {
	VideoID  uint   `json:"video_id" binding:"required"`
	FolderID uint64 `json:"folder_id" binding:"required"`
}

// =============================================================================
// 关注 / 粉丝列表
// =============================================================================

// ListFollowersReq 粉丝列表请求
type ListFollowersReq struct {
	UserID   string `form:"user_id" binding:"required"`
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
}

// ListFollowingReq 关注列表请求
type ListFollowingReq struct {
	UserID   string `form:"user_id" binding:"required"`
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
}

// =============================================================================
// 通知
// =============================================================================

// ListNotificationsReq 通知列表请求
type ListNotificationsReq struct {
	Type       string `form:"type"`        // 可选：类型过滤
	OnlyUnread bool   `form:"only_unread"` // 仅未读
	Page       int    `form:"page"`
	PageSize   int    `form:"page_size"`
}

// MarkNotificationReadReq 标记通知已读
type MarkNotificationReadReq struct {
	NotificationID uint64 `json:"notification_id" binding:"required"`
}

// MuteLikeNotifReq 静默评论点赞通知
type MuteLikeNotifReq struct {
	CommentID uint64 `json:"comment_id" binding:"required"`
}

// DeleteNotificationReq 删除单条通知
type DeleteNotificationReq struct {
	NotificationID uint64 `json:"notification_id" binding:"required"`
}

// =============================================================================
// 历史
// =============================================================================

// ListVideoHistoryReq 视频观看历史请求
type ListVideoHistoryReq struct {
	Page     int `form:"page"`
	PageSize int `form:"page_size"`
}

// DeleteVideoHistoryReq 删除单条观看历史
type DeleteVideoHistoryReq struct {
	VideoID uint64 `json:"video_id" binding:"required"`
}

// ListArticleHistoryReq 文章阅读历史请求
type ListArticleHistoryReq struct {
	Page     int `form:"page"`
	PageSize int `form:"page_size"`
}

// DeleteArticleHistoryReq 删除单条阅读历史
type DeleteArticleHistoryReq struct {
	ArticleID uint64 `json:"article_id" binding:"required"`
}

// SaveSearchHistoryReq 保存搜索历史
type SaveSearchHistoryReq struct {
	Keyword string `json:"keyword" binding:"required,min=1,max=100"`
}

// DeleteSearchHistoryReq 删除单条搜索历史
type DeleteSearchHistoryReq struct {
	Keyword string `json:"keyword" binding:"required"`
}

// RecordVideoViewReq 记录视频观看进度
type RecordVideoViewReq struct {
	VideoID     uint    `json:"video_id" binding:"required"`
	ProgressSec float64 `json:"progress_sec"`
	DurationSec float64 `json:"duration_sec"`
	Device      string  `json:"device"` // web | mobile
}

// =============================================================================
// 用户主页
// =============================================================================

// ListUserVideosReq 用户主页视频列表请求
type ListUserVideosReq struct {
	UserID   string `form:"user_id" binding:"required"`
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
}

// ArticleFavoriteReq 文章收藏请求
type ArticleFavoriteReq struct {
	ArticleID uint64 `json:"article_id" binding:"required"`
}

// ArticleCoinReq 文章投币请求
type ArticleCoinReq struct {
	ArticleID uint64 `json:"article_id" binding:"required"`
	Amount    int    `json:"amount" binding:"required,oneof=1 2"`
}

// =============================================================================
// 用户动态
// =============================================================================

// CreateDynamicReq 发布动态请求
type CreateDynamicReq struct {
	Title   string   `json:"title" binding:"max=20"`
	Content string   `json:"content" binding:"max=233"`
	Images  []string `json:"images"` // 最多 9 张图片 URL
}

// ListUserDynamicsReq 用户动态列表请求
type ListUserDynamicsReq struct {
	UserID   string `form:"user_id" binding:"required"`
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
}

// ListDynamicFeedReq 关注用户动态流请求
type ListDynamicFeedReq struct {
	Page     int `form:"page"`
	PageSize int `form:"page_size"`
}

// LikeDynamicReq 点赞动态请求
type LikeDynamicReq struct {
	DynamicID uint64 `json:"dynamic_id" binding:"required"`
}
