package rabbitmq

import (
	"context"
	"encoding/json"

	"fake_tiktok/internal/dto/cache"
	"fake_tiktok/internal/repository/interfaces"
	"fake_tiktok/internal/repository/redis"

	amqp091 "github.com/rabbitmq/amqp091-go"
)

// 编译期断言：VideoLikeCountPublisherRepo 必须实现 interfaces.VideoLikeCountPublisher。
var _ interfaces.VideoLikeCountPublisher = (*VideoLikeCountPublisherRepo)(nil)

// VideoLikeCountPublisherRepo 把"视频点赞数 +1"事件发布到 RabbitMQ。
//
// 与 PlayCountPublisherRepo 设计一致：
//   - 持有 *PublishBuffer，连接断开时自动缓冲重放
//   - 调用方完全感知不到"连接断开 / 重连中"等状态
//   - 点赞数是弱一致性数据，极端情况丢最旧可接受
type VideoLikeCountPublisherRepo struct {
	buf *PublishBuffer
}

// NewVideoLikeCountPublisherRepo 构造一个视频点赞数增量发布器。
func NewVideoLikeCountPublisherRepo(buf *PublishBuffer) *VideoLikeCountPublisherRepo {
	return &VideoLikeCountPublisherRepo{buf: buf}
}

// PublishLikeIncrement 把"videoID 点赞数增加 increment"事件发到点赞增量队列。
//
// 流程与 PlayCountPublisherRepo.PublishIncrement 一致：
//  1. 把 (videoID, increment) 序列化为 JSON
//  2. 走 buf.Publish() 自动选择"直发 / 入缓冲"
//  3. 返回值：直发成功 → nil；入缓冲 → 原始错误
func (r *VideoLikeCountPublisherRepo) PublishLikeIncrement(ctx context.Context, videoID uint, increment int64) error {
	body, err := json.Marshal(cache.VideoLikeIncrementMsg{VideoID: videoID, Increment: increment})
	if err != nil {
		return err
	}
	return r.buf.Publish(ctx, "", redis.VideoLikeCountQueueName, amqp091.Publishing{
		ContentType: "application/json",
		Body:        body,
	})
}
