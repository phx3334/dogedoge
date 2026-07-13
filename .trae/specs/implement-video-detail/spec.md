# 视频详情页后端逻辑 Spec

## Why
用户点击视频进入详情页时，后端需要一次性返回视频完整信息（基本信息、上传者信息、互动状态、播放量），同时触发播放量计数和弹幕房间加入。当前系统只有首页视频列表接口，缺少视频详情、播放量增量同步、弹幕实时通信等核心功能。

## What Changes
- 新增视频详情 API（GET /video/detail），返回视频完整信息、上传者信息、用户互动状态
- 新增用户静态信息 Redis 缓存（Hash），与视频静态缓存模式一致，由定时任务同步
- 将视频缓存中的 `author_name` 字段来源从独立 `user:name` Hash 改为用户静态缓存 Hash
- 新增播放量增量机制：Redis HINCRBY 原子自增 → RabbitMQ 异步消息 → Worker 消费者批量写回 MySQL
- 新增 WebSocket 弹幕系统：Gorilla WebSocket + Redis Pub/Sub 跨实例广播
- 新增弹幕历史查询 API（GET /video/danmaku）
- 新增弹幕发送 API（POST /video/danmaku），仅限已登录用户
- Worker 服务从空壳实现为 RabbitMQ 消费者（播放量批量写回）

## Impact
- Affected specs: 视频缓存体系（新增用户静态缓存）、定时任务（新增用户缓存同步）、Worker 服务（播放量消费者）、弹幕权限控制
- Affected code:
  - `internal/repository/redis/keys.go` — 新增用户静态缓存键和弹幕频道常量
  - `internal/repository/redis/batchcache.go` — 新增用户缓存读写方法
  - `internal/dto/cache/usercache.go` — 扩展 UserCacheData
  - `internal/dto/cache/videocache.go` — 新增播放量增量消息体
  - `internal/dto/request/video.go` — 新增请求结构体
  - `internal/dto/response/video.go` — 新增视频详情响应结构体
  - `internal/repository/interfaces/` — 新增接口定义
  - `internal/repository/mysql/` — 新增互动状态查询、弹幕查询
  - `internal/repository/redis/` — 新增用户缓存、播放量自增、弹幕 Pub/Sub
  - `internal/logic/video.go` — 新增视频详情逻辑
  - `internal/handler/video.go` — 新增视频详情、弹幕相关 handler
  - `internal/routers/video.go` — 新增路由
  - `internal/initialize/cron.go` — 新增用户缓存同步定时任务
  - `internal/initialize/router.go` — 注入新依赖
  - `internal/logic/enter.go` — 新增依赖字段
  - `internal/repository/repos.go` — 新增仓库字段
  - `cmd/worker/main.go` — 实现 RabbitMQ 消费者

## ADDED Requirements

### Requirement: 用户登录状态判断
系统 SHALL 通过 Account.Role 字段判断用户是否已登录。Role 为 User(1) 或 Admin(2) 时视为已登录，Role 为 Guest(0) 时视为未登录。此判断逻辑适用于所有需要区分登录状态的接口（视频详情互动状态、弹幕权限等）。

#### Scenario: 已登录用户
- **WHEN** 从 JWT 中解析出的 userID 对应的 Account.Role 为 User 或 Admin
- **THEN** 视为已登录用户，可访问所有功能（查看互动状态、发送弹幕、WebSocket 弹幕连接）

#### Scenario: 未登录用户
- **WHEN** 请求中无有效 JWT 或 JWT 解析出的 userID 对应的 Account.Role 为 Guest
- **THEN** 视为未登录用户，互动状态字段全部返回 false，禁止发送弹幕和 WebSocket 弹幕连接

### Requirement: 视频详情 API
系统 SHALL 提供 GET /video/detail 接口，接收 video_id 参数，返回视频完整信息。

#### Scenario: 已登录用户请求视频详情
- **WHEN** 已登录用户（Role 为 User 或 Admin）请求 GET /video/detail?video_id=42
- **THEN** 返回视频基本信息（标题、封面、时长、描述、播放地址、标签、分区）、上传者信息（用户名、头像、签名）、互动计数（播放/点赞/收藏/投币/弹幕/评论数）、当前用户互动状态（是否点赞、收藏、投币），HTTP 200

#### Scenario: 未登录用户请求视频详情
- **WHEN** 未登录用户（Role 为 Guest 或无 JWT）请求 GET /video/detail?video_id=42
- **THEN** 返回视频信息和互动计数，互动状态字段全部为 false，HTTP 200

#### Scenario: 视频不存在
- **WHEN** 请求不存在的 video_id
- **THEN** 返回错误提示，HTTP 200（业务错误码）

### Requirement: 用户静态信息 Redis 缓存
系统 SHALL 将用户基本信息缓存到 Redis Hash 中，与视频静态缓存模式一致。

#### Scenario: 读取用户缓存
- **WHEN** 查询用户信息时先查 Redis Hash
- **THEN** 命中则直接返回，未命中则回源 MySQL 并回填缓存

#### Scenario: 定时同步用户缓存
- **WHEN** 定时任务每 24 小时执行
- **THEN** 全量读取 MySQL 用户数据，Pipeline 写入 Redis Hash，过期时间与视频静态缓存一致（3 天）

### Requirement: 播放量增量同步
系统 SHALL 通过 Redis 原子自增 + RabbitMQ 异步消息实现播放量计数。

#### Scenario: 用户播放视频
- **WHEN** 用户请求视频详情
- **THEN** Redis 动态 Hash 的 play_count 字段 HINCRBY +1，同时向 RabbitMQ 发送播放增量消息

#### Scenario: Worker 消费播放增量消息
- **WHEN** Worker 从 RabbitMQ 拉取到播放增量消息
- **THEN** 等待 5 秒聚合窗口，批量将增量写回 MySQL video 表的 play_count 字段

### Requirement: WebSocket 弹幕系统
系统 SHALL 提供 WebSocket 弹幕实时通信功能，仅限已登录用户使用。

#### Scenario: 已登录用户进入视频页
- **WHEN** 已登录用户（Role 为 User 或 Admin）通过 WebSocket 连接 /ws/danmaku?video_id=42
- **THEN** 服务端将用户加入该视频的弹幕房间，返回最近弹幕历史

#### Scenario: 未登录用户尝试 WebSocket 连接
- **WHEN** 未登录用户（Role 为 Guest 或无 JWT）尝试 WebSocket 连接 /ws/danmaku?video_id=42
- **THEN** 服务端拒绝连接，返回 HTTP 401 或 WebSocket 关闭帧

#### Scenario: 已登录用户发送弹幕
- **WHEN** 已登录用户通过 WebSocket 发送弹幕消息
- **THEN** 弹幕写入 MySQL、发布到 Redis Pub/Sub 频道、广播给同房间所有在线用户

#### Scenario: 跨实例弹幕同步
- **WHEN** 不同服务实例上的用户在同一视频房间
- **THEN** 通过 Redis Pub/Sub 实现跨实例弹幕广播，所有实例的在线用户都能收到弹幕

#### Scenario: 用户断开连接
- **WHEN** 用户 WebSocket 断开
- **THEN** 从弹幕房间移除，清理资源

### Requirement: 弹幕历史查询
系统 SHALL 提供 GET /video/danmaku 接口查询视频弹幕历史，所有用户均可访问。

#### Scenario: 查询弹幕历史
- **WHEN** 请求 GET /video/danmaku?video_id=42
- **THEN** 返回该视频的弹幕列表（按视频时间排序）

### Requirement: 弹幕发送
系统 SHALL 提供 POST /video/danmaku 接口发送弹幕，仅限已登录用户。

#### Scenario: 已登录用户发送弹幕
- **WHEN** 已登录用户（Role 为 User 或 Admin）POST /video/danmaku，包含 video_id、content、video_time、color 等字段
- **THEN** 弹幕写入 MySQL，Redis Pub/Sub 广播，返回成功

#### Scenario: 未登录用户发送弹幕
- **WHEN** 未登录用户（Role 为 Guest 或无 JWT）POST /video/danmaku
- **THEN** 返回权限错误，HTTP 200（业务错误码提示未登录）

## MODIFIED Requirements

### Requirement: 视频静态缓存中 author_name 来源
视频静态缓存 Hash 中的 `author_name` 字段，其数据来源 SHALL 从用户静态缓存 Hash 中获取，而非独立的 `user:name` Hash。定时任务同步视频静态缓存时，也从用户静态缓存读取 author_name。

### Requirement: Worker 服务
Worker 服务 SHALL 实现 RabbitMQ 消费者，负责播放量增量消息的批量写回。消费者拉取消息后等待 5 秒聚合窗口，将同一 video_id 的增量合并后批量 UPDATE MySQL。

## REMOVED Requirements

### Requirement: 独立的 AuthorNameHashKey 缓存
**Reason**: 用户信息统一缓存到用户静态 Hash 后，author_name 可直接从用户缓存获取，独立的 `user:name` Hash 不再需要作为主要数据源
**Migration**: 保留 `AuthorNameHashKey` 常量定义避免编译错误，但新逻辑优先从用户静态缓存读取
