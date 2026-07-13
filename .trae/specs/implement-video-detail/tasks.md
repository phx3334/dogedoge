# Tasks

- [x] Task 1: Redis 键常量与用户缓存 DTO 扩展
  - [x] SubTask 1.1: 在 `redis/keys.go` 中新增用户静态缓存键常量 `UserStaticHashKey = "user:static"`、过期时间 `UserStaticHashExpire = 3 * 24 * time.Hour`、弹幕 Pub/Sub 频道前缀 `DanmakuChannelPrefix = "danmaku:room:"`、播放增量队列名 `PlayCountQueueName = "video:play_count_increment"`
  - [x] SubTask 1.2: 扩展 `dto/cache/usercache.go` 的 `UserCacheData`，确保包含 username、avatar_url、signature、video_count 等字段，新增 `UserCacheWriteItem` 用于批量写入
  - [x] SubTask 1.3: 在 `dto/cache/videocache.go` 中新增 `PlayCountIncrementMsg` 结构体（VideoID uint, Increment int64），用于 RabbitMQ 消息体序列化

- [x] Task 2: 用户静态缓存 Redis 读写实现
  - [x] SubTask 2.1: 在 `repository/interfaces/` 新增 `UserCacheRepository` 接口，定义 `GetUserCache(ctx, userID string) (*cache.UserCacheData, error)` 和 `BatchWriteUserCache(ctx, items []cache.UserCacheWriteItem)` 方法
  - [x] SubTask 2.2: 在 `repository/redis/` 新建 `usercache.go`，实现 `UserCacheRepo`，Pipeline 批量读取/写入用户静态 Hash，编译期校验接口
  - [x] SubTask 2.3: 在 `repository/repos.go` 中注册 `UserCacheRepo`

- [x] Task 3: 视频详情相关 MySQL 查询
  - [x] SubTask 3.1: 在 `repository/interfaces/video.go` 的 `VideoRepository` 接口中新增 `FindVideoByID(ctx, id uint) (*database.Video, error)` 方法
  - [x] SubTask 3.2: 在 `repository/mysql/video.go` 中实现 `FindVideoByID`
  - [x] SubTask 3.3: 新建 `repository/interfaces/interaction.go`，定义 `InteractionRepository` 接口：`GetUserVideoInteraction(ctx, userID string, videoID uint) (*InteractionStatus, error)`，InteractionStatus 包含 IsLiked/IsFavorited/IsCoined bool 字段
  - [x] SubTask 3.4: 新建 `repository/mysql/interaction.go`，实现 `InteractionRepo`，查询 VideoLike/VideoFavorite/VideoCoin 三张表判断互动状态
  - [x] SubTask 3.5: 在 `repository/repos.go` 中注册 `InteractionRepo`

- [x] Task 4: 播放量增量 Redis + RabbitMQ 实现
  - [x] SubTask 4.1: 在 `repository/interfaces/batch_cache.go` 的 `BatchCacheRepository` 接口中新增 `IncrementPlayCount(ctx, videoID uint) error` 方法
  - [x] SubTask 4.2: 在 `repository/redis/batchcache.go` 中实现 `IncrementPlayCount`：对视频动态 Hash 的 play_count 字段执行 HINCRBY +1
  - [x] SubTask 4.3: 新建 `repository/interfaces/play_count.go`，定义 `PlayCountPublisher` 接口：`PublishIncrement(ctx, videoID uint, increment int64) error`
  - [x] SubTask 4.4: 新建 `repository/rabbitmq/playcount.go`，实现 `PlayCountPublisher`：将 `PlayCountIncrementMsg` JSON 序列化后发布到 RabbitMQ 的 `video:play_count_increment` 队列
  - [x] SubTask 4.5: 在 `repository/repos.go` 中注册 `PlayCountPublisher`

- [x] Task 5: 弹幕相关 MySQL 查询
  - [x] SubTask 5.1: 新建 `repository/interfaces/danmaku.go`，定义 `DanmakuRepository` 接口：`FindByVideoID(ctx, videoID uint64, limit int) ([]database.Danmaku, error)`、`Create(ctx, danmaku *database.Danmaku) error`
  - [x] SubTask 5.2: 新建 `repository/mysql/danmaku.go`，实现 `DanmakuRepo`
  - [x] SubTask 5.3: 在 `repository/repos.go` 中注册 `DanmakuRepo`

- [x] Task 6: 弹幕 Redis Pub/Sub 实现
  - [x] SubTask 6.1: 新建 `repository/interfaces/danmaku_pubsub.go`，定义 `DanmakuPubSub` 接口：`Publish(ctx, videoID uint64, msg []byte) error`、`Subscribe(ctx, videoID uint64) (<-chan []byte, error)`、`Unsubscribe(ctx, videoID uint64) error`
  - [x] SubTask 6.2: 新建 `repository/redis/danmaku_pubsub.go`，实现 `DanmakuPubSubRepo`：使用 Redis Pub/Sub，频道名为 `BuildKey(DanmakuChannelPrefix, videoID字符串)`
  - [x] SubTask 6.3: 在 `repository/repos.go` 中注册 `DanmakuPubSubRepo`

- [x] Task 7: 视频详情 Logic 层
  - [x] SubTask 7.1: 在 `dto/request/video.go` 新增 `VideoDetailReq`（VideoID uint）、`SendDanmakuReq`（VideoID uint64, Content string, VideoTime float64, Color string, FontSize string）
  - [x] SubTask 7.2: 在 `dto/response/video.go` 新增 `VideoDetailResp`（包含视频信息、上传者信息、互动计数、互动状态）、`DanmakuItem`（弹幕条目）、`AuthorInfo`（上传者信息）
  - [x] SubTask 7.3: 在 `logic/video.go` 新增 `GetVideoDetail(ctx, userID string, videoID uint)` 方法：先查视频缓存（复用 BatchCacheRepo.GetVideoCache），未命中回源 MySQL；查用户缓存获取上传者信息；判断用户登录状态（Role != Guest 才查互动状态）；触发播放量自增+MQ发送
  - [x] SubTask 7.4: 在 `logic/video.go` 新增 `GetDanmakuList(ctx, videoID uint64)` 和 `SendDanmaku(ctx, userID string, req request.SendDanmakuReq)` 方法，`SendDanmaku` 需校验用户 Role 不是 Guest

- [x] Task 8: WebSocket 弹幕 Hub 实现
  - [x] SubTask 8.1: 新建 `internal/ws/` 目录，创建 `hub.go`：实现 `DanmakuHub` 结构体，管理所有视频房间的客户端连接（`map[uint64]map[*Client]bool`），提供 `JoinRoom`/`LeaveRoom`/`BroadcastToRoom` 方法
  - [x] SubTask 8.2: 新建 `internal/ws/client.go`：实现 `Client` 结构体，封装 WebSocket 连接、读泵（读取客户端消息写入 MySQL + Pub/Sub 广播，需校验用户 Role 不是 Guest 才允许发送）、写泵（从 channel 读取消息写入 WebSocket）
  - [x] SubTask 8.3: 新建 `internal/ws/handler.go`：实现 `ServeWS` 函数，升级 HTTP 为 WebSocket，校验用户登录状态（Role 为 Guest 则拒绝连接返回 401），创建 Client 注册到 Hub，启动读写泵 goroutine，订阅 Redis Pub/Sub 频道

- [x] Task 9: Handler + Router 层
  - [x] SubTask 9.1: 在 `handler/video.go` 新增 `GetVideoDetail`、`GetDanmakuList`、`SendDanmaku` 方法，`SendDanmaku` 需从 JWT 获取 userID 并校验 Role 不是 Guest
  - [x] SubTask 9.2: 在 `handler/enter.go` 的 `HandlerGroup` 中注入 WebSocket Hub 依赖
  - [x] SubTask 9.3: 在 `routers/video.go` 新增路由：`GET /video/detail`（public，handler 内部判断登录状态）、`GET /video/danmaku`（public）、`POST /video/danmaku`（private，JWT 中间件保证已登录）、`GET /ws/danmaku`（public，WebSocket 升级时内部校验登录状态）

- [x] Task 10: 依赖注入整合
  - [x] SubTask 10.1: 更新 `logic/enter.go` 的 `LogicDeps`：新增 `UserCacheRepo`、`InteractionRepo`、`DanmakuRepo`、`DanmakuPubSub`、`PlayCountPublisher` 字段
  - [x] SubTask 10.2: 更新 `repository/repos.go`：新增 `UserCacheRepo`、`InteractionRepo`、`DanmakuRepo`、`DanmakuPubSubRepo`、`PlayCountPublisher` 字段和初始化
  - [x] SubTask 10.3: 更新 `initialize/router.go`：注入新依赖到 LogicDeps 和 HandlerGroup
  - [x] SubTask 10.4: 在 `initialize/router.go` 中初始化 `DanmakuHub` 并注入

- [x] Task 11: 定时任务 — 用户静态缓存同步
  - [x] SubTask 11.1: 在 `pkg/task/video_ranking.go` 的 `VideoRankingTask` 中新增 `SyncUserStaticCache(ctx)` 方法：全量读取 MySQL Account 表，Pipeline 写入 Redis 用户静态 Hash
  - [x] SubTask 11.2: 在 `initialize/cron.go` 中注册 `@every 24h` 用户缓存同步定时任务

- [x] Task 12: Worker 服务 — 播放量消费者
  - [x] SubTask 12.1: 在 `cmd/worker/main.go` 中实现 RabbitMQ 消费者：连接 RabbitMQ、声明队列、消费 `video:play_count_increment` 队列
  - [x] SubTask 12.2: 实现 5 秒聚合窗口逻辑：收到消息后启动/重置 5 秒定时器，窗口到期时按 video_id 合并增量，批量执行 `UPDATE videos SET play_count = play_count + ? WHERE id = ?`

- [x] Task 13: 编译验证与错误修复
  - [x] SubTask 13.1: 执行 `go build ./...` 确保编译通过
  - [x] SubTask 13.2: 修复所有编译错误

# Task Dependencies
- [Task 2] depends on [Task 1]
- [Task 3] depends on [Task 1]
- [Task 4] depends on [Task 1]
- [Task 5] depends on [Task 1]
- [Task 6] depends on [Task 1]
- [Task 7] depends on [Task 2, Task 3, Task 4, Task 5, Task 6]
- [Task 8] depends on [Task 6]
- [Task 9] depends on [Task 7, Task 8]
- [Task 10] depends on [Task 2, Task 3, Task 4, Task 5, Task 6, Task 8]
- [Task 11] depends on [Task 2]
- [Task 12] depends on [Task 4]
- [Task 13] depends on [Task 9, Task 10, Task 11, Task 12]
