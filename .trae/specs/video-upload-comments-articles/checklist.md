# Checklist

## 配置与存储抽象
- [x] `TranscodeConfig` / `StorageConfig` / `FFmpegConfig` 三个子配置块在 `loadconfig.go` 中定义
- [x] `UploadConfig.TempUploadDir` 字段已添加，默认 `./uploads/temp`
- [x] `configs/config.compose-local.yaml` 和 `configs/config.docker.yaml` 包含新配置项
- [x] `validateConfig` 校验 `storage.driver` 必须为 `local` 或 `oss`
- [x] `Storage` 接口在 `internal/pkg/storage/storage.go` 中定义
- [x] `LocalStorage` 实现 `Storage` 接口，写入后返回 `/uploads/{key}`
- [x] `OSSStorage` 实现 `Storage` 接口，上传后返回 OSS 公开 URL
- [x] `NewStorage` 工厂函数按 driver 返回对应实现

## 视频草稿上传
- [x] `VideoDraftRepository` 接口在 `interfaces/video.go` 中定义，含 `CreateDraft` / `UpdateTranscodeResult` / `UpdateTranscodeFailure` / `FindDraftByID`
- [x] `VideoRepo` 实现了 `VideoDraftRepository` 接口（编译期断言通过）
- [x] `TranscodeQueueName = "mini_bili_transcode"` 常量已添加到 `redis/keys.go`
- [x] `TranscodeMsg` DTO 已定义
- [x] `TranscodePublisherRepo` 复用 `PublishBuffer`，模式与 `PlayCountPublisherRepo` 一致
- [x] `TranscodePublisher` 接口在 `interfaces/` 中定义
- [x] `VideoDraftLogic.UploadDraft` 校验文件大小和扩展名
- [x] 上传文件采用「写 .tmp → os.Rename」原子写入模式
- [x] 上传成功后插入 `Video{Status:"draft", DraftRawPath, DraftCoverPath, ...}` 记录
- [x] 发布 `TranscodeMsg` 到 `mini_bili_transcode` 队列
- [x] `VideoDraftHandler.UploadDraft` 能正确解析 multipart `file` / `cover` / 表单字段
- [x] `VideoDraftLogic.GetStatus` 校验调用者为视频作者后返回状态
- [x] `GET /video/draft/status` 返回 `VideoDraftStatusResp{Status, FailReason, VideoURL, CoverURL}`
- [x] 未登录用户访问 `POST /video/draft/upload` 被 JWTAuth 中间件拦截返回 401
- [x] 文件超过 `max_file_size` 返回业务错误
- [x] 扩展名不在白名单返回业务错误

## 视频转码 Worker
- [x] `worker/transcode.go` 中 `transcodeConsumeLoop` 遵循现有双层循环模式（外层 `WaitReady` + 重订阅）
- [x] 消费者声明 `mini_bili_transcode` 队列（durable=true）
- [x] `Qos(10, 0, false)` 限制未 ack 消息数量
- [x] ffmpeg 命令使用 `-c:v libx264 -preset medium -crf 23 -c:a aac` 转码
- [x] 成功解析视频时长写入 `duration_sec`
- [x] 用户未传封面时通过 `ffmpeg -ss {mid} -i {draft} -frames:v 1 {cover}.jpg` 截帧
- [x] 转码后视频和封面通过 `Storage.Put` 上传得到公开 URL
- [x] 成功时 `UpdateTranscodeResult` 设置 `video_url` / `cover_url` / `duration_sec` / `status='published'`
- [x] 失败时 `UpdateTranscodeFailure` 设置 `status='failed'` + `fail_reason`（截断 2000 字符）
- [x] 转码失败仍 Ack 消息避免死循环
- [x] 消息体损坏时 `Nack(false, false)`
- [x] `cmd/worker/main.go` 启动 transcode 消费者 goroutine 并加入 `wg`
- [x] 收到 SIGTERM 时 transcode 消费者与现有消费者一起退出

## 多级评论
- [x] `VideoCommentRepository` / `ArticleCommentRepository` / `DynamicCommentRepository` 三个接口在 `interfaces/comment.go` 中定义
- [x] 三个 repo 在 `mysql/comment.go` 中实现，所有方法走 `withTimeout`
- [x] `CommentLikeRepository`（三套）提供 `CreateLike`（返回 created bool）/ `DeleteLike` / `ExistsLike`
- [x] `CommentLogic.CreateComment` 按 `target_type` 路由到对应 repo
- [x] `parent_id != 0` 时校验父评论存在、`Level < 3`、`Level = parent.Level + 1`
- [x] `parent_id != 0` 且父评论 `Level >= 3` 返回「超过最大回复层级」错误
- [x] 目标 `comments_closed=true` 时返回「评论区已关闭」
- [x] 精选模式下新评论 `Approved=false`，不增加 `comments_count`
- [x] 非精选模式下新评论 `Approved=true`，`comments_count +1`
- [x] 回复时发送 `reply_received` 通知到 `Notification` 表
- [x] `ListComments` 返回顶级评论 + 每条前 3 条回复 + `reply_count`
- [x] `ListComments` 排序为 `pinned DESC, like_count DESC, created_at DESC`
- [x] `ListComments` 仅返回 `Approved=true`
- [x] `ListReplies` 按 `created_at ASC` 排序
- [x] `LikeComment` 通过 `CreateLike` 返回的 `created` 判断是否真正新增，仅 created=true 时 +1
- [x] `UnlikeComment` 幂等
- [x] `DeleteComment` 校验调用者为评论作者或目标 UP 主
- [x] `DeleteComment` 硬删除评论 + Decrement `comments_count`
- [x] 评论相关操作通过 `Breakers.MySQL.Execute` 包装
- [x] `CommentHandler` 实现了 6 个方法：CreateComment / ListComments / ListReplies / LikeComment / UnlikeComment / DeleteComment
- [x] 路由 `/comment/create`、`/comment/list`、`/comment/replies`、`/comment/like`、`/comment/unlike`、`/comment/delete` 已注册
- [x] `list` / `replies` 在公开组，其他在私有组
- [x] `InteractionHandler.CommentVideo` 和 `DeleteComment` 桩方法已移除
- [x] `interaction.go` 路由中 `/interaction/video/comment` 和 `/interaction/video/delete` 已移除

## 文章投稿
- [x] `ArticleRepository` 接口在 `interfaces/article.go` 中定义
- [x] `ArticleRepo` 在 `mysql/article.go` 中实现
- [x] `ArticleLogic.SaveDraft` 插入 `Article{Status:"draft", ...}` 返回 `article_id`
- [x] `ArticleLogic.PublishArticle` 校验文章归属后 UPDATE `status='published', published_at=NOW()`
- [x] 非作者尝试发布返回「无权限」
- [x] `ArticleLogic.GetArticleDetail` 返回文章 + 作者信息
- [x] `GetArticleDetail` 查询时 `view_count +1`
- [x] 查询未发布文章（status != published）且非作者返回「文章不存在」
- [x] `ArticleHandler` 实现了 SaveDraft / PublishArticle / GetArticleDetail
- [x] 路由 `POST /article/draft`、`POST /article/publish`（私有组）、`GET /article/detail`（公开组）已注册

## 依赖注入与编译
- [x] `Repos` 结构体新增 9 个字段
- [x] `NewRepos` 构造并注入所有新 repo
- [x] `LogicDeps` 新增对应字段
- [x] `InitRouter` 中构造 `Storage` 实例并注入 `VideoDraftLogic`
- [x] `InitRouter` 调用 `InitCommentRouter` 和 `InitArticleRouter`
- [x] `VideoRouter` 注册 draft 路由
- [x] `HandlerGroup` 新增 `VideoDraftHandler` / `CommentHandler` / `ArticleHandler` 字段
- [x] `LogicGroup` 新增 `VideoDraftLogic` / `CommentLogic` / `ArticleLogic` 字段
- [x] `RouterGroup` 新增 `CommentRouter` / `ArticleRouter`
- [x] `cd server && go build ./...` 通过
- [x] `cd server && go vet ./...` 无错误
- [x] 启动 worker，日志输出 `transcode consume registered, waiting for messages`
- [ ] curl 上传测试视频，能拿到 video_id，轮询 status 接口能观察到 transcoding → published 状态变化
