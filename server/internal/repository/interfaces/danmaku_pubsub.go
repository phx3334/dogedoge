package interfaces

import "context"

type DanmakuPubSub interface {
	Publish(ctx context.Context, videoID uint64, msg []byte) error
	Subscribe(ctx context.Context, videoID uint64) (<-chan []byte, error)
	Unsubscribe(ctx context.Context, videoID uint64) error
	// RemoveListener 移除指定 listener 的消息通道（客户端断开时调用）
	// 当所有 listener 都移除后，自动关闭底层 Redis 订阅
	RemoveListener(videoID uint64, targetCh <-chan []byte) error
}
