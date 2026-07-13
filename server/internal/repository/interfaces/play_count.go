package interfaces

import "context"

type PlayCountPublisher interface {
	PublishIncrement(ctx context.Context, videoID uint, increment int64) error
}

// UserPlayCountPublisher 用户总播放量增量发布器
type UserPlayCountPublisher interface {
	PublishUserIncrement(ctx context.Context, userID string, increment int64) error
}

// VideoLikeCountPublisher 视频点赞数增量发布器
// 将点赞数 +1 事件发布到 RabbitMQ，由 worker 消费后延迟 3 秒批量写入 MySQL
// 延迟批量写入的原因：短时间内多个用户可能点赞同一个视频，
// 聚合后一次 UPDATE likes_count + N 比逐条 +1 更高效
type VideoLikeCountPublisher interface {
	PublishLikeIncrement(ctx context.Context, videoID uint, increment int64) error
}
