# Checklist

## Task 1 — Redis 混合持久化
- [x] `deploy/redis/redis.conf` 存在，包含 `appendonly yes`、`aof-use-rdb-preamble yes`、`appendfsync everysec`、RDB 三档 `save` 指令、`auto-aof-rewrite-percentage 100` 等关键开关；每段有中文注释
- [x] `deploy/redis/README.md` 存在并解释混合模式工作原理、参数含义、容量规划
- [x] `docker-compose.yml` 的 redis service 已挂载 `redis.conf` 并通过配置文件启动
- [x] `initialize/redis.go` 的 `verifyPersistence` 启动时输出 AOF/RDB 状态；都未启用时 Warn 不阻断
- [x] 容器 `docker-compose up -d redis` 后 `redis-cli CONFIG GET appendonly` 返回 `yes`

## Task 2 — 跨平台优雅停机
- [x] `endless` 已从 `go.mod` 移除
- [x] `core.Server` 接口统一 `Start()` / `Shutdown(ctx)`
- [x] `server_win.go` 和 `server_other.go` 实现一致（都用 `*http.Server`）
- [x] `cmd/api/main.go` 使用 `signal.NotifyContext` 监听 SIGINT/SIGTERM
- [x] `cmd/worker/main.go` 同样使用 signal ctx
- [x] `go build ./...` 在 Windows + Linux 均通过

## Task 3 — RabbitMQ 弹性连接
- [x] `repository/rabbitmq/connection.go` 存在，实现指数退避重连
- [x] `repository/rabbitmq/publish_buffer.go` 存在，实现 10000 容量有界缓冲
- [x] `PlayCountPublisherRepo` 不再持有 `*amqp091.Channel`，改为 `*Connection`
- [x] `cmd/worker/main.go` 消费者在 channel 关闭后自动重订阅
- [x] `initialize/app.go` 的 `Close()` 优雅关闭 `Connection`（5s 超时）
- [x] 所有新增代码带详细中文注释（结构体、方法、关键字段）

## Task 4 — 上传原子 rename + Captcha 迁 Redis
- [x] `logic/user.go` 的 `UploadAvatar` 改为"写 .tmp + os.Rename"模式
- [x] 失败/异常路径清理 .tmp 残留
- [x] `repository/redis/captcha_store.go` 实现 `base64Captcha.Store`，使用 `GETDEL` 原子
- [x] `initialize/router.go` 注入 Redis captcha store
- [x] 所有新增/修改代码带详细中文注释

## Task 5 — Cron 移到 Worker
- [x] `Config.App.RunCron` 字段已添加，YAML 配置项 `app.run_cron`
- [x] `cmd/api/main.go` 默认不启动 cron（除非配置开启）
- [x] `cmd/worker/main.go` 启动 cron，注册 3 个任务（RebuildZSet / SyncStaticCache / SyncUserStaticCache）
- [x] `pkg/lock/redis_lock.go` 分布式锁实现存在（Acquire + Release + 续期）
- [x] `RebuildZSet` 通过 Redis 锁保证单 worker 执行
- [x] 所有新增/修改代码带详细中文注释

## Task 6 — 熔断器 + 降级
- [x] `internal/breaker/breaker.go` 实现三态状态机（closed/open/half_open）
- [x] `LogicDeps.Breakers` 字段已添加（Redis、MySQL 两组）
- [x] `logic/video.go` 中 `BackfillUserCache` / `BackfillFansCountCache` 走熔断
- [x] 熔断 Open 时 goroutine 立即返回，不阻塞响应
- [x] 所有新增/修改代码带详细中文注释

## Task 7 — 编译验证
- [x] `go mod tidy` 干净（endless 已移除，无未使用依赖）
- [x] `go build ./...` 通过
- [x] `go vet ./...` 无 warning
