package ws

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"fake_tiktok/internal/breaker"
	"fake_tiktok/internal/config"
	"fake_tiktok/internal/pkg"
	"fake_tiktok/internal/repository/interfaces"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// upgrader 负责将 HTTP 连接升级为 WebSocket 连接
//
// 安全性说明：
//   - CheckOrigin 校验请求来源，防止跨站 WebSocket 劫持（CSWSH）攻击
//   - 生产环境应配置 AllowedOrigins 白名单，开发环境可设为 ["*"] 允许所有来源
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     checkOrigin,
}

// checkOrigin 校验 WebSocket 升级请求的 Origin 头
//
// 策略：
//   - 如果请求没有 Origin 头（如同源请求或非浏览器客户端），允许通过
//   - 如果 Origin 在 AllowedOrigins 白名单中，允许通过
//   - AllowedOrigins 包含 "*" 时允许所有来源（仅限开发环境）
//   - AllowedOrigins 为空时允许所有来源（未配置则不限制，保持向后兼容）
//   - 其他情况拒绝，防止跨站 WebSocket 劫持攻击
func checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// 非浏览器客户端或同源请求，没有 Origin 头，允许通过
		return true
	}

	allowed := allowedOrigins

	// 未配置白名单时允许所有来源（向后兼容）
	if len(allowed) == 0 {
		return true
	}

	for _, o := range allowed {
		if o == "*" {
			return true
		}
		// 支持带端口的 Origin 匹配，如 "http://localhost:3000"
		if strings.EqualFold(origin, o) {
			return true
		}
	}

	return false
}

// allowedOrigins 存储从配置加载的 WebSocket Origin 白名单
// 由 SetAllowedOrigins 在应用启动时设置
var allowedOrigins []string

// SetAllowedOrigins 设置 WebSocket 允许的 Origin 白名单
// 应在应用启动时调用，将配置中的 allowed_origins 传入
func SetAllowedOrigins(origins []string) {
	allowedOrigins = origins
}

// ServeWS 处理 WebSocket 升级请求，建立弹幕实时连接
//
// 完整流程：
//  1. 解析 video_id 参数，确定客户端要加入的弹幕房间
//  2. 验证用户身份（仅 User/Admin 可使用弹幕，Guest 返回 401）
//  3. 调用 upgrader.Upgrade 将 HTTP 连接升级为 WebSocket
//  4. 创建 Client 并加入 Hub 对应的视频房间
//  5. 订阅 Redis Pub/Sub 频道，获取独立的消息通道
//  6. 启动订阅转发 goroutine：将 Pub/Sub 消息转发到 Hub 广播
//  7. 启动读写泵 goroutine 处理消息收发
//
// 注意：Subscribe 失败时关闭 WebSocket 连接，因为客户端无法收到他人弹幕，
// 继续保持连接会造成用户困惑（能发但看不到别人的弹幕）
func ServeWS(hub *DanmakuHub, w http.ResponseWriter, r *http.Request, jwtConfig *config.JWTConfig, accountRepo interfaces.AccountRepository, danmakuRepo interfaces.DanmakuRepository, danmakuCacheRepo interfaces.DanmakuCacheRepository, danmakuPubSub interfaces.DanmakuPubSub, videoRepo interfaces.VideoRepository, videoCacheRepo interfaces.VideoCacheRepository, breakers *breaker.Group, logger *zap.Logger) {
	// 1. 解析 video_id 参数
	videoIDStr := r.URL.Query().Get("video_id")
	videoID, err := strconv.ParseUint(videoIDStr, 10, 64)
	if err != nil || videoID == 0 {
		http.Error(w, "invalid video_id", http.StatusBadRequest)
		return
	}

	// 2. 验证用户身份
	// 从 Header、Cookie 或 URL query 中获取 access token（浏览器 WebSocket 无法设置自定义 Header，必须通过 query 或 cookie 传 token）
	// Guest 用户（Role=0）不允许发送弹幕，返回 401
	var userID string
	accessToken := r.URL.Query().Get("token")
	if accessToken == "" {
		accessToken = r.Header.Get("x-access-token")
	}
	if accessToken == "" {
		cookie, err := r.Cookie("x-access-token")
		if err == nil {
			accessToken = cookie.Value
		}
	}

	if accessToken != "" {
		j := pkg.NewJWT(jwtConfig)
		claims, err := j.ParseAccessToken(accessToken)
		if err == nil && claims != nil {
			account, err := accountRepo.FindByID(r.Context(), claims.UserID)
			if err == nil && account != nil && account.Role != 0 {
				userID = account.ID
			}
		}
	}

	if userID == "" {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	// 3. 升级为 WebSocket 连接
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	// 4. 创建客户端并加入弹幕房间
	client := NewClient(hub, conn, videoID, userID, danmakuRepo, danmakuCacheRepo, danmakuPubSub, videoRepo, videoCacheRepo, breakers, logger)
	hub.JoinRoom(videoID, client)

	// 5. 订阅 Redis Pub/Sub 频道
	// 每个客户端获取独立的消息通道，底层共享同一个 Redis Pub/Sub 连接
	subCh, err := danmakuPubSub.Subscribe(r.Context(), videoID)
	if err != nil {
		// 订阅失败：关闭 WebSocket 连接
		// 原因：客户端无法收到他人弹幕，继续保持连接会造成用户困惑
		logger.Warn("subscribe danmaku pubsub failed, closing WebSocket",
			zap.Uint64("video_id", videoID), zap.Error(err))
		hub.LeaveRoom(videoID, client)
		conn.Close()
		return
	}
	client.SubCh = subCh

	// 6. 启动订阅转发 goroutine
	// 将 Redis Pub/Sub 消息转发到 Hub 广播，由 Hub 分发给房间内所有客户端
	// 使用可取消的 context，客户端断开时停止转发，防止 goroutine 泄漏
	subCtx, cancelSub := context.WithCancel(context.Background())
	client.CancelSub = cancelSub

	go func() {
		for {
			select {
			case msg, ok := <-subCh:
				if !ok {
					// Pub/Sub 通道已关闭（Redis 连接断开或客户端断开）
					return
				}
				hub.BroadcastToRoom(videoID, msg)
			case <-subCtx.Done():
				// 客户端断开，取消转发
				return
			}
		}
	}()

	// 7. 启动读写泵
	// ReadPump：从客户端读取弹幕消息，持久化到 MySQL 并广播
	// WritePump：向客户端推送弹幕消息和心跳
	go client.WritePump()
	go client.ReadPump()
}
