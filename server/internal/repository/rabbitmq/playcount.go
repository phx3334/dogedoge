package rabbitmq

import (
	"context"
	"encoding/json"

	"fake_tiktok/internal/dto/cache"
	"fake_tiktok/internal/repository/interfaces"
	"fake_tiktok/internal/repository/redis"

	amqp091 "github.com/rabbitmq/amqp091-go"
)

// 编译期断言：PlayCountPublisherRepo 必须实现 interfaces.PlayCountPublisher。
// 任何方法签名变更会在编译期被发现。
var _ interfaces.PlayCountPublisher = (*PlayCountPublisherRepo)(nil)

// PlayCountPublisherRepo 把"播放量 +1"事件发布到 RabbitMQ。
//
// 关键改造（Task 3）：
//   - 不再持有 *amqp091.Channel，改为持有 *PublishBuffer
//   - PublishBuffer 内部已封装 Connection + 有界缓冲 + 自动重放
//   - 调用方（PlayCountIncrement）完全感知不到"连接断开 / 重连中"等状态
//
// 为什么走缓冲而不是直连 publish：
//   - 播放量增量场景下 MQ 短暂抖动不应该让 API 请求失败
//   - PublishBuffer 在连接恢复后自动重放，业务方零感知
//   - 极端情况（broker 长时间不可用）缓冲满会丢最旧，但播放量是弱一致数据可接受
type PlayCountPublisherRepo struct {
	buf *PublishBuffer
}

// NewPlayCountPublisherRepo 构造一个播放量发布器。
//
// buf 必须已经调用过 Start(ctx)，否则 drainLoop 不会启动。
// 当前约定在 router 初始化时统一 Start，这里只接收已启动的实例。
func NewPlayCountPublisherRepo(buf *PublishBuffer) *PlayCountPublisherRepo {
	return &PlayCountPublisherRepo{buf: buf}
}

// PublishIncrement 把"videoID 增加 increment"事件发到播放量队列。
//
// 流程：
//  1. 把 (videoID, increment) 序列化为 JSON
//  2. 走 buf.Publish() 自动选择"直发 / 入缓冲"
//  3. 返回值：直发成功 → nil；入缓冲 → 原始错误（供调用方感知）
//
// 关于 QueueDeclare：
//   - 严格来说 worker 端启动时已声明过，这里再声明一次是幂等的
//   - 但调用 buf.Publish() 走的是"Connection 拿 channel"，channel 在重建后
//     是新对象，broker 端若队列被外部删除（运维误操作），本路径暂时不会感知
//   - 当前选择"不在 publish 路径中做 QueueDeclare"以减少与 broker 的额外 RTT；
//     实际生产中可考虑在 drainLoop 末尾周期性 verify 队列存在性
func (r *PlayCountPublisherRepo) PublishIncrement(ctx context.Context, videoID uint, increment int64) error {
	body, err := json.Marshal(cache.PlayCountIncrementMsg{VideoID: videoID, Increment: increment})
	if err != nil {
		// 序列化失败：业务方错误，不应入缓冲
		return err
	}

	// 走缓冲发布（连接可用时直发，不可用时入缓冲并自动重放）
	// 默认 exchange "" + routingKey=queueName 是 RabbitMQ 直发队列的标准用法
	return r.buf.Publish(ctx, "", redis.PlayCountQueueName, amqp091.Publishing{
		ContentType: "application/json",
		Body:        body,
	})
}
