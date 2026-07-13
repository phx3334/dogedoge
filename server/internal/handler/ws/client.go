package ws

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"fake_tiktok/internal/breaker"
	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/dto/response"
	"fake_tiktok/internal/repository/interfaces"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// Client 表示一个 WebSocket 弹幕客户端连接
//
// 生命周期：
//   - 创建：ServeWS 中 NewClient + JoinRoom
//   - 运行：ReadPump + WritePump 两个 goroutine 并行运行
//   - 销毁：ReadPump 退出时执行 LeaveRoom + cleanup + Conn.Close
//
// 并发安全：
//   - Send 通道：由 WritePump 消费，BroadcastToRoom/ReadPump 生产
//   - Close()：sync.Once 保护，BroadcastToRoom 和 ReadPump 可安全并发调用
type Client struct {
	Hub            *DanmakuHub                // 所属的弹幕房间管理中心
	Conn           *websocket.Conn            // gorilla/websocket 底层连接
	Send           chan []byte                // 发送消息缓冲通道（容量 256），WritePump 从此读取并写入 WebSocket
	VideoID        uint64                     // 当前观看的视频 ID，用于加入对应的弹幕房间
	UserID         string                     // 当前用户 ID，空字符串表示只读模式（Guest 不允许发弹幕）
	DanmakuRepo    interfaces.DanmakuRepository   // 弹幕 MySQL 仓储，用于持久化弹幕
	DanmakuCacheRepo interfaces.DanmakuCacheRepository // 弹幕 Redis 缓存仓储，用于发送后实时更新缓存
	DanmakuPubSub  interfaces.DanmakuPubSub       // 弹幕 Redis Pub/Sub 仓储，用于跨实例广播
	VideoRepo      interfaces.VideoRepository     // 视频 MySQL 仓储，用于更新弹幕计数
	VideoCacheRepo interfaces.VideoCacheRepository // 视频缓存仓储，用于检查弹幕是否已关闭
	Breakers       *breaker.Group             // Redis/MySQL 熔断器组，用于弹幕关闭检查和 Pub/Sub 广播的快速失败
	Logger         *zap.Logger                // 日志记录器
	SubCh          <-chan []byte              // 当前客户端的 Pub/Sub 消息通道，由 Subscribe 返回
	CancelSub      context.CancelFunc         // 用于取消订阅转发 goroutine，客户端断开时调用
	closeOnce      sync.Once                  // 保证 Close() 只执行一次，防止并发 close channel panic
}

// NewClient 创建一个新的 WebSocket 弹幕客户端
func NewClient(hub *DanmakuHub, conn *websocket.Conn, videoID uint64, userID string, danmakuRepo interfaces.DanmakuRepository, danmakuCacheRepo interfaces.DanmakuCacheRepository, danmakuPubSub interfaces.DanmakuPubSub, videoRepo interfaces.VideoRepository, videoCacheRepo interfaces.VideoCacheRepository, breakers *breaker.Group, logger *zap.Logger) *Client {
	return &Client{
		Hub:              hub,
		Conn:             conn,
		Send:             make(chan []byte, 256),
		VideoID:          videoID,
		UserID:           userID,
		DanmakuRepo:      danmakuRepo,
		DanmakuCacheRepo: danmakuCacheRepo,
		DanmakuPubSub:    danmakuPubSub,
		VideoRepo:        videoRepo,
		VideoCacheRepo:   videoCacheRepo,
		Breakers:         breakers,
		Logger:           logger,
	}
}

// ReadPump 从客户端读取消息（弹幕发送），写入 MySQL 并广播到 Redis Pub/Sub
//
// 消息处理流程：
//  1. 读取 WebSocket 消息
//  2. 反序列化为 SendDanmakuReq
//  3. 检查弹幕是否已关闭（通过 Redis 缓存 + 熔断器保护）
//  4. 写入 MySQL 持久化（5 秒超时，防止慢查询阻塞 goroutine）
//  5. 通过 Redis Pub/Sub 广播（3 秒超时 + 熔断器保护，防止 Redis 不可用时阻塞）
//
// 连接管理：
//   - 设置 512 字节读限制，防止客户端发送超大消息
//   - 60 秒读超时，客户端无响应则断开
//   - Pong 处理器刷新读超时，实现心跳保活
//   - 退出时执行清理：LeaveRoom + cleanup + Conn.Close
func (c *Client) ReadPump() {
	defer func() {
		c.Hub.LeaveRoom(c.VideoID, c)
		c.cleanup()
		c.Conn.Close()
	}()

	// 读限制：防止单条消息过大占用内存
	c.Conn.SetReadLimit(512)
	// 读超时：60 秒内必须收到消息（包括 Pong），否则断开连接
	c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	// Pong 处理器：收到 Pong 时刷新读超时，与 WritePump 的 30 秒 Ping 配合实现心跳保活
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			// 读取失败（连接关闭/超时/协议错误），退出循环触发 defer 清理
			break
		}

		// Guest 用户（UserID 为空）只允许观看弹幕，不允许发送
		if c.UserID == "" {
			continue
		}

		// 反序列化弹幕请求，格式错误则静默忽略
		var req request.SendDanmakuReq
		if err := json.Unmarshal(message, &req); err != nil {
			continue
		}

		req.VideoID = c.VideoID

		// 检查弹幕是否已关闭
		// 通过熔断器包装 Redis 缓存查询：
		//   - Redis 正常时：查缓存判断 DanmakuClosed
		//   - Redis 熔断时：跳过检查，允许弹幕发送（fail-open）
		//     因为 Redis 不可用时无法判断弹幕是否关闭，选择不阻断用户操作
		//   - GetVideoCache 返回 error，闭包将错误返回给熔断器，
		//     使熔断器能感知 Redis 不可用并累计失败次数
		var danmakuClosed bool
		cacheCtx, cacheCancel := context.WithTimeout(context.Background(), 2*time.Second)
		redisErr := c.Breakers.Redis.Execute(func() error {
			videoMap, _, err := c.VideoCacheRepo.GetVideoCache(cacheCtx, []uint{uint(c.VideoID)})
			if err != nil {
				return err
			}
			if vd, ok := videoMap[uint(c.VideoID)]; ok && vd.DanmakuClosed {
				danmakuClosed = true
			}
			return nil
		})
		cacheCancel()
		if redisErr != nil {
			// Redis 熔断：跳过弹幕关闭检查，允许发送（fail-open）
			c.Logger.Debug("Redis circuit open, skip DanmakuClosed check", zap.Uint64("video_id", c.VideoID))
		}
		if danmakuClosed {
			errMsg, _ := json.Marshal(map[string]string{"error": "该视频已关闭弹幕"})
			c.Send <- errMsg
			continue
		}

		// 构建弹幕数据对象
		danmaku := &database.Danmaku{
			VideoID:   req.VideoID,
			UserID:    c.UserID,
			Content:   req.Content,
			VideoTime: req.VideoTime,
			Color:     req.Color,
			FontSize:  req.FontSize,
			// Type 字段在 DB 中为 NOT NULL 且无默认值，必须显式设置，否则 INSERT 失败
			Type:      "scroll",
			CreatedAt: time.Now(),
		}
		// FontSize/Color 兜底：DB 字段 NOT NULL
		if danmaku.FontSize == "" {
			danmaku.FontSize = "md"
		}
		if danmaku.Color == "" {
			danmaku.Color = "#ffffff"
		}

		// MySQL 持久化：使用带超时的 context，防止慢查询阻塞该 goroutine
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := c.DanmakuRepo.Create(ctx, danmaku); err != nil {
			cancel()
			c.Logger.Warn("create danmaku failed", zap.Error(err))
			continue
		}
		cancel() // Create 成功后立即释放 context 资源

		// 更新视频弹幕计数（MySQL + Redis 缓存）
		videoIDUint := uint(c.VideoID)
		_ = c.Breakers.MySQL.Execute(func() error {
			countCtx, countCancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer countCancel()
			return c.VideoRepo.IncrementDanmakuCount(countCtx, videoIDUint, 1)
		})
		_ = c.Breakers.Redis.Execute(func() error {
			countCtx, countCancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer countCancel()
			return c.VideoCacheRepo.IncrementDanmakuCount(countCtx, videoIDUint)
		})

		// 构造标准弹幕消息对象（与 HTTP 接口保持一致）
		danmakuItem := response.DanmakuItem{
			ID:        danmaku.ID,
			Content:   danmaku.Content,
			VideoTime: danmaku.VideoTime,
			Color:     danmaku.Color,
			FontSize:  danmaku.FontSize,
			UserID:    danmaku.UserID,
			CreatedAt: danmaku.CreatedAt.Unix(),
		}

		// 写入 Redis 弹幕缓存，保证 WebSocket 发送后列表刷新也能看到
		_ = c.Breakers.Redis.Execute(func() error {
			cacheCtx, cacheCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cacheCancel()
			return c.DanmakuCacheRepo.Create(cacheCtx, c.VideoID, &danmakuItem)
		})

		// 广播弹幕到 Redis Pub/Sub（跨实例同步）
		// 通过熔断器保护：Redis 不可用时快速失败，避免 3 秒超时阻塞 ReadPump
		broadcastMsg, _ := json.Marshal(danmakuItem)

		pubErr := c.Breakers.Redis.Execute(func() error {
			pubCtx, pubCancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer pubCancel()
			return c.DanmakuPubSub.Publish(pubCtx, c.VideoID, broadcastMsg)
		})
		if pubErr != nil {
			if errors.Is(pubErr, breaker.ErrCircuitOpen) {
				c.Logger.Debug("Redis circuit open, skip danmaku publish",
					zap.Uint64("video_id", c.VideoID))
			} else {
				c.Logger.Warn("publish danmaku to pubsub failed", zap.Error(pubErr))
			}
		}
	}
}

// WritePump 向客户端写入消息（弹幕推送 + 心跳）
//
// 消息来源：
//   - Send 通道：由 Hub.BroadcastToRoom 写入的弹幕消息
//   - 心跳 ticker：每 30 秒发送 Ping，客户端应回 Pong 刷新 ReadPump 的读超时
//
// 退出条件：
//   - Send 通道关闭（ReadPump 退出或 BroadcastToRoom 缓冲区满触发 Close）
//   - 写入失败（连接已断开）
//   - Ping 发送失败
func (c *Client) WritePump() {
	// 心跳 ticker：每 30 秒发送一次 Ping
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			// 设置写超时，防止对端不可达时写操作永久阻塞
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				// Send 通道已关闭，发送 WebSocket Close 帧通知对端
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				// 写入失败（连接已断开），退出
				return
			}
		case <-ticker.C:
			// 心跳：发送 Ping 帧，客户端应回 Pong
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				// Ping 发送失败（连接已断开），退出
				return
			}
		}
	}
}

// cleanup 清理客户端的 Pub/Sub 订阅资源
//
// 在 ReadPump 退出时调用，确保：
//   - 取消订阅转发 goroutine（通过 CancelSub）
//   - 从 Pub/Sub 的 listener 列表中移除当前客户端的通道
//   - 所有 listener 移除后，底层 Redis Pub/Sub 连接自动关闭
func (c *Client) cleanup() {
	// 取消订阅转发 goroutine
	if c.CancelSub != nil {
		c.CancelSub()
	}
	// 通过接口方法移除当前客户端的 listener channel，无需类型断言
	if c.SubCh != nil {
		_ = c.DanmakuPubSub.RemoveListener(c.VideoID, c.SubCh)
	}
}

// Close 安全关闭客户端的发送通道
//
// 使用 sync.Once 保证只关闭一次，原因：
//   - BroadcastToRoom 在客户端缓冲区满时会调用 Close()
//   - ReadPump 退出时也会通过 defer 间接触发 Close（通过 cleanup → SubCh 关闭 → WritePump 退出）
//   - 两个路径可能并发执行，sync.Once 防止重复 close channel 导致 panic
func (c *Client) Close() {
	c.closeOnce.Do(func() {
		close(c.Send)
	})
}
