# 生产级硬化（高可用 + 持久化）Spec

## Why
本项目当前在 **P0 高可用** 和 **持久化策略** 上存在明显缺口：
1. Redis 仅有 AOF（无 RDB 兜底、无混合持久化），重启恢复时间随 AOF 增长而劣化；
2. API 进程在收到信号时无法优雅停机（`defer app.Close()` 永远跑不到）；
3. RabbitMQ 单连接不可重连，MQ 抖动会导致播放量增量静默丢失；
4. 用户上传文件采用"直写目标文件"模式，进程中途崩溃可能留下半截文件导致图片/视频损坏；进程内 captcha store 会在重启后清空，让用户验证码失效；
5. 视频热度 ZSet 重建跑在 API 进程，多实例会互相覆盖（即便当前是单实例，worker 已存在，迁过去更合理）；
6. `GetVideoDetail` 缺少 Redis 全挂时的降级熔断，会把 MySQL 一起打挂。

**架构定位：当前为单体部署**（单 API + 单 worker + 单 Redis + 单 MySQL + 单 RabbitMQ），本次硬化聚焦 **单机场景下的风险收敛与可靠性提升**，**不引入云存储、不引入 K8s 探针、不引入跨实例 session**。

## What Changes
- **新增** `deploy/redis/redis.conf`：开启 RDB+AOF 混合持久化、`aof-use-rdb-preamble yes`、`appendfsync everysec`，附中文注释
- **新增** `deploy/redis/README.md`：混合持久化原理、调优参数说明、容量规划
- **修改** `docker-compose.yml`：redis service 挂载 `redis.conf`
- **修改** `server/configs/config.docker.yaml`、`config.compose-local.yaml`：在 `redis` 配置下加注释指向 `deploy/redis/README.md`
- **修改** `server/internal/initialize/redis.go`：移除"尝试 CONFIG SET 启用 AOF"的误导注释，改为启动时检查 AOF/RDB 状态并 Warn
- **重构** `server/internal/core/server.go`、`server_win.go`、`server_other.go`：用统一 `Server` 接口，移除 endless 依赖
- **新增** `server/internal/initialize/graceful.go`：跨平台信号处理（`signal.NotifyContext`）+ 30s shutdown 超时
- **修改** `server/cmd/api/main.go`：`SIGINT/SIGTERM` → `srv.Shutdown(ctx)` → `cron.Stop()` → `app.Close()`
- **修改** `server/cmd/worker/main.go`：同样信号处理 + 最后一次 flush
- **重构** `server/internal/initialize/rabbitmq.go`：拆出 `Connection` 类型（**BREAKING**：移除直接的 `Channel` 字段访问）
- **新增** `server/internal/repository/rabbitmq/connection.go`：自动重连 + NotifyClose 监听
- **新增** `server/internal/repository/rabbitmq/publish_buffer.go`：有界内存缓冲 + 重连后重放
- **修改** `server/internal/repository/rabbitmq/playcount.go`：通过 `Connection.Publish` 发布
- **重构** `server/cmd/worker/main.go`：消费者自动重订阅
- **修改** `server/internal/logic/user.go` `UploadAvatar`：改为"写临时文件 + 原子 rename"模式防损坏（**纯本地改动，零依赖**）
- **新增** `server/internal/repository/redis/captcha_store.go`：基于 Redis 的 captcha Store
- **修改** `server/internal/initialize/router.go`：captcha 切换为 Redis store
- **修改** `server/cmd/api/main.go` + `cmd/worker/main.go`：cron 只在 worker 启动（`app.run_cron: false`）
- **新增** `server/internal/breaker/`：极简熔断器（closed/open/half-open）
- **修改** `server/internal/logic/video.go`：`BackfillUserCache` / `BackfillFansCountCache` 走熔断

## Impact
- Affected specs: 进程生命周期、消息可靠性、缓存降级、单实例存储可靠性
- Affected code:
  - `cmd/api/main.go` — 信号处理 + 优雅停机
  - `cmd/worker/main.go` — cron 接管 + rabbit 重连消费者
  - `internal/core/server.go` + `server_win.go` + `server_other.go` — 统一 Server 接口
  - `internal/initialize/` — 新增 graceful 子模块
  - `internal/repository/rabbitmq/` — 重写为 resilient connection
  - `internal/repository/redis/captcha_store.go` — 新增
  - `internal/breaker/` — 新增
  - `internal/logic/user.go` — 上传原子 rename
  - `internal/logic/video.go` — 熔断包装
  - `internal/initialize/router.go` — captcha store 切换
  - `docker-compose.yml` — redis 持久化挂载
  - `deploy/redis/` — 新增
  - `configs/*.yaml` — 新增配置项

## ADDED Requirements

### Requirement: Redis 混合持久化配置
系统 SHALL 在生产部署（docker-compose）中启用 RDB + AOF 混合持久化，并通过挂载 `deploy/redis/redis.conf` 应用配置。

#### Scenario: 容器启动时加载混合持久化配置
- **WHEN** `docker-compose up` 启动 redis 服务
- **THEN** redis 进程加载挂载的 `redis.conf`，开启 `appendonly yes`、`aof-use-rdb-preamble yes`、RDB 快照三档（`900 1`/`300 10`/`60 10000`），AOF rewrite 阈值 100%/64MB，`appendfsync everysec`

#### Scenario: 重启恢复混合负载
- **WHEN** redis 进程被 kill 后再次启动
- **THEN** redis 自动以 RDB 格式重写 AOF 前缀，秒级恢复完整数据集

#### Scenario: 应用启动时检查持久化状态
- **WHEN** API/Worker 启动并连接 Redis
- **THEN** 通过 `CONFIG GET appendonly` / `CONFIG GET save` 校验服务端持久化策略；若 AOF/RDB 都未启用，输出 Warn 日志

### Requirement: 优雅停机
API 与 Worker 进程 SHALL 实现跨平台优雅停机，收到 `SIGINT` 或 `SIGTERM` 后完成"停止接收新请求 → 排空在途请求 → 关闭 cron/worker → 关闭下游连接"的完整流程。

#### Scenario: API 收到 SIGTERM
- **WHEN** 部署系统向 api 进程发送 `SIGTERM`
- **THEN** `srv.Shutdown(ctx)` 阻塞至多 30s，等待在途请求完成或超时；随后 `cron.Stop()`、`app.Close()`；主进程以 exit code 0 退出

#### Scenario: Worker 收到 SIGTERM
- **WHEN** 部署系统向 worker 进程发送 `SIGTERM`
- **THEN** 停止 `msgs` channel 消费，等待在途批量写回完成（最多 10s），然后 `app.Close()`

#### Scenario: Windows 下收到 Ctrl+C
- **WHEN** Windows 终端按下 Ctrl+C
- **THEN** `os.Interrupt` 信号触发同样的优雅停机流程（不依赖 endless）

#### Scenario: 在途请求超过 30s 未完成
- **WHEN** shutdown 超时
- **THEN** `srv.Shutdown` 返回 `context.DeadlineExceeded` 错误，记 Error 日志，强制继续后续清理

### Requirement: RabbitMQ 弹性连接
系统 SHALL 提供自动重连、内存缓冲、消费者重订阅的 RabbitMQ 连接抽象，MQ 抖动期间 publish 不丢失、consume 自动恢复。

#### Scenario: API 发布期间 MQ 短暂断开（< 5s）
- **WHEN** `Connection.Publish` 调用时 channel 不可用
- **THEN** 消息进入有界内存缓冲（容量 10000），异步重连成功后按 FIFO 顺序重放

#### Scenario: API 发布期间 MQ 长时间断开（> 5s）
- **WHEN** 缓冲队列满
- **THEN** 丢弃最旧的消息并 `Warn` 日志（保证内存可控，不阻塞 publish 调用方）

#### Scenario: Worker 消费者订阅断开
- **WHEN** worker 的 `msgs` channel 关闭
- **THEN** `Connection` 自动重连并 re-declare 队列、re-set Qos、re-Consume

#### Scenario: 启动时 MQ 不可用
- **WHEN** 进程启动时 `amqp.Dial` 失败
- **THEN** 启动 `reconnect goroutine`，按指数退避（1s→2s→4s→…→30s 上限）重试，**不阻塞进程启动**

### Requirement: 文件上传原子写入
用户头像上传 SHALL 使用"写临时文件 + 原子 rename"模式，防止进程中途崩溃留下半截文件导致图片损坏。**纯本地改动，零新增依赖**。

#### Scenario: 正常上传
- **WHEN** 用户上传 1MB 头像
- **THEN** 写入 `{avatarDir}/{filename}.tmp`，完成后 `os.Rename` 到目标路径；任意时刻只看到完整文件或不存在

#### Scenario: 进程中途崩溃
- **WHEN** 上传过程中进程被 kill
- **THEN** 磁盘上要么是完整的旧文件（如果有），要么只有 `.tmp` 残留（下次启动可清理）；目标文件不会被半截写入污染

### Requirement: Captcha 跨进程持久化
验证码 SHALL 从进程内 `base64Captcha.DefaultMemStore` 切换到 Redis 存储，防止 API 进程重启后用户正在填写的验证码失效。

#### Scenario: 进程重启后验证
- **WHEN** 用户从 API 实例获取 captcha，API 进程意外重启，用户提交校验
- **THEN** 通过 Redis 中残留的 captcha 数据校验成功（TTL 5min 内）

#### Scenario: 同一验证码多次提交
- **WHEN** 用户重复提交同一个 captcha id
- **THEN** 第一次校验成功后立即从 Redis 删除（`GETDEL` 原子），第二次返回"验证码已失效"

#### Scenario: 验证码过期
- **WHEN** captcha 存储超过 5 分钟
- **THEN** Redis 自动过期清理

### Requirement: Cron 仅在 Worker 进程运行
视频热度 ZSet 重建、用户/视频静态缓存同步等 cron 任务 SHALL 只在 worker 进程执行，避免多 API 实例重复重建（即便当前是单实例，职责也应在 worker）。

#### Scenario: API 进程启动
- **WHEN** `cmd/api/main.go` 启动
- **THEN** 检测 `app.run_cron` 配置，默认 `false`，不注册任何 cron 任务，输出"cron disabled in API process"日志

#### Scenario: Worker 进程启动
- **WHEN** `cmd/worker/main.go` 启动
- **THEN** 注册 `RebuildZSet`（@every 1m）、`SyncStaticCache`（@every 24h）、`SyncUserStaticCache`（@every 24h），并启动 cron 调度

#### Scenario: 偶发双 worker 部署
- **WHEN** 临时部署 2 个 worker 实例
- **THEN** `RebuildZSet` 通过 Redis 分布式锁（SET NX EX 300）保证同时只有 1 个实例执行

### Requirement: 核心接口降级熔断
`GetVideoDetail` 等核心读路径 SHALL 在 Redis/MySQL 异常时通过熔断器降级，避免下游雪崩。

#### Scenario: Redis 持续不可用（连续 5 次失败）
- **WHEN** 熔断器阈值触发（5 consecutive failures in 10s window）
- **THEN** 熔断器进入 Open 状态（30s），期间 `BackfillUserCache` / `BackfillFansCountCache` 调用立即返回 `ErrCircuitOpen`，不发起 MySQL 查询

#### Scenario: Open 状态 30s 后
- **WHEN** 冷却期结束
- **THEN** 熔断器进入 Half-Open，放行 1 个探测请求；成功则 Closed，失败则重新 Open 30s

#### Scenario: Redis 完全不可用时视频详情仍可返回
- **WHEN** 缓存层全挂、熔断器 Open
- **THEN** `GetVideoDetail` 直接走 MySQL 单次回源，返回基本视频信息（author 字段为空），`HTTP 200` + 业务码 0

## MODIFIED Requirements

### Requirement: 文件上传方式
`UserLogic.UploadAvatar` SHALL 改为原子写入模式（写临时文件 + `os.Rename`）。**BREAKING**：磁盘上文件路径不变，但写入过程改为 tmp 文件。

### Requirement: Captcha 存储后端
captcha 存储 SHALL 从 `base64Captcha.DefaultMemStore` 切换为 Redis 实现。**BREAKING**：进程重启时若 Redis 中无对应 captcha id，会返回"验证码已失效"（与之前进程重启表现一致，因为旧版也是内存存；改进点是支持进程重启后短时间内仍然有效）。

### Requirement: RabbitMQ Channel 访问
`Repos.PlayCountPublisher` 和 `cmd/worker/main.go` SHALL 通过 `rabbitmq.Connection` 抽象访问 channel，**不再直接持有 `*amqp091.Channel`**。**BREAKING**：现有直接调用 `app.RabbitMQ.Channel` 的代码需迁移到 `Connection`。

### Requirement: Cron 启动控制
`Config.App.RunCron` 配置项 SHALL 控制 cron 是否启动，默认 API 为 `false`、worker 为 `true`。

## REMOVED Requirements

### Requirement: 无限的 AOF 增长
**Reason**：纯 AOF 在写多的场景（缓存更新、ZSet rebuild）下文件会持续增长，重启恢复时间线性劣化。
**Migration**：切换到混合持久化后，AOF 头部为 RDB 快照（紧凑），尾部为增量 AOF；rewrite 触发后体积可控。

### Requirement: endless 信号处理
**Reason**：endless 依赖 fork 子进程实现热重启，跨平台行为差异大；与本项目"统一优雅停机"目标冲突。
**Migration**：用 `signal.NotifyContext` + `http.Server.Shutdown` 替代。endless 包将从 `go.mod` 移除。
