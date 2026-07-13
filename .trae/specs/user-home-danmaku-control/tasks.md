# Tasks

- [x] Task 1: 修复 UserCacheRepo 缺失 total_play_count 字段
  - [x] SubTask 1.1: 在 `repository/redis/usercache.go` 的 `GetUserCache` 中新增 `total_play_count` 字段解析（strconv.ParseInt）
  - [x] SubTask 1.2: 在 `repository/redis/usercache.go` 的 `BatchWriteUserCache` 中新增 `total_play_count` 字段写入

- [x] Task 2: 弹幕关闭管控
  - [x] SubTask 2.1: 修改 `logic/danmaku.go` 的 `GetDanmakuList`：在方法开头通过 `getVideoDataWithFallback` 获取视频缓存数据，若 `DanmakuClosed=true` 直接返回空列表
  - [x] SubTask 2.2: 修改 `logic/danmaku.go` 的 `SendDanmaku`：新增 `videoID uint` 参数，查询视频缓存判断 `DanmakuClosed`，为 true 时返回错误 "该视频已关闭弹幕"
  - [x] SubTask 2.3: 修改 `handler/video.go` 的 `SendDanmaku`：传递 `req.VideoID` 给 `SendDanmaku`
  - [x] SubTask 2.4: 修改 `handler/ws/handler.go` 的读泵逻辑：在写入 MySQL 前查询视频缓存判断 `DanmakuClosed`，为 true 时不写入不广播

- [x] Task 3: 新增 Redis 键常量与缓存 DTO
  - [x] SubTask 3.1: 在 `repository/redis/keys.go` 新增 `UserFollowingCountHashKey = "user:following_count"` 和 `UserPlayCountQueueName = "user:play_count_increment"`
  - [x] SubTask 3.2: 在 `dto/cache/usercache.go` 新增 `UserPlayCountIncrementMsg` 结构体（UserID string, Increment int64）

- [x] Task 4: 信号量与 Redis 熔断器基础设施
  - [x] SubTask 4.1: 新增 `internal/semaphore/semaphore.go`：封装 `golang.org/x/sync/semaphore`，提供 `Acquire(ctx) error` 和 `Release()` 方法，超时随 ctx 控制
  - [x] SubTask 4.2: 修改 `breaker/group.go` 的 `Group`：新增 `MySQLReadSem *semaphore.Weighted`（容量 30）、`MySQLWriteSem *semaphore.Weighted`（容量 10）、`RedisPipelineSem *semaphore.Weighted`（容量 50）字段
  - [x] SubTask 4.3: 修改 `initialize/breaker.go` 的 `NewBreakerGroup`：初始化三个信号量
  - [x] SubTask 4.4: 修改 `logic/enter.go` 的 `LogicDeps`：Breakers 字段类型不变（Group 已扩展），无需额外字段

- [x] Task 5: 现有 Redis 操作添加熔断保护
  - [x] SubTask 5.1: 修改 `logic/video.go` 的 `getVideoDataWithFallback`：将 `VideoCacheRepo.GetVideoCache` 调用包装在 `Breakers.Redis.Execute` 中，Redis 熔断时直接走 MySQL 降级路径
  - [x] SubTask 5.2: 修改 `logic/video.go` 的 `GetVideoDetail`：将 `VideoCacheRepo.IncrementPlayCount` 调用包装在 `Breakers.Redis.Execute` 中
  - [x] SubTask 5.3: 修改 `logic/video.go` 的 `GetVideoDetail`：将 `UserCacheRepo.GetUserCache` 调用包装在 `Breakers.Redis.Execute` 中
  - [x] SubTask 5.4: 修改 `logic/video.go` 的 `getInteractionWithFallback`：将 `InteractionCacheRepo.GetInteractionBatch` 调用包装在 `Breakers.Redis.Execute` 中
  - [x] SubTask 5.5: 修改 `logic/danmaku.go` 的 `GetDanmakuList`：将 `DanmakuCacheRepo.GetDanmakuCache` 调用包装在 `Breakers.Redis.Execute` 中，Redis 熔断时走 MySQL 回源

- [x] Task 6: 现有 MySQL 操作添加信号量保护
  - [x] SubTask 6.1: 修改 `logic/video.go`：所有 `Breakers.MySQL.Execute` 内的读操作前获取 `Breakers.MySQLReadSem`，defer Release
  - [x] SubTask 6.2: 修改 `logic/danmaku.go` 的 `SendDanmaku`：MySQL 写操作前获取 `Breakers.MySQLWriteSem`，defer Release
  - [x] SubTask 6.3: 修改 `logic/video.go` 的 `redisPath`：Pipeline 批量查询前获取 `Breakers.RedisPipelineSem`，defer Release

- [x] Task 7: 关注数缓存接口与实现
  - [x] SubTask 7.1: 在 `repository/interfaces/interaction_cache.go` 新增 `GetFollowingCount(ctx, userID) (int64, error)` 和 `SetFollowingCount(ctx, userID, count) error` 方法
  - [x] SubTask 7.2: 在 `repository/redis/interaction_cache.go` 实现关注数缓存：Hash key=`user:following_count:{userID}`，field="count"，TTL=InteractionCacheExpire

- [x] Task 8: 关注数 MySQL 查询
  - [x] SubTask 8.1: 在 `repository/interfaces/interaction.go` 新增 `GetFollowingCount(ctx, userID) (int64, error)` 方法
  - [x] SubTask 8.2: 在 `repository/mysql/interaction.go` 实现 `GetFollowingCount`：`SELECT COUNT(*) FROM user_follows WHERE follower_id = ?`

- [x] Task 9: 收藏夹查询接口与实现
  - [x] SubTask 9.1: 在 `repository/interfaces/` 新增 `favorite_folder.go`，定义 `FavoriteFolderRepository` 接口：`FindByUserID(ctx, userID) ([]database.FavoriteFolder, error)`
  - [x] SubTask 9.2: 在 `repository/mysql/` 新增 `favorite_folder.go`，实现 `FavoriteFolderRepo`：`SELECT id, user_id, title, cover_url, is_default FROM favorite_folders WHERE user_id = ?`，设置 3s 超时
  - [x] SubTask 9.3: 在 `repository/repos.go` 注册 `FavoriteFolderRepo`

- [x] Task 10: 作者视频按时间排序查询
  - [x] SubTask 10.1: 在 `repository/interfaces/video.go` 新增 `FindPublishedVideosByAuthorID(ctx, authorID, limit, offset) ([]database.Video, error)` 方法
  - [x] SubTask 10.2: 在 `repository/mysql/video.go` 实现：使用 `idx_author_time` 复合索引，`WHERE author_id = ? AND status = 'published' ORDER BY created_at DESC LIMIT ? OFFSET ?`，设置 3s 超时

- [x] Task 11: 用户播放量增量消息队列
  - [x] SubTask 11.1: 在 `repository/interfaces/play_count.go` 新增 `UserPlayCountPublisher` 接口：`PublishUserIncrement(ctx, userID string, increment int64) error`
  - [x] SubTask 11.2: 在 `repository/rabbitmq/` 新增 `user_playcount.go`，实现 `UserPlayCountPublisherRepo`：序列化 `UserPlayCountIncrementMsg` 发布到 `user:play_count_increment` 队列，复用 `PublishBuffer`
  - [x] SubTask 11.3: 在 `repository/repos.go` 注册 `UserPlayCountPublisher`

- [x] Task 12: 用户缓存播放量自增接口
  - [x] SubTask 12.1: 在 `repository/interfaces/user_cache.go` 新增 `IncrementTotalPlayCount(ctx, userID) error` 方法
  - [x] SubTask 12.2: 在 `repository/redis/usercache.go` 实现 `IncrementTotalPlayCount`：对 user:static:{userID} Hash 的 total_play_count 字段执行 HIncrBy +1，设置 200ms 超时

- [x] Task 13: 扩展响应 DTO
  - [x] SubTask 13.1: 在 `dto/response/user.go` 新增 `UserHomeResp` 结构体（ID、AvatarURL、Signature、Username、Address、VideoCount、Birthday、Gender、TotalLikesReceived、TotalPlayCount、Experience、FansCount、FollowingCount、FavoriteFolders、Videos）
  - [x] SubTask 13.2: 在 `dto/response/user.go` 新增 `FavoriteFolderInfo` 结构体（ID uint64、Title string、CoverURL string、IsDefault bool）

- [x] Task 14: BackfillRepo 扩展
  - [x] SubTask 14.1: 在 `repository/interfaces/backfill.go` 新增 `BackfillFollowingCountCache(ctx, userID) (int64, error)` 方法
  - [x] SubTask 14.2: 在 `repository/mysql/backfill.go` 实现 `BackfillFollowingCountCache`：查 MySQL 获取关注数并回填 Redis 缓存

- [x] Task 15: LogicDeps 依赖注入扩展
  - [x] SubTask 15.1: 在 `logic/enter.go` 的 `LogicDeps` 新增 `FavoriteFolderRepo`、`UserPlayCountPublisher` 字段
  - [x] SubTask 15.2: 在 `initialize/router.go` 注入新依赖到 `LogicDeps`

- [x] Task 16: GetVideoDetail 新增用户总播放量自增
  - [x] SubTask 16.1: 修改 `logic/video.go` 的 `GetVideoDetail`：在视频播放量自增之后，新增用户总播放量 Redis 自增（`UserCacheRepo.IncrementTotalPlayCount`，通过 `Breakers.Redis.Execute` 包装）和 RabbitMQ 发布（`UserPlayCountPublisher.PublishUserIncrement`）

- [x] Task 17: 实现用户主页 Logic
  - [x] SubTask 17.1: 修改 `logic/user.go` 的 `UserCard` 方法：
    - 获取用户缓存（UserCacheRepo.GetUserCache，通过 Breakers.Redis.Execute 包装），未命中走熔断 MySQL 回源（BackfillUserCache，获取 MySQLReadSem）
    - 并行获取粉丝数（InteractionCacheRepo.GetFansCount，通过 Breakers.Redis.Execute 包装，未命中走 BackfillFansCountCache，获取 MySQLReadSem）和关注数（InteractionCacheRepo.GetFollowingCount，通过 Breakers.Redis.Execute 包装，未命中走 BackfillFollowingCountCache，获取 MySQLReadSem），使用 goroutine + buffered channel + ctx 超时
    - 若 PrivacyPublicFavorites 为 false，查询收藏夹（FavoriteFolderRepo.FindByUserID，通过 Breakers.MySQL.Execute 包装，获取 MySQLReadSem）
    - 查询作者视频列表（VideoRepo.FindPublishedVideosByAuthorID，通过 Breakers.MySQL.Execute 包装，获取 MySQLReadSem），转换为 []HomeVideoInfo
    - 组装 UserHomeResp 返回

- [x] Task 18: Worker 端新增用户播放量消费者
  - [x] SubTask 18.1: 修改 `cmd/worker/main.go`：新增 `userPending map[string]int64` 和对应 `userFlush` 函数，批量更新 accounts 表的 total_play_count，5s 超时
  - [x] SubTask 18.2: 新增 `userConsumeLoop` 函数：消费 `user:play_count_increment` 队列，解析 `UserPlayCountIncrementMsg`，累加到 userPending
  - [x] SubTask 18.3: 新增 userFlush 协程：5 秒定时批量更新 accounts 表 `total_play_count = total_play_count + ?`

- [x] Task 19: 编译验证
  - [x] SubTask 19.1: 执行 `go build ./...` 确保编译通过
  - [x] SubTask 19.2: 修复所有编译错误

# Task Dependencies
- [Task 2] depends on [Task 1] (弹幕管控需要读取视频缓存数据)
- [Task 4] depends on [Task 3] (信号量基础设施需要先有 Redis 键常量)
- [Task 5] depends on [Task 4] (Redis 熔断保护依赖信号量基础设施)
- [Task 6] depends on [Task 4] (MySQL 信号量保护依赖信号量基础设施)
- [Task 7] depends on [Task 3] (关注数缓存需要 Redis 键常量)
- [Task 8] depends on [Task 3] (关注数查询需要接口定义)
- [Task 11] depends on [Task 3] (消息队列需要 DTO 和队列名常量)
- [Task 12] depends on [Task 1] (用户播放量自增依赖 UserCacheRepo 修复)
- [Task 14] depends on [Task 7, Task 8] (BackfillRepo 扩展依赖缓存和 MySQL 接口)
- [Task 15] depends on [Task 9, Task 11] (依赖注入需要新 repo)
- [Task 16] depends on [Task 11, Task 12, Task 15] (GetVideoDetail 扩展依赖新接口和注入)
- [Task 17] depends on [Task 7, Task 8, Task 9, Task 10, Task 13, Task 14, Task 15] (用户主页依赖所有新接口)
- [Task 18] depends on [Task 3, Task 11] (Worker 依赖消息 DTO 和队列名)
- [Task 19] depends on [Task 5, Task 6, Task 16, Task 17, Task 18]
