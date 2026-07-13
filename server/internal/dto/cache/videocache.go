package cache

import (
	"fake_tiktok/internal/domain/database"
	"time"
)

type VideoCacheData struct {
	VideoID        uint      `json:"video_id"`
	PlayURL        string    `json:"play_url"`
	CoverURL       string    `json:"cover_url"`
	Duration       float64   `json:"duration"`
	AuthorID       string    `json:"author_id"`
	Title          string    `json:"title"`
	Description    string    `json:"description"`
	Zone           string    `json:"zone"`
	CommentsClosed bool      `json:"comments_closed"`
	DanmakuClosed  bool      `json:"danmaku_closed"`
	CreatedAt      time.Time `json:"created_at"`
	PlayCount      int64     `json:"play_count"`
	CommentCnt     int64     `json:"comment_count"`
	LikesCnt       int64     `json:"likes_count"`
	FavCnt         uint64    `json:"fav_count"`
	CoinCnt        uint64    `json:"coin_count"`
	DanmakuCnt     uint64    `json:"danmaku_count"`
	AuthorName     string    `json:"author_name"`
	AuthorAvatar   string    `json:"author_avatar"`
	Popularity     float64   `json:"popularity"`
	IsEmpty        bool      `json:"is_empty"`
}

// 修复：添加 JSON 标签，与其他消息类型（VideoLikeIncrementMsg、UserPlayCountIncrementMsg）保持一致
type PlayCountIncrementMsg struct {
	VideoID   uint  `json:"video_id"`
	Increment int64 `json:"increment"`
}

// VideoLikeIncrementMsg 视频点赞数增量消息，由 RabbitMQ 传递给 worker
type VideoLikeIncrementMsg struct {
	VideoID   uint  `json:"video_id"`
	Increment int64 `json:"increment"`
}

// VideoToCacheData 将 database.Video 和作者名转换为 VideoCacheData，消除重复赋值
func VideoToCacheData(video *database.Video, authorName string, authorAvatar string) *VideoCacheData {
	return &VideoCacheData{
		VideoID:        video.ID,
		PlayURL:        video.PlayURL,
		CoverURL:       video.CoverURL,
		Duration:       video.DurationSec,
		AuthorID:       video.AuthorID,
		Title:          video.Title,
		Description:    video.Description,
		Zone:           video.Zone,
		AuthorName:     authorName,
		AuthorAvatar:   authorAvatar,
		CommentsClosed: video.CommentsClosed,
		DanmakuClosed:  video.DanmakuClosed,
		Popularity:     float64(video.Popularity),
		CreatedAt:      video.CreatedAt,
		PlayCount:      video.PlayCount,
		CommentCnt:     video.CommentsCount,
		LikesCnt:       video.LikesCount,
		FavCnt:         video.FavCount,
		CoinCnt:        video.CoinCount,
		DanmakuCnt:     video.DanmakuCount,
	}
}
