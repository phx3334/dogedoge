package response

import (
	"time"
)

// =============================================================================
// 通用分页响应
// =============================================================================

// PaginatedResp 分页响应通用结构
type PaginatedResp[T any] struct {
	List     []T   `json:"list"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

// =============================================================================
// 投币
// =============================================================================

// CoinResultResp 投币结果
type CoinResultResp struct {
	Added        int   `json:"added"`          // 本次新增的硬币数
	VideoCoinCnt int64 `json:"video_coin_cnt"` // 视频累计收到硬币数
	UserBalance  int64 `json:"user_balance"`   // 用户当前剩余硬币（0.1 硬币单位）
}

// CoinLedgerItem 流水项
type CoinLedgerItem struct {
	ID          uint64    `json:"id"`
	DeltaTenths int64     `json:"delta_tenths"` // 正=收入，负=支出
	ReasonType  string    `json:"reason_type"`
	VideoID     uint64    `json:"video_id"`
	CreatedAt   time.Time `json:"created_at"`
}

// =============================================================================
// 收藏夹
// =============================================================================

// FavoriteFolderDetailResp 收藏夹详情
type FavoriteFolderDetailResp struct {
	ID         uint64    `json:"id"`
	Title      string    `json:"title"`
	CoverURL   string    `json:"cover_url"`
	IsDefault  bool      `json:"is_default"`
	VideoCount int64     `json:"video_count"`
	CreatedAt  time.Time `json:"created_at"`
}

// =============================================================================
// 关注 / 粉丝
// =============================================================================

// FollowUserItem 关注/粉丝列表项
type FollowUserItem struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
	Signature string `json:"signature"`
}

// =============================================================================
// 通知
// =============================================================================

// NotificationItem 通知项
type NotificationItem struct {
	ID              uint64    `json:"id"`
	Type            string    `json:"type"`
	RelatedID       string    `json:"related_id"`
	SenderNamesJSON string    `json:"sender_names_json"`
	TotalLikes      int       `json:"total_likes"`
	CommentPreview  string    `json:"comment_preview"`
	PayloadJSON     string    `json:"payload_json"`
	IsRead          bool      `json:"is_read"`
	CreatedAt       time.Time `json:"created_at"`
}

// UnreadCountResp 未读数响应
type UnreadCountResp struct {
	Count int64 `json:"count"`
}

// =============================================================================
// 私信
// =============================================================================

// MessageItem 单条私信
type MessageItem struct {
	ID          uint64    `json:"id"`
	SenderID    string    `json:"sender_id"`
	RecipientID string    `json:"recipient_id"`
	Content     string    `json:"content"`
	IsRead      bool      `json:"is_read"`
	CreatedAt   time.Time `json:"created_at"`
}

// ConversationItem 会话预览（每个对端一条）
type ConversationItem struct {
	PeerID      string    `json:"peer_id"`
	PeerName    string    `json:"peer_name"`
	PeerAvatar  string    `json:"peer_avatar"`
	LastContent string    `json:"last_content"`
	LastAt      time.Time `json:"last_at"`
	UnreadCount int64     `json:"unread_count"`
}

// MessageUnreadResp 私信未读数响应
type MessageUnreadResp struct {
	Count int64 `json:"count"`
}

// =============================================================================
// 历史
// =============================================================================

// VideoHistoryItem 视频观看历史项
type VideoHistoryItem struct {
	VideoID     uint64    `json:"video_id"`
	ProgressSec float64   `json:"progress_sec"`
	DurationSec float64   `json:"duration_sec"`
	Device      string    `json:"device"`
	ViewedAt    time.Time `json:"viewed_at"`
	// 关联视频信息（JOIN 查询返回）
	Title    string  `json:"title"`
	CoverURL string  `json:"cover_url"`
	Duration float64 `json:"duration"`
	UpName   string  `json:"up_name"`
}

// ArticleHistoryItem 文章阅读历史项
type ArticleHistoryItem struct {
	ArticleID uint64    `json:"article_id"`
	Device    string    `json:"device"`
	ViewedAt  time.Time `json:"viewed_at"`
	// 关联文章信息（JOIN 查询返回）
	Title    string `json:"title"`
	CoverURL string `json:"cover_url"`
}

// SearchHistoryItem 搜索历史项
type SearchHistoryItem struct {
	Keyword   string    `json:"keyword"`
	UpdatedAt time.Time `json:"updated_at"`
}

// =============================================================================
// 用户主页
// =============================================================================

// UserVideoListItem 用户主页视频列表项
type UserVideoListItem struct {
	ID        uint      `json:"id"`
	Title     string    `json:"title"`
	CoverURL  string    `json:"cover_url"`
	PlayCount int64     `json:"play_count"`
	CreatedAt time.Time `json:"created_at"`
}

// =============================================================================
// 用户动态
// =============================================================================

// DynamicItem 动态项（支持图文动态、视频投稿、文章投稿三种类型）
type DynamicItem struct {
	ID           uint64    `json:"id"`
	UserID       string    `json:"user_id"`
	Username     string    `json:"username"`
	AvatarURL    string    `json:"avatar_url"`
	Type         string    `json:"type"` // dynamic | video | article
	Title        string    `json:"title"`
	Content      string    `json:"content"`
	ImagesJSON   string    `json:"images_json"`
	VideoID      uint      `json:"video_id,omitempty"`
	ArticleID    uint64    `json:"article_id,omitempty"`
	CoverURL     string    `json:"cover_url,omitempty"`
	Duration     float64   `json:"duration,omitempty"`
	PlayCount    int64     `json:"play_count,omitempty"`
	ViewCount    uint64    `json:"view_count,omitempty"`
	LikeCount    uint64    `json:"like_count"`
	CommentCount uint64    `json:"comment_count"`
	IsLiked      bool      `json:"is_liked"` // 当前用户是否已点赞
	CreatedAt    time.Time `json:"created_at"`
}

// =============================================================================
// 用户等级 / 经验
// =============================================================================

// UserLevelResp 用户等级响应
type UserLevelResp struct {
	Level           int    `json:"level"`
	Experience      uint64 `json:"experience"`
	CurrentLevelExp uint64 `json:"current_level_exp"` // 当前等级起始累计经验（Lv1 为 0）
	NextLevelExp    uint64 `json:"next_level_exp"`  // 升到下一级所需经验（满级时为 0）
	MaxLevelExp     uint64 `json:"max_level_exp"`   // 满级所需累计经验
	IsMaxLevel      bool   `json:"is_max_level"`
}
