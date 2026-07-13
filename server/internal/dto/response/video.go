package response

import (
	"time"
)

type HomeVideoInfo struct {
	ID           uint      `json:"id"`
	UpName       string    `json:"up_name"`
	UpAvatar     string    `json:"up_avatar"`
	Title        string    `json:"title"`
	CoverURL     string    `json:"cover_url"`
	PlayCount    int64     `json:"play_count"`
	CommentCount int64     `json:"comment_count"`
	Duration     float64   `json:"duration"`
	CreatedAt    time.Time `json:"created_at"`
	FavCount     uint64    `json:"fav_count"`
}

type AuthorInfo struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
	Signature string `json:"signature"`
	FansCount int64  `json:"fans_count"`
}

type VideoDetailResp struct {
	ID             uint                  `json:"id"`
	Title          string                `json:"title"`
	Description    string                `json:"description"`
	PlayURL        string                `json:"play_url"`
	CoverURL       string                `json:"cover_url"`
	Duration       float64               `json:"duration"`
	Zone           string                `json:"zone"`
	PlayCount      int64                 `json:"play_count"`
	LikesCnt       int64                 `json:"likes_count"`
	CommentCnt     int64                 `json:"comment_count"`
	FavCnt         uint64                `json:"fav_count"`
	CoinCnt        uint64                `json:"coin_count"`
	DanmakuCnt     uint64                `json:"danmaku_count"`
	CommentsClosed bool                  `json:"comments_closed"`
	DanmakuClosed  bool                  `json:"danmaku_closed"`
	CreatedAt      time.Time             `json:"created_at"`
	Author         AuthorInfo            `json:"author"`
	Interaction    InteractionStatusResp `json:"interaction"`
}

type InteractionStatusResp struct {
	IsLiked     bool  `json:"is_liked"`
	IsFavorited bool  `json:"is_favorited"`
	CoinCount   int64 `json:"coin_count"`
	IsFollowed  bool  `json:"is_followed"`
}

type DanmakuItem struct {
	ID        uint64  `json:"id"`
	Content   string  `json:"content"`
	VideoTime float64 `json:"video_time"`
	Color     string  `json:"color"`
	FontSize  string  `json:"font_size"`
	Mode      int     `json:"mode"` // 0=滚动, 1=顶部, 2=底部
	UserID    string  `json:"user_id"`
	CreatedAt int64   `json:"created_at"`
}

// VideoDraftStatusResp 视频草稿/转码状态查询响应
//
// status 取值：
//   - draft：草稿已落库，转码消息已发布但 worker 尚未开始处理
//   - transcoding：worker 正在执行 ffmpeg 转码
//   - pending_review：转码完成但等待人工审核（当前未启用审核流程，暂不出现）
//   - published：转码完成且可对外播放，video_url / cover_url 已就绪
//   - failed：转码失败，fail_reason 携带 stderr 摘要供用户查看
type VideoDraftStatusResp struct {
	Status     string `json:"status"` // draft/transcoding/pending_review/published/failed
	FailReason string `json:"fail_reason"`
	VideoURL   string `json:"video_url"` // 转码完成后的可播放 URL
	CoverURL   string `json:"cover_url"` // 封面 URL
}
