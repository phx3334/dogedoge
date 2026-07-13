# 用户主页与弹幕管控 Spec

## Why
当前用户主页（GET /user/home）返回空数据（stub 实现），弹幕系统未校验视频的 `danmaku_closed` 字段，且用户缓存缺少 `total_play_count` 的读写。需要完善用户主页返回完整信息、弹幕管控、用户总播放量原子自增等功能。

## What Changes
- 修复 `UserCacheRepo` 缺失 `total_play_count` 字段的读写
- 弹幕管控：`GetDanmakuList` 和 `SendDanmaku` 校验 `DanmakuClosed` 字段，关闭时返回空/拒绝发送
- 实现用户主页完整逻辑：返回用户信息、粉丝数、关注数、收藏夹（按隐私设置）、作者视频列表
- `GetVideoDetail` 中原子自增用户总播放量，并通过消息队列异步同步 MySQL
- 新增关注数缓存（Redis Hash，与粉丝数缓存模式一致）
- 新增收藏夹查询接口和作者视频按时间排序查询（复用 `idx_author_time` 复合索引）
- 新增用户播放量增量消息队列 `user:play_count_increment`，Worker 端消费并批量更新 accounts 表
- 所有新增 MySQL 查询路径通过熔断器保护
- 新增信号量（Semaphore）限制数据库回写和频繁查询的并发数
- 频繁调用的 Redis 操作（视频缓存批量查询、互动缓存批量查询、用户缓存查询、播放量自增）通过 Redis 熔断器保护
- 所有新增 Redis/MySQL 操作设置合适的超时时间

## Impact
- Affected specs: implement-video-detail, production-hardening
- Affected code:
  - `dto/response/user.go` — 扩展 UserCard 为 UserHomeResp
  - `dto/cache/usercache.go` — 新增 UserPlayCountIncrementMsg
  - `dto/cache/videocache.go` — 无变更
  - `logic/user.go` — 实现 UserCard 完整逻辑
  - `logic/video.go` — GetVideoDetail 新增用户总播放量自增
  - `logic/danmaku.go` — 新增 DanmakuClosed 校验
  - `repository/interfaces/` — 新增 InteractionRepo.GetFollowingCount、VideoRepo.FindPublishedVideosByAuthorID、FavoriteFolderRepository
  - `repository/mysql/` — 实现上述接口
  - `repository/redis/` — 修复 UserCacheRepo total_play_count、新增关注数缓存、新增用户播放量自增
  - `repository/redis/keys.go` — 新增 FollowingCount 和 UserPlayCount 队列常量
  - `repository/rabbitmq/` — 新增 UserPlayCountPublisher
  - `cmd/worker/main.go` — 新增用户播放量消费者
  - `handler/video.go` — SendDanmaku 校验 DanmakuClosed
  - `handler/ws/handler.go` — WebSocket 弹幕发送校验 DanmakuClosed
  - `breaker/group.go` — Group 新增 Redis 信号量和 MySQL 信号量
  - `logic/enter.go` — LogicDeps 新增信号量依赖
  - `initialize/breaker.go` — 初始化信号量

## ADDED Requirements

### Requirement: 用户主页完整数据返回
系统 SHALL 在 GET /user/home 请求中返回以下信息：
- 用户基本信息（ID、AvatarURL、Signature、Username、Address、VideoCount、Birthday、Gender、TotalLikesReceived、TotalPlayCount、Experience）
- 粉丝数（fans_count）
- 关注数（following_count）
- 当 PrivacyPublicFavorites 为 false（收藏夹公开）时，返回收藏夹列表（id、title、cover_url、is_default）
- 作者视频列表按时间倒序排列（使用 idx_author_time 复合索引直接查询数据库），返回 HomeVideoInfo 结构

#### Scenario: 正常获取用户主页
- **WHEN** 客户端请求 GET /user/home?user_id=xxx
- **THEN** 返回包含用户信息、粉丝数、关注数、视频列表的完整数据；若 PrivacyPublicFavorites 为 false 则额外返回收藏夹信息

#### Scenario: 用户收藏夹隐私保护
- **WHEN** 目标用户的 PrivacyPublicFavorites 为 true（收藏夹不公开）
- **THEN** 不返回收藏夹信息，favorite_folders 字段为空数组

### Requirement: 弹幕关闭管控
系统 SHALL 在视频的 DanmakuClosed 字段为 true 时禁止弹幕的展示和发送。

#### Scenario: 视频关闭弹幕时获取弹幕列表
- **WHEN** 客户端请求 GET /video/danmaku?video_id=xxx 且该视频 DanmakuClosed=true
- **THEN** 返回空弹幕列表

#### Scenario: 视频关闭弹幕时发送弹幕
- **WHEN** 客户端请求 POST /video/danmaku 且该视频 DanmakuClosed=true
- **THEN** 返回错误提示"该视频已关闭弹幕"

#### Scenario: WebSocket 弹幕发送时视频已关闭弹幕
- **WHEN** 用户通过 WebSocket 发送弹幕且该视频 DanmakuClosed=true
- **THEN** 不写入 MySQL、不广播，返回错误提示

### Requirement: 用户总播放量原子自增
系统 SHALL 在 GetVideoDetail 中同时原子自增用户总播放量（Redis HIncrBy + RabbitMQ 异步同步 MySQL）。

#### Scenario: 观看视频时自增用户总播放量
- **WHEN** 用户请求 GET /video/detail 且视频存在
- **THEN** 用户缓存中的 total_play_count 原子 +1，同时通过消息队列异步更新 accounts 表

### Requirement: 关注数缓存
系统 SHALL 提供关注数缓存（Redis Hash，field="count"），与粉丝数缓存模式一致，TTL 24 小时。

#### Scenario: 缓存命中
- **WHEN** 查询用户关注数且 Redis 缓存存在
- **THEN** 直接返回缓存值

#### Scenario: 缓存未命中
- **WHEN** 查询用户关注数且 Redis 缓存不存在
- **THEN** 降级查 MySQL 并回填 Redis 缓存

### Requirement: 用户播放量增量消息队列
系统 SHALL 提供独立的用户播放量增量消息队列 `user:play_count_increment`，Worker 端消费后批量更新 accounts 表的 total_play_count 字段。

#### Scenario: 消息发布
- **WHEN** GetVideoDetail 被调用
- **THEN** 发布 UserPlayCountIncrementMsg{UserID, Increment} 到 user:play_count_increment 队列

#### Scenario: 消息消费
- **WHEN** Worker 消费到用户播放量增量消息
- **THEN** 按 5 秒聚合窗口批量更新 accounts 表 total_play_count 字段

### Requirement: 熔断保护
系统 SHALL 对所有新增 MySQL 查询路径通过 Breakers.MySQL.Execute 包装，熔断 Open 时降级返回空数据。

#### Scenario: MySQL 熔断时获取用户主页
- **WHEN** MySQL 熔断器处于 Open 状态
- **THEN** 用户主页返回缓存中可获取的数据，缺失字段填零值

### Requirement: 信号量并发限制
系统 SHALL 对数据库回写和频繁查询操作添加信号量，限制并发数防止资源耗尽。

信号量容量按行业标准设定：
- MySQL 读操作信号量：30（与 GORM 默认连接池大小匹配，防止读请求压垮数据库）
- MySQL 写操作信号量：10（写操作更重，限制更严格，防止写锁竞争）
- Redis Pipeline 批量操作信号量：50（Pipeline 本身减少 RTT，但大量并发 Pipeline 仍可能阻塞 Redis 单线程）

#### Scenario: 数据库并发超限
- **WHEN** 并发请求数超过信号量容量
- **THEN** 新请求阻塞等待直到有信号量释放，或 ctx 超时返回降级结果

#### Scenario: 信号量获取超时
- **WHEN** 在 ctx 超时时间内未能获取信号量
- **THEN** 记录 Warn 日志并返回降级结果

### Requirement: Redis 操作熔断保护
系统 SHALL 对以下频繁或耗时的 Redis 操作通过 Breakers.Redis.Execute 包装：

- `VideoCacheRepo.GetVideoCache`（Pipeline 批量查询，每次视频列表/详情请求调用）
- `VideoCacheRepo.IncrementPlayCount`（HIncrBy 写操作，每次视频详情请求调用）
- `InteractionCacheRepo.GetInteractionBatch`（Pipeline 批量查询，每次视频详情请求调用）
- `UserCacheRepo.GetUserCache`（HGetAll，每次视频详情/用户主页请求调用）
- `UserCacheRepo.IncrementTotalPlayCount`（HIncrBy 写操作，每次视频详情请求调用）
- `DanmakuCacheRepo.GetDanmakuCache`（ZRevRangeWithScores，每次弹幕列表请求调用）

#### Scenario: Redis 熔断时获取视频详情
- **WHEN** Redis 熔断器处于 Open 状态
- **THEN** 视频详情走 MySQL 降级路径（已通过 MySQL 熔断器保护）

#### Scenario: Redis 熔断时获取弹幕列表
- **WHEN** Redis 熔断器处于 Open 状态且 MySQL 可用
- **THEN** 弹幕列表走 MySQL 降级路径

### Requirement: 操作超时控制
系统 SHALL 对所有新增 Redis/MySQL 操作设置合适的超时时间：
- Redis 单键操作：200ms（与 DanmakuCacheRepo 现有超时一致）
- Redis Pipeline 操作：2s（与 VideoCacheRepo 现有超时一致）
- Redis Pipeline 批量写入：3s（与 BatchWriteUserCache 现有超时一致）
- MySQL 读操作：3s（与 withTimeout 现有默认值一致）
- MySQL 写操作：5s（写操作稍长，确保数据一致性）
- 信号量获取等待超时：与操作超时一致（复用 ctx）

## MODIFIED Requirements

### Requirement: UserCacheRepo 完整性
UserCacheRepo.GetUserCache SHALL 解析 total_play_count 字段；BatchWriteUserCache SHALL 写入 total_play_count 字段。

### Requirement: PlayCountPublisher 接口扩展
新增 UserPlayCountPublisher 接口，方法为 PublishUserIncrement(ctx, userID string, increment int64) error，与现有 PlayCountPublisher 模式一致但操作用户播放量队列。

### Requirement: BreakerGroup 扩展
BreakerGroup SHALL 新增 MySQL 读信号量、MySQL 写信号量、Redis Pipeline 信号量字段，供 Logic 层在数据库操作前获取信号量。
