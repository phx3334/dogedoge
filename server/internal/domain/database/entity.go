package database

import (
	"time"

	"gorm.io/gorm"
)

type Role int

const (
	Guest Role = iota
	User
	Admin
)

type Account struct {
	Role                   Role      `gorm:"default:0"`
	ID                     string    `gorm:"type:char(36);uniqueIndex"`
	Username               string    `gorm:"type:varchar(255);uniqueIndex"`
	Password               string    `gorm:"type:varchar(255);not null"`
	Email                  string    `gorm:"type:varchar(255);uniqueIndex"`
	AvatarURL              string    `gorm:"type:varchar(512)"`
	Signature              string    `gorm:"type:varchar(512);default:'签名是空白的，这位用户似乎比较高冷。'"`
	Address                string    `gorm:"type:varchar(255)"`
	Freeze                 bool      `gorm:"default:false"`
	VideoCount             int64     `gorm:"default:0"`
	TotalLikesReceived     int64     `gorm:"not null;default:0"`
	TotalPlayCount         int64     `gorm:"not null;default:0"`
	CreatedAt              time.Time `gorm:"autoCreateTime"`
	UpdatedAt              time.Time
	Gender                 string `gorm:"size:16"` // male | female | secret
	Birthday               string `gorm:"size:10"` // YYYY-MM-DD, may be empty
	PrivacyPublicFavorites bool   `gorm:"not null;default:0"`
	PrivacyPublicFollowing bool   `gorm:"not null;default:0"`
	PrivacyPublicFans      bool   `gorm:"not null;default:0"`
	// Experience is total user EXP for account level (Lv1–Lv6 thresholds in userlevel package).
	Experience uint64 `gorm:"not null;default:0"`
	// CoinBalanceTenths is the user's 硬币 balance in 0.1-coin units (230 = 23.0 coins).
	CoinBalanceTenths int64 `gorm:"not null;default:210"`
	// ViewHistoryPaused stops recording new watch-history entries when true.
	ViewHistoryPaused bool `gorm:"not null;default:0"`
}

type Video struct {
	ID            uint      `gorm:"primaryKey;autoIncrement"`
	AuthorID      string    `gorm:"type:char(36);not null;index:idx_author_time"`
	DurationSec   float64   `gorm:"column:duration_sec"`
	Status        string    `gorm:"size:32;index:idx_video_status"`
	FailReason    string    `gorm:"size:2000"`
	Title         string    `gorm:"type:varchar(255);not null"`
	Description   string    `gorm:"type:varchar(512)"`
	PlayURL       string    `gorm:"type:varchar(512);not null"`
	CoverURL      string    `gorm:"type:varchar(512)"`
	PlayCount     int64     `gorm:"default:0"`
	DanmakuCount  uint64    `gorm:"default:0"`
	FavCount      uint64    `gorm:"default:0"`
	CoinCount     uint64    `gorm:"default:0"`
	LikesCount    int64     `gorm:"default:0"`
	CommentsCount int64     `gorm:"default:0"`
	Popularity    int64     `gorm:"column:popularity;not null;default:0;index:idx_videos_popularity_time_id,priority:1,sort:desc"`
	CreatedAt     time.Time `gorm:"autoCreateTime;index:idx_author_time,sort:desc"`
	UpdatedAt     time.Time
	// CommentsClosed：UP 关闭评论区后禁止新发评论；列表对访客返回空。
	CommentsClosed bool `gorm:"not null;default:0"`
	// DanmakuClosed：UP 关闭弹幕后禁止新发弹幕。
	DanmakuClosed bool `gorm:"not null;default:0"`
	// TagsJSON is a JSON array of strings, e.g. ["录屏","教程"]；空串表示无标签。
	TagsJSON string `gorm:"type:text"`
	// Zone is the publish partition, e.g. "动画" or "生活-日常".
	Zone string `gorm:"size:64"`
	// DraftRawPath / DraftCoverPath：status=draft 时本地暂存路径，投稿转码前使用。
	DraftRawPath      string `gorm:"size:1024"`
	DraftCoverPath    string `gorm:"size:1024"`
	ReviewedAt        *time.Time
	ReviewedByAdminID *uint64 `gorm:"index"`

	// DeletedAt 软删除时间戳；非 nil 表示作者已删除该投稿。
	// GORM 自动在 Where/Find/First 查询中追加 "deleted_at IS NULL" 过滤，
	// 使已删除视频从首页列表、分区榜、作者主页、搜索等所有查询中隐藏。
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

type Danmaku struct {
	ID      uint64 `gorm:"primaryKey"`
	VideoID uint64 `gorm:"index:idx_danmaku_video;not null"`
	UserID  string `gorm:"type:varchar(64);index;not null"`
	Content string `gorm:"size:400;not null"`
	Color   string `gorm:"size:16;not null"`
	Type    string `gorm:"size:16;not null"`
	// FontSize: sm | md | lg（弹幕字号，默认 md）
	FontSize  string  `gorm:"size:8;not null;default:md"`
	VideoTime float64 `gorm:"column:video_time;not null"`
	LikeCount uint64  `gorm:"default:0"`
	CreatedAt time.Time
}

type Comment struct {
	ID        uint64 `gorm:"primaryKey"`
	VideoID   uint64 `gorm:"index:idx_comment_video;not null"`
	UserID    string `gorm:"type:varchar(64);index;not null"`
	ParentID  uint64 `gorm:"index;default:0"`
	Level     int    `gorm:"not null"`
	Content   string `gorm:"size:2000;not null"`
	LikeCount uint64 `gorm:"default:0"`
	Pinned    bool   `gorm:"index;default:0"`
	// Approved：评论精选模式下，false 表示待 UP 精选；非精选模式创建时设为 true。
	Approved bool `gorm:"not null;default:0;index"`
	// CuratedIgnored：精选模式下 UP 忽略（不公开），仍保持 approved=false。
	CuratedIgnored bool   `gorm:"not null;default:0;index"`
	IpLocation     string `gorm:"size:32;not null;default:''"`
	CreatedAt      time.Time
}

// CommentLike records a user's like on a comment.
type CommentLike struct {
	ID        uint64 `gorm:"primaryKey"`
	UserID    string `gorm:"type:varchar(64);uniqueIndex:idx_like_user_comment;not null"`
	CommentID uint64 `gorm:"uniqueIndex:idx_like_user_comment;not null"`
	CreatedAt time.Time
}

// VideoLike records a user's like on a published video (e.g. 动态点赞).
type VideoLike struct {
	ID        uint64 `gorm:"primaryKey"`
	UserID    string `gorm:"type:varchar(64);uniqueIndex:idx_video_like_user_video;not null"`
	VideoID   uint64 `gorm:"uniqueIndex:idx_video_like_user_video;not null"`
	CreatedAt time.Time
}

// FavoriteFolder groups a user's favorited videos (收藏夹).
type FavoriteFolder struct {
	ID        uint64 `gorm:"primaryKey"`
	UserID    string `gorm:"type:varchar(64);index:idx_fav_folder_user;not null"`
	Title     string `gorm:"size:20;not null"`
	CoverURL  string `gorm:"size:1024;not null;default:''"`
	IsDefault bool   `gorm:"not null;default:0"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// VideoFavorite records a user's favorite (收藏) in one folder (same video may appear in multiple folders).
type VideoFavorite struct {
	ID        uint64 `gorm:"primaryKey"`
	UserID    string `gorm:"type:varchar(64);uniqueIndex:idx_video_fav_user_video_folder,priority:1;not null"`
	VideoID   uint64 `gorm:"uniqueIndex:idx_video_fav_user_video_folder,priority:2;not null"`
	FolderID  uint64 `gorm:"uniqueIndex:idx_video_fav_user_video_folder,priority:3;index:idx_video_fav_folder;not null;default:0"`
	CreatedAt time.Time
}

// VideoCoin records a user's coin (投币) on a published video (one per user per video).
type VideoCoin struct {
	ID        uint64 `gorm:"primaryKey"`
	UserID    string `gorm:"type:varchar(64);uniqueIndex:idx_video_coin_user_video;not null"`
	VideoID   uint64 `gorm:"uniqueIndex:idx_video_coin_user_video;not null"`
	Amount    int    `gorm:"not null;default:1"` // 1 or 2
	CreatedAt time.Time
}

// UserFollow records follower -> followee (关注关系).
type UserFollow struct {
	ID         uint64 `gorm:"primaryKey"`
	FollowerID string `gorm:"type:varchar(64);uniqueIndex:idx_user_follow_pair,priority:1;index:idx_user_follow_follower;not null"`
	FolloweeID string `gorm:"type:varchar(64);uniqueIndex:idx_user_follow_pair,priority:2;index:idx_user_follow_followee;not null"`
	CreatedAt  time.Time
}

// Notification is an inbox item (like aggregation, etc.).
type Notification struct {
	ID              uint64 `gorm:"primaryKey"`
	RecipientID     string `gorm:"type:varchar(64);index:idx_notif_recipient;not null"`
	Type            string `gorm:"size:48;index;not null"`
	RelatedID       string `gorm:"type:varchar(64);index"`
	SenderNamesJSON string `gorm:"type:text"`
	TotalLikes      int    `gorm:"default:0"`
	CommentPreview  string `gorm:"size:32"`
	// PayloadJSON holds type-specific fields (e.g. reply_received: sender, reply body, video_id).
	PayloadJSON string `gorm:"type:text"`
	IsRead      bool   `gorm:"index"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// LikeNotifMute marks「不再通知」for like aggregation on a specific comment (recipient + comment_id).
type LikeNotifMute struct {
	RecipientID string `gorm:"type:varchar(64);uniqueIndex:idx_like_notif_mute_pair;not null"`
	CommentID   uint64 `gorm:"uniqueIndex:idx_like_notif_mute_pair;not null"`
	CreatedAt   time.Time
}

// Message 用户间私信（单表，按 sender/recipient 双向存储，无独立会话表）。
type Message struct {
	ID          uint64     `gorm:"primaryKey"`
	SenderID    string     `gorm:"type:varchar(64);index:idx_msg_sender;not null"`
	RecipientID string     `gorm:"type:varchar(64);index:idx_msg_recipient;not null"`
	Content     string     `gorm:"type:varchar(2000);not null"`
	ReadAt      *time.Time `gorm:"index"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// MessageConversation 会话预览（每个对端一条），由 MessageRepo 聚合查询填充。
type MessageConversation struct {
	PeerID      string
	LastAt      time.Time
	UnreadCount int64
}

func DisplayUsername(u *Account) string {
	if u == nil {
		return ""
	}
	return u.Username
}

// Article is a published column (专栏) with Markdown body.
type Article struct {
	ID           uint64 `gorm:"primaryKey"`
	UserID       string `gorm:"type:varchar(64);index:idx_article_user;not null"`
	Title        string `gorm:"size:80;not null"`
	CoverURL     string `gorm:"size:1024"`
	BodyMD       string `gorm:"type:longtext;not null"`
	Status       string `gorm:"size:32;index:idx_article_status;not null;default:draft"`
	TagsJSON     string `gorm:"type:text"`
	ImagesJSON   string `gorm:"type:text"`
	ViewCount    uint64 `gorm:"default:0"`
	CommentCount uint64 `gorm:"default:0"`
	// CommentsClosed：作者关闭评论区后禁止新发评论；列表对访客返回空。
	CommentsClosed bool `gorm:"not null;default:0"`
	// CommentsCurated：开启评论精选后，新评论需作者确认才对所有人可见。
	CommentsCurated   bool   `gorm:"not null;default:0"`
	CoinCount         uint64 `gorm:"default:0"`
	FavCount          uint64 `gorm:"default:0"`
	ForwardCount      uint64 `gorm:"default:0"`
	FailReason        string `gorm:"size:2000"`
	PublishedAt       *time.Time
	ReviewedAt        *time.Time
	ReviewedByAdminID *uint64   `gorm:"index"`
	CreatedAt         time.Time `gorm:"index:idx_article_created"`
	UpdatedAt         time.Time
}

// ArticleFavorite records a user's favorite on an article (图文收藏夹).
type ArticleFavorite struct {
	ID        uint64 `gorm:"primaryKey"`
	UserID    string `gorm:"type:varchar(64);uniqueIndex:idx_article_fav_user_article;not null"`
	ArticleID uint64 `gorm:"uniqueIndex:idx_article_fav_user_article;not null"`
	CreatedAt time.Time
}

// ArticleCoin records coins tipped on an article (one row per user per article).
type ArticleCoin struct {
	ID        uint64 `gorm:"primaryKey"`
	UserID    string `gorm:"type:varchar(64);uniqueIndex:idx_article_coin_user_article;not null"`
	ArticleID uint64 `gorm:"uniqueIndex:idx_article_coin_user_article;not null"`
	Amount    int    `gorm:"not null;default:1"`
	CreatedAt time.Time
}

// ArticleComment is a threaded comment under an article (max depth 3).
type ArticleComment struct {
	ID        uint64 `gorm:"primaryKey"`
	ArticleID uint64 `gorm:"index:idx_article_comment_article;not null"`
	UserID    string `gorm:"type:varchar(64);index;not null"`
	ParentID  uint64 `gorm:"index;default:0"`
	Level     int    `gorm:"not null"`
	Content   string `gorm:"size:2000;not null"`
	LikeCount uint64 `gorm:"default:0"`
	Pinned    bool   `gorm:"index;default:0"`
	// Approved：评论精选模式下，false 表示待作者精选；非精选模式创建时设为 true。
	Approved       bool   `gorm:"not null;default:0;index"`
	CuratedIgnored bool   `gorm:"not null;default:0;index"`
	IpLocation     string `gorm:"size:32;not null;default:''"`
	CreatedAt      time.Time
}

// ArticleCommentLike records a user's like on an article comment.
type ArticleCommentLike struct {
	ID        uint64 `gorm:"primaryKey"`
	UserID    string `gorm:"type:varchar(64);uniqueIndex:idx_article_cmt_like_user_cmt;not null"`
	CommentID uint64 `gorm:"uniqueIndex:idx_article_cmt_like_user_cmt;not null"`
	CreatedAt time.Time
}

type Login struct {
	UserID      string `json:"user_id"`
	IP          string `json:"ip"`
	Address     string `json:"address"`
	OS          string `json:"os"`
	DeviceInfo  string `json:"device_info"`
	BrowserInfo string `json:"browser_info"`
	Status      int    `json:"status"`
}
