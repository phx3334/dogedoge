# Tasks

> 原则：每个任务产生"可运行 + 可编译"的中间状态。先做基础设施（持久化、连接、信号），再做依赖切换（captcha、上传），最后做降级（breaker）。
> 范围：单体部署（单 API + 单 worker + 单 Redis + 单 MySQL + 单 RabbitMQ），不引入云存储 / K8s 探针 / 跨实例 session。

- [x] Task 1: Redis 混合持久化配置（基础设施）
  - [x] SubTask 1.1: 创建 `deploy/redis/redis.conf`，包含：基础安全（`requirepass`、`bind 0.0.0.0`、`protected-mode no`）、RDB 三档快照（`save 900 1` / `save 300 10` / `save 60 10000`）、AOF（`appendonly yes` / `appendfsync everysec` / `auto-aof-rewrite-percentage 100` / `auto-aof-rewrite-min-size 64mb`）、**混合模式关键开关** `aof-use-rdb-preamble yes`、内存上限 `maxmemory 2gb` / 淘汰策略 `maxmemory-policy allkeys-lru`、慢查询 `slowlog-log-slower-than 10000`，每段带中文注释
  - [x] SubTask 1.2: 创建 `deploy/redis/README.md`，写明：混合持久化原理（RDB 头 + 增量 AOF 尾）、AOF/RDB 各档参数含义、容量规划、误用风险（rewrite 期间 IO 抖动）、与 docker-compose 的对接方式
  - [x] SubTask 1.3: 修改 `docker-compose.yml` 的 redis service：`command: ["redis-server", "/usr/local/etc/redis/redis.conf"]`、`volumes` 添加 `./deploy/redis/redis.conf:/usr/local/etc/redis/redis.conf:ro`
  - [x] SubTask 1.4: 修改 `server/internal/initialize/redis.go` 的 `ConnectRedis`：把原注释中"尝试 CONFIG SET 启用 AOF"改为"启动时仅做 Ping，启动后由运维侧保证持久化配置"；新增 `verifyPersistence(ctx, client)` 在 `ConnectRedis` 末尾调用，通过 `CONFIG GET appendonly` / `CONFIG GET save` 校验；AOF+RDB 都未启用时 `Warn` 日志，**不阻断启动**
  - [x] SubTask 1.5: 修改 `server/configs/config.compose-local.yaml` 与 `config.docker.yaml` 的 `redis` 块，新增 3 行注释指向 `deploy/redis/README.md`

- [x] Task 2: 跨平台优雅停机
  - [x] SubTask 2.1: 修改 `server/internal/core/server.go`：将 `server interface` 改为 `Server interface { Start() error; Shutdown(ctx context.Context) error }`；重写 `RunServer(ctx, cfg, app, cron) error`：在 goroutine 中调 `srv.Start()`，主流程 `select { case err := <-started: ...; case <-ctx.Done(): ... }`；ctx 取消后按顺序执行 `srv.Shutdown(30s)` → `cron.Stop()` → `app.Close()`
  - [x] SubTask 2.2: 修改 `server/internal/core/server_win.go`：实现 `Server` 接口包装 `*http.Server`；新增 `Start()` 调用 `ListenAndServe()`，`Shutdown(ctx)` 透传
  - [x] SubTask 2.3: 修改 `server/internal/core/server_other.go`：移除 endless 引用（**BREAKING**），改用 `*http.Server` 实现 `Server` 接口（与 win 端一致）；`go.mod` 中移除 `github.com/fvbock/endless`
  - [x] SubTask 2.4: 修改 `server/cmd/api/main.go`：用 `signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)` 构造根 ctx；`defer app.Close()` 移到 `core.RunServer` 之后（保证 Shutdown 后才 Close）
  - [x] SubTask 2.5: 修改 `server/cmd/worker/main.go`：同样用 `signal.NotifyContext` + 优雅停机：ctx 取消后停止 `msgs` 消费循环、再执行最后一次 `flush()`、再 `app.Close()`，整体 ≤10s 完成

- [x] Task 3: RabbitMQ 弹性连接（依赖 Task 2 的信号处理）
  - [x] SubTask 3.1: 新建 `server/internal/repository/rabbitmq/connection.go`：
    - 结构体 `Connection`（私有 `conn` / `channel` / `mu sync.RWMutex` / `ready atomic.Bool` / `closed atomic.Bool` / `ctx` / `cancel` / `done chan struct{}` / `logger *zap.Logger` / `cfg`）
    - 方法 `NewConnection(cfg, logger) *Connection`（不立即建连，lazy connect）
    - 方法 `Run(ctx)` 阻塞，启动重连循环：指数退避（1s→2s→4s→…→30s），成功后监听 `conn.NotifyClose` 触发重连
    - 方法 `Channel() (*amqp091.Channel, error)` 返回当前 channel（ready 状态）
    - 方法 `WaitReady(ctx) (*amqp091.Channel, error)` 阻塞等待 ready（带 ctx 取消）
    - 方法 `Close()` 主动关闭并停止重连
  - [x] SubTask 3.2: 新建 `server/internal/repository/rabbitmq/publish_buffer.go`：
    - 有界缓冲 `ringBuffer`（容量 10000，丢最旧）
    - 方法 `Publish(ctx, exchange, routingKey, msg) error`：先 `WaitReady`（最多 100ms），拿到 channel 后 `PublishWithContext`；失败入缓冲；缓冲满丢最旧 + Warn
    - 后台协程 `drainLoop`：每 1s 检查 `ready` 状态，ready 时按 FIFO 顺序重放缓冲
  - [x] SubTask 3.3: 修改 `server/internal/repository/rabbitmq/playcount.go`：`PlayCountPublisherRepo` 改为持有 `*Connection` 而非 `*amqp091.Channel`；`PublishIncrement` 改为 `conn.Publish(...)`
  - [x] SubTask 3.4: 修改 `server/internal/initialize/rabbitmq.go`：
    - 删除直接 `amqp.Dial` + 暴露 `Conn`/`Channel` 字段（**BREAKING**）
    - 新增 `NewRabbitMQConn(cfg, logger) *rabbitmq.Connection` 返回 `Connection`
    - 启动时 `go conn.Run(ctx)` 异步重连
  - [x] SubTask 3.5: 修改 `server/internal/repository/repos.go`：`PlayCountPublisher` 字段类型不变（仍是 `interfaces.PlayCountPublisher`），实现类改为通过 `Connection` 构造
  - [x] SubTask 3.6: 修改 `server/cmd/worker/main.go`：
    - 使用 `Connection` 抽象
    - 新增 `consumeLoop(ctx, conn)`：循环内 `conn.WaitReady(ctx)` → `QueueDeclare` → `Qos` → `Consume` → 循环 `for msg := range msgs` 处理
    - channel 关闭时（`msgs` 退出）回到外层 `WaitReady` 重新订阅
  - [x] SubTask 3.7: 修改 `server/internal/initialize/app.go`：`Close()` 中改为关闭 `*rabbitmq.Connection`（优雅停 channel、conn，等待最多 5s）

- [x] Task 4: 上传原子 rename + Captcha 迁 Redis（最小化改动）
  - [x] SubTask 4.1: 修改 `server/internal/logic/user.go` 的 `UploadAvatar`：
    - 在创建文件前计算 `tmpPath = savePath + ".tmp"`
    - 写文件到 tmpPath（`os.Create` + `dst.ReadFrom`）
    - 写入成功后 `os.Rename(tmpPath, savePath)` 原子提交
    - 失败/异常路径删除 tmp 残留
    - 增加详细中文注释解释原子 rename 原理
  - [x] SubTask 4.2: 新建 `server/internal/repository/redis/captcha_store.go`：
    - 实现 `base64Captcha.Store` 接口（`Set/Get/Verify`）
    - 键 `captcha:{id}`，值 `{answer}`，TTL 5 分钟
    - `Set` 用 `SET ... EX 300`
    - `Verify` 用 `GETDEL` 原子获取并删除（防止重放）；比较 answer（大小写不敏感）
    - 详细中文注释
  - [x] SubTask 4.3: 修改 `server/internal/initialize/router.go`：将 `LogicDeps.CaptchaStore` 改为 `redis_repo.NewRedisCaptchaStore(redisClient)`（替换 `base64Captcha.DefaultMemStore`）

- [x] Task 5: Cron 移到 Worker（依赖 Task 2、Task 3）
  - [x] SubTask 5.1: 修改 `server/internal/config/loadconfig.go`：`Config` 结构体新增 `App AppConfig { RunCron bool }` 字段；YAML key `app.run_cron`
  - [x] SubTask 5.2: 修改 `server/configs/config.compose-local.yaml` 和 `config.docker.yaml`：新增 `app: { run_cron: false }`
  - [x] SubTask 5.3: 修改 `server/cmd/api/main.go`：`if cfg.App.RunCron { ... }` 包裹 `StartCronTasks` 调用；默认 false 时只输出"cron disabled"日志
  - [x] SubTask 5.4: 修改 `server/cmd/worker/main.go`：在 main 启动时若 `cfg.App.RunCron` 为 true 则启动 cron 调度，注册与 API 端相同的 3 个任务；引入 Redis 分布式锁 `cron:rebuild_zset`（SET NX EX 300 + 续期 goroutine）保证同时只有 1 个 worker 执行 `RebuildZSet`
  - [x] SubTask 5.5: 修改 `server/internal/initialize/cron.go`：把 `StartCronTasks` 改造为可被两个进程复用：参数化 `app *App` 不变；提供 `BuildCronJobs(app) []cron.Job` 拆出任务注册逻辑
  - [x] SubTask 5.6: 新建 `server/internal/pkg/lock/redis_lock.go`：极简 Redis 分布式锁：`Acquire(ctx, key, ttl) (release func(), err error)`；释放用 Lua 脚本（GET + DEL 原子）；续期通过 `context.AfterFunc` 定时 PEXPIRE

- [x] Task 6: 熔断器 + 降级（依赖 Task 3 完成）
  - [x] SubTask 6.1: 新建 `server/internal/breaker/breaker.go`：
    - 结构体 `Breaker`（状态机 closed/open/half_open + `consecutiveFailures` / `lastStateChange` / `nextRetry`）
    - 阈值配置：`FailureThreshold=5` / `OpenDuration=30s` / `HalfOpenMaxRequests=1`
    - 方法 `Execute(fn func() error) error`：closed 直调；open 直接 `ErrCircuitOpen`；half_open 放行 1 个
    - 方法 `State()` 返回当前状态（用于 metrics/log）
    - 详细中文注释
  - [x] SubTask 6.2: 新建 `server/internal/initialize/breaker.go`：初始化全局 `*Breaker`（per-dependency：RedisBreaker、MySQLBreaker）；注入到 `LogicDeps.Breakers`
  - [x] SubTask 6.3: 修改 `server/internal/logic/enter.go`：`LogicDeps` 新增 `Breakers *BreakerGroup` 字段（按域分组：`Redis`、`MySQL`）
  - [x] SubTask 6.4: 修改 `server/internal/logic/video.go` 第 280-340 行：
    - `BackfillUserCache` / `BackfillFansCountCache` 改为 `breakers.MySQL.Execute(func() error { ... })`
    - 熔断 Open 时记录 Warn 日志，goroutine 立即返回空值（不阻塞响应）
  - [x] SubTask 6.5: 修改 `server/internal/initialize/router.go` 注入 `BreakerGroup` 到 `LogicDeps`

- [x] Task 7: 编译验证
  - [x] SubTask 7.1: `go mod tidy` 移除 endless 依赖，确认 `go.mod` 中 `require` 干净
  - [x] SubTask 7.2: `go build ./...` 通过，无 warning
  - [x] SubTask 7.3: `go vet ./...` 通过

# Task Dependencies
- [Task 1] → standalone
- [Task 2] → standalone
- [Task 3] depends on [Task 2]（需要 signal ctx 驱动重连循环的退出）
- [Task 4] → standalone（纯本地改动，零依赖）
- [Task 5] depends on [Task 2, Task 3]（worker 端 cron 启动依赖 Connection + signal ctx）
- [Task 6] depends on [Task 3]（breaker 包装 Connection 的 Ping）
- [Task 7] depends on [Task 1..6]
