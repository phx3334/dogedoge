package rabbitmq

import (
	"context"
	"encoding/json"

	"fake_tiktok/internal/dto/cache"
	"fake_tiktok/internal/repository/interfaces"
	"fake_tiktok/internal/repository/redis"

	amqp091 "github.com/rabbitmq/amqp091-go"
)

var _ interfaces.UserPlayCountPublisher = (*UserPlayCountPublisherRepo)(nil)

// UserPlayCountPublisherRepo 把"用户总播放量 +1"事件发布到 RabbitMQ
type UserPlayCountPublisherRepo struct {
	buf *PublishBuffer
}

func NewUserPlayCountPublisherRepo(buf *PublishBuffer) *UserPlayCountPublisherRepo {
	return &UserPlayCountPublisherRepo{buf: buf}
}

func (r *UserPlayCountPublisherRepo) PublishUserIncrement(ctx context.Context, userID string, increment int64) error {
	body, err := json.Marshal(cache.UserPlayCountIncrementMsg{UserID: userID, Increment: increment})
	if err != nil {
		return err
	}
	return r.buf.Publish(ctx, "", redis.UserPlayCountQueueName, amqp091.Publishing{
		ContentType: "application/json",
		Body:        body,
	})
}
