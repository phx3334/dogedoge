package interfaces

import (
	"context"

	"fake_tiktok/internal/dto/cache"
)

type VideoCacheRepository interface {
	// GetVideoCache 批量查询视频缓存，返回命中数据和未命中 ID 列表
	//
	// 返回值说明：
	//   - map[uint]*cache.VideoCacheData: 命中的视频缓存数据（包括空对象标记）
	//   - []uint: 未命中的视频 ID 列表（需要从 MySQL 回源）
	//   - error: Pipeline 整体执行错误（如 Redis 连接断开、超时等）
	//     调用方应将此 error 返回给熔断器闭包，使熔断器能感知下游失败
	GetVideoCache(ctx context.Context, videoIDs []uint) (map[uint]*cache.VideoCacheData, []uint, error)
	WriteVideoCache(ctx context.Context, items []*cache.VideoCacheData, emptyIDs []uint)
	IncrementPlayCount(ctx context.Context, videoID uint) error
	// IncrementLikeCount 视频动态缓存中 likes_count +1
	IncrementLikeCount(ctx context.Context, videoID uint) error
	// DecrementLikeCount 视频动态缓存中 likes_count -1（取消点赞）
	// 计数不会减到负数（INCRBY 后用 MAX 0 兜底）
	DecrementLikeCount(ctx context.Context, videoID uint) error
	// IncrementDanmakuCount 视频动态缓存中 danmaku_count +1（发送弹幕时调用）
	IncrementDanmakuCount(ctx context.Context, videoID uint) error
	// SetDanmakuCount 直接设置 danmaku_count（自愈校正用）
	SetDanmakuCount(ctx context.Context, videoID uint, count uint64) error
	// IncrementFavCount 视频动态缓存中 fav_count +1（收藏时调用）
	IncrementFavCount(ctx context.Context, videoID uint) error
	// DecrementFavCount 视频动态缓存中 fav_count -1（取消收藏时调用）
	DecrementFavCount(ctx context.Context, videoID uint) error
	// IncrementCoinCount 视频动态缓存中 coin_count +delta（投币时调用）
	IncrementCoinCount(ctx context.Context, videoID uint, delta int) error
	// IncrementCommentCount 视频动态缓存中 comment_count +delta（评论时调用）
	IncrementCommentCount(ctx context.Context, videoID uint, delta int) error

	// DeleteVideoCache 删除指定视频的静态/动态 Hash 缓存与空对象标记，
	// 使其从缓存失效；后续请求回源 MySQL 时若已软删除则返回 404，
	// 避免已删除视频仍通过陈旧缓存被展示。
	DeleteVideoCache(ctx context.Context, videoID uint) error
}
