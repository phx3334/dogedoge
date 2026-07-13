package interfaces

import (
	"context"

	"fake_tiktok/internal/domain/database"

	"gorm.io/gorm"
)

// VideoDraftRepository 视频草稿上传与转码结果回写接口
//
// 与 VideoRepository 区别：本接口仅承担草稿上传链路相关的写/读：
//   - CreateDraft：插入 status=draft 的 Video 记录，等待 worker 转码
//   - UpdateTranscodeResult：转码成功后回写 video_url / cover_url / duration / status
//   - UpdateTranscodeFailure：转码失败时标记 status=failed + fail_reason
//   - FindDraftByID：前端轮询状态、worker 转码前查询草稿元数据
//
// 拆分理由：避免 VideoRepository 接口膨胀，且草稿路径与已发布视频查询路径
// 在索引、超时、并发模型上有所不同（草稿路径以 idx_video_status 为主）。
type VideoDraftRepository interface {
	// CreateDraft 插入一条 status=draft 的 Video 记录，v.ID 由 GORM 回填
	CreateDraft(ctx context.Context, v *database.Video) error

	// UpdateTranscodeResult 转码成功后回写可播放 URL、时长与状态
	UpdateTranscodeResult(ctx context.Context, videoID uint, videoURL, coverURL string, duration float64, status string) error

	// UpdateTranscodeFailure 转码失败时将 status 置为 failed 并记录原因（截断 2000 字符）
	UpdateTranscodeFailure(ctx context.Context, videoID uint, failReason string) error

	// FindDraftByID 按 ID 查询 Video 草稿；不存在时返回 gorm.ErrRecordNotFound
	FindDraftByID(ctx context.Context, id uint) (*database.Video, error)
}

// VideoRepository 定义视频数据的 MySQL 查询接口。
type VideoRepository interface {
	// FindPublishedVideos 返回已发布视频的基础查询（未执行，供 PaginateRepo 进一步处理）
	FindPublishedVideos() *gorm.DB

	// FindPublishedVideosWithZone 返回指定分区已发布视频的基础查询
	FindPublishedVideosWithZone(zone string) *gorm.DB

	// FindPublishedVideosByIDs 根据视频 ID 列表批量查询已发布视频
	// 用于 Redis 缓存未命中时的批量 MySQL 回源
	FindPublishedVideosByIDs(ctx context.Context, ids []uint) ([]database.Video, error)

	// FindAllPublishedVideoIDs 查询所有已发布视频的关键字段
	// 只 Select 热度计算所需字段，减少数据传输量，用于定时任务全量重建 ZSet
	FindAllPublishedVideoIDs(ctx context.Context) ([]database.Video, error)

	// UpdatePopularity 更新视频的 popularity 热度字段
	// 定时任务将计算出的热度分同步回 MySQL，保证降级路径使用一致的热度排序
	UpdatePopularity(ctx context.Context, id uint, popularity int64) error

	// FindPublishedVideosByAuthorID 查询指定作者的已发布视频，按创建时间倒序
	// 使用 idx_author_time 复合索引，用于用户主页视频列表
	FindPublishedVideosByAuthorID(ctx context.Context, authorID string, limit, offset int) ([]database.Video, error)

	// FindPublishedVideosByAuthorIDs 批量查询指定作者列表的已发布视频，按创建时间倒序
	// 用于动态 Feed 流
	FindPublishedVideosByAuthorIDs(ctx context.Context, authorIDs []string, limit, offset int) ([]database.Video, error)

	FindVideoByID(ctx context.Context, id uint) (*database.Video, error)

	// IncrementFavCount 视频 fav_count +delta（收藏时调用）
	IncrementFavCount(ctx context.Context, videoID uint, delta int) error
	// DecrementFavCount 视频 fav_count -delta（取消收藏时调用）
	// 不会减到负数（SQL 用 GREATEST(fav_count - delta, 0) 兜底）
	DecrementFavCount(ctx context.Context, videoID uint, delta int) error
	// IncrementCoinCount 视频 coin_count +delta（投币时调用）
	IncrementCoinCount(ctx context.Context, videoID uint, delta int) error
	// IncrementDanmakuCount 视频 danmaku_count +delta（发送弹幕时调用）
	IncrementDanmakuCount(ctx context.Context, videoID uint, delta int) error

	// DeleteVideo 软删除指定视频（设置 deleted_at）。
	// 仅当调用方（VideoLogic.DeleteVideo）已校验"操作者即作者"后调用。
	DeleteVideo(ctx context.Context, id uint) error
}
