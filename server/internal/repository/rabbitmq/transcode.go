package rabbitmq

import (
	"context"
	"encoding/json"

	"fake_tiktok/internal/dto/cache"
	"fake_tiktok/internal/repository/interfaces"
	"fake_tiktok/internal/repository/redis"

	amqp091 "github.com/rabbitmq/amqp091-go"
)

// 编译期断言：TranscodePublisherRepo 必须实现 interfaces.TranscodePublisher。
// 任何方法签名变更会在编译期被发现。
var _ interfaces.TranscodePublisher = (*TranscodePublisherRepo)(nil)

// TranscodePublisherRepo 把"视频草稿转码"任务发布到 RabbitMQ 的 mini_bili_transcode 队列。
//
// 与 PlayCountPublisherRepo 完全一致的实现模式：
//   - 不直接持有 *amqp091.Channel，而是持有 *PublishBuffer
//   - PublishBuffer 内部已封装 Connection + 有界缓冲 + 自动重放
//   - 调用方（VideoDraftLogic.UploadDraft）感知不到"连接断开 / 重连中"等状态
//
// 为什么走缓冲而不是直连 publish：
//   - 视频上传链路是用户请求路径，MQ 抖动不应让上传失败
//   - PublishBuffer 在 broker 恢复后自动重放，业务方零感知
//   - 极端情况（broker 长时间不可用）缓冲满会丢最旧，但转码任务可由运维重新触发
type TranscodePublisherRepo struct {
	buf *PublishBuffer
}

// NewTranscodePublisherRepo 构造一个转码任务发布器。
//
// buf 必须已经调用过 Start(ctx)，否则 drainLoop 不会启动。
// 当前约定在 router 初始化时统一 Start，这里只接收已启动的实例。
func NewTranscodePublisherRepo(buf *PublishBuffer) *TranscodePublisherRepo {
	return &TranscodePublisherRepo{buf: buf}
}

// Publish 将一条 TranscodeMsg 发布到 mini_bili_transcode 队列。
//
// 流程：
//  1. 把 msg 序列化为 JSON
//  2. 走 buf.Publish() 自动选择"直发 / 入缓冲"
//  3. 返回值：直发成功 → nil；入缓冲 → 原始错误（供调用方感知）
//
// 关于 QueueDeclare：
//   - 严格来说 worker 端启动时已声明过，这里再声明一次是幂等的
//   - 但调用 buf.Publish() 走的是"Connection 拿 channel"，channel 在重建后
//     是新对象，broker 端若队列被外部删除（运维误操作），本路径暂时不会感知
//   - 当前选择"不在 publish 路径中做 QueueDeclare"以减少与 broker 的额外 RTT
func (r *TranscodePublisherRepo) Publish(ctx context.Context, msg cache.TranscodeMsg) error {
	body, err := json.Marshal(msg)
	if err != nil {
		// 序列化失败：业务方错误，不应入缓冲
		return err
	}

	// 确保目标队列已存在：消除"worker 尚未声明队列时直发消息被 broker 静默丢弃"的竞态，
	// 否则视频草稿会一直停在 draft、前端上传永远转圈
	r.buf.DeclareQueue(redis.TranscodeQueueName)

	// 走缓冲发布（连接可用时直发，不可用时入缓冲并自动重放）
	// 默认 exchange "" + routingKey=queueName 是 RabbitMQ 直发队列的标准用法
	return r.buf.Publish(ctx, "", redis.TranscodeQueueName, amqp091.Publishing{
		ContentType: "application/json",
		Body:        body,
	})
}
