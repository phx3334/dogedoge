# Tasks

## Phase 1: 配置与存储抽象（基础设施）

- [x] Task 1: 扩展配置结构，新增 Transcode/Storage/FFmpeg 子配置块
  - [x] SubTask 1.1: 在 `internal/config/loadconfig.go` 中新增 `TranscodeConfig`（队列名、并发数、超时）、`StorageConfig`（driver: local/oss、OSS endpoint/bucket/access_key/secret_key、Local base_dir）、`FFmpegConfig`（binary_path、output_codec、crf、preset）
  - [x] SubTask 1.2: 在 `UploadConfig` 中新增 `TempUploadDir` 字段（默认 `./uploads/temp`）
  - [x] SubTask 1.3: 在 `configs/config.compose-local.yaml` 和 `configs/config.docker.yaml` 中添加对应配置项（本地默认 `storage.driver: local`）
  - [x] SubTask 1.4: 在 `validateConfig` 中校验 `storage.driver` 必须为 `local` 或 `oss`，`oss` 时必须配置 endpoint/bucket/access_key/secret_key

- [x] Task 2: 实现 Storage 抽象层
  - [x] SubTask 2.1: 创建 `internal/pkg/storage/storage.go`，定义 `Storage` 接口：`Put(ctx, key string, r io.Reader) (url string, err error)`、`Delete(ctx, key string) error`
  - [x] SubTask 2.2: 实现 `LocalStorage`：写入 `{base_dir}/{key}`，返回 `/uploads/{key}`（与现有 `Router.StaticFS("/uploads", ...)` 对齐）
  - [x] SubTask 2.3: 实现 `OSSStorage`：使用 `github.com/aliyun/aliyun-oss-go-sdk` 或 `aws-sdk-go-v2` S3 兼容协议上传，返回 `{bucket-endpoint}/{key}`
  - [x] SubTask 2.4: 提供 `NewStorage(cfg *config.StorageConfig) Storage` 工厂函数，按 `driver` 返回对应实现

## Phase 2: 视频草稿上传与转码

- [x] Task 3: 扩展 VideoRepository 接口与实现
  - [x] SubTask 3.1: 在 `internal/repository/interfaces/video.go` 新增 `VideoDraftRepository` 接口：`CreateDraft(ctx, *database.Video) error`、`UpdateTranscodeResult(ctx, videoID uint, videoURL, coverURL string, duration float64, status string) error`、`UpdateTranscodeFailure(ctx, videoID uint, failReason string) error`、`FindDraftByID(ctx, id uint) (*database.Video, error)`
  - [x] SubTask 3.2: 在 `internal/repository/mysql/video.go` 实现 `VideoDraftRepository`，所有方法走 `withTimeout` 与现有 `idx_video_status` 索引
  - [x] SubTask 3.3: 编译期断言 `var _ interfaces.VideoDraftRepository = (*VideoRepo)(nil)`

- [x] Task 4: 实现 TranscodePublisher
  - [x] SubTask 4.1: 在 `internal/repository/redis/keys.go` 新增 `TranscodeQueueName = "mini_bili_transcode"` 常量
  - [x] SubTask 4.2: 在 `internal/dto/cache/` 新增 `transcode_msg.go`，定义 `TranscodeMsg{VideoID uint, DraftRawPath, DraftCoverPath, UserID string}`（JSON 序列化）
  - [x] SubTask 4.3: 在 `internal/repository/rabbitmq/transcode.go` 实现 `TranscodePublisherRepo`，复用 `PublishBuffer.Publish(ctx, "", TranscodeQueueName, amqp091.Publishing{...})`，模式与 `PlayCountPublisherRepo` 完全一致
  - [x] SubTask 4.4: 在 `internal/repository/interfaces/` 新增 `TranscodePublisher` 接口，编译期断言

- [x] Task 5: 实现视频草稿上传 Logic 与 Handler
  - [x] SubTask 5.1: 在 `internal/logic/video_draft.go` 实现 `VideoDraftLogic.UploadDraft(ctx, userID string, file *multipart.FileHeader, cover *multipart.FileHeader, meta request.VideoDraftUploadReq) (videoID uint, err error)`，参考 `UserLogic.UploadAvatar` 原子写入模式（写 `.tmp` → `os.Rename`）
  - [x] SubTask 5.2: 校验文件大小（`cfg.Upload.MaxFileSize`）、扩展名白名单（mp4/mov/avi/mkv/flv），生成 `{TempUploadDir}/{userID}/{snowflakeID}{ext}` 路径
  - [x] SubTask 5.3: 插入 `Video{Status:"draft", AuthorID, DraftRawPath, DraftCoverPath(可选), Title, Description, Zone, TagsJSON}`，发布 `TranscodeMsg` 到 RabbitMQ
  - [x] SubTask 5.4: 实现 `VideoDraftLogic.GetStatus(ctx, userID, videoID) (response.VideoDraftStatusResp, error)`，校验调用者为视频作者
  - [x] SubTask 5.5: 在 `internal/dto/request/video.go` 新增 `VideoDraftUploadReq{Title, Description, Zone string, Tags []string}`，在 `internal/dto/response/video.go` 新增 `VideoDraftStatusResp{Status, FailReason, VideoURL, CoverURL string}`
  - [x] SubTask 5.6: 在 `internal/handler/video_draft.go` 实现 `VideoDraftHandler.UploadDraft`（解析 multipart `file`/`cover`/表单字段）和 `VideoDraftHandler.GetStatus`（query 参数）
  - [x] SubTask 5.7: 在 `internal/handler/enter.go` 新增 `VideoDraftHandler` 字段；在 `internal/logic/enter.go` 新增 `VideoDraftLogic` 字段

- [x] Task 6: 实现转码 Worker 消费者
  - [x] SubTask 6.1: 创建 `worker/transcode.go`，定义 `transcodeConsumeLoop(ctx, conn, db, storage, ffmpegCfg, logger)` 函数，遵循现有 `consumeLoop` 双层循环模式（外层 `WaitReady` + 重订阅，内层 `for msg := range msgs`）
  - [x] SubTask 6.2: 实现 `transcodeOne(ctx, msg, db, storage, ffmpegCfg) error`：调用 `exec.CommandContext("ffmpeg", ...)` 转码为 H.264 MP4（`-c:v libx264 -preset medium -crf 23 -c:a aac`），输出到 `{TempUploadDir}/transcoded/{videoID}.mp4`
  - [x] SubTask 6.3: 调用 `ffprobe` 或 `ffmpeg -i` 解析时长写入 `duration_sec`；若 `DraftCoverPath` 为空，调用 `ffmpeg -ss {duration/2} -i {draft} -frames:v 1 {cover}.jpg` 截帧
  - [x] SubTask 6.4: 通过 `Storage.Put(ctx, "video/{videoID}.mp4", file)` 和 `Storage.Put(ctx, "cover/{videoID}.jpg", coverFile)` 上传，得到公开 URL
  - [x] SubTask 6.5: 成功时调用 `VideoDraftRepo.UpdateTranscodeResult`，status 设为 `published`（暂不做人工审核）；失败时调用 `UpdateTranscodeFailure`，status=`failed`，fail_reason 截断 2000 字符
  - [x] SubTask 6.6: 失败时仍 Ack 消息（避免死循环重试），仅在消息体损坏时 Nack(false, false)
  - [x] SubTask 6.7: 在 `cmd/worker/main.go` 中启动 `transcodeConsumeLoop` goroutine，加入 `wg`，停机时与现有消费者一起退出

## Phase 3: 多级评论

- [x] Task 7: 实现评论 Repository 接口与 MySQL 实现
  - [x] SubTask 7.1: 创建 `internal/repository/interfaces/comment.go`，定义三个接口：
    - `VideoCommentRepository`：`Create(ctx, *database.Comment) error`、`FindByVideoID(ctx, videoID uint, page, pageSize int) ([]database.Comment, int64, error)`、`FindReplies(ctx, parentID uint64, page, pageSize int) ([]database.Comment, int64, error)`、`FindByID(ctx, id uint64) (*database.Comment, error)`、`Delete(ctx, id uint64) error`、`IncrementLikeCount(ctx, id uint64, delta int) error`、`IncrementVideoCommentCount(ctx, videoID uint, delta int) error`
    - `ArticleCommentRepository`：相同方法签名，操作 `article_comments` + `articles.comment_count`
    - `DynamicCommentRepository`：相同方法签名，操作 `dynamic_comments` + `user_dynamics.comment_count`
  - [x] SubTask 7.2: 创建 `internal/repository/mysql/comment.go`，实现三个 repo；查询走 `idx_comment_video` / `idx_article_comment_article` / `idx_dyn_cmt_dynamic` 索引，全部 `withTimeout`
  - [x] SubTask 7.3: 实现 `CommentLikeRepository`（视频/文章/动态评论点赞三套，操作 `comment_likes` / `article_comment_likes` / `dynamic_comment_likes`），提供 `CreateLike`（返回 created bool 用于幂等）、`DeleteLike`、`ExistsLike`

- [x] Task 8: 实现评论 Logic 层
  - [x] SubTask 8.1: 创建 `internal/logic/comment.go`，定义 `CommentLogic` 持有 `videoCommentRepo` / `articleCommentRepo` / `dynamicCommentRepo` + 三套 likeRepo + Breakers
  - [x] SubTask 8.2: 实现 `CreateComment(ctx, userID, req request.CreateCommentReq) (commentID uint64, err error)`：按 `target_type` 路由到对应 repo；校验目标存在且 `comments_closed=false`；若 `parent_id != 0` 查询父评论、校验 `Level < 3`、`Level = parent.Level + 1`；非精选模式 `Approved=true`；插入后调用 `IncrementVideoCommentCount`（仅 `Approved=true` 时）；发送回复通知（复用现有 `Notification` 表，类型 `reply_received`）
  - [x] SubTask 8.3: 实现 `ListComments(ctx, req request.ListCommentsReq) ([]response.CommentItem, int64, error)`：分页拉取顶级评论（Level=1，Approved=true），按 `pinned DESC, like_count DESC, created_at DESC` 排序；每条顶级评论附加 `reply_count` 和前 3 条回复
  - [x] SubTask 8.4: 实现 `ListReplies(ctx, parentID, page, pageSize)`：分页拉取指定父评论的回复
  - [x] SubTask 8.5: 实现 `LikeComment(ctx, userID, commentID, targetType)` / `UnlikeComment`：通过 `CreateLike` 返回的 `created` 判断是否真正新增，仅 created=true 时 `IncrementLikeCount +1`；通过 Breakers.MySQL.Execute 包装
  - [x] SubTask 8.6: 实现 `DeleteComment(ctx, userID, commentID, targetType)`：查询评论，校验调用者为评论作者或目标 UP 主；硬删除评论 + Decrement comment_count

- [x] Task 9: 实现评论 Handler 与路由
  - [x] SubTask 9.1: 在 `internal/dto/request/comment.go` 定义 `CreateCommentReq{TargetType string, TargetID uint64, ParentID uint64, Content string}`、`ListCommentsReq{TargetType, TargetID, Page, PageSize}`、`CommentLikeReq{TargetType, CommentID}`、`DeleteCommentReq{TargetType, CommentID}`
  - [x] SubTask 9.2: 在 `internal/dto/response/comment.go` 定义 `CommentItem{ID, User:UserCard, Content, LikeCount, CreatedAt, IpLocation, ReplyCount, Replies []CommentItem}`、`UserCard{ID, Username, AvatarURL}`
  - [x] SubTask 9.3: 创建 `internal/handler/comment.go`，实现 `CommentHandler.CreateComment` / `ListComments` / `ListReplies` / `LikeComment` / `UnlikeComment` / `DeleteComment`，复用 `pkg.GetUserID` 获取 userID
  - [x] SubTask 9.4: 创建 `internal/routers/comment.go`，注册路由：`POST /comment/create`、`GET /comment/list`、`GET /comment/replies`、`POST /comment/like`、`POST /comment/unlike`、`POST /comment/delete`（私有组，list/replies 可放公开组）
  - [x] SubTask 9.5: 在 `internal/handler/enter.go` 新增 `CommentHandler` 字段；在 `internal/logic/enter.go` 新增 `CommentLogic` 字段；在 `internal/routers/enter.go` 新增 `CommentRouter`
  - [x] SubTask 9.6: 移除 `internal/handler/interaction.go` 中 `CommentVideo` 和 `DeleteComment` 桩方法；移除 `internal/routers/interaction.go` 中对应路由

## Phase 4: 文章投稿

- [x] Task 10: 实现文章 Repository 接口与 MySQL 实现
  - [x] SubTask 10.1: 创建 `internal/repository/interfaces/article.go`，定义 `ArticleRepository` 接口：`Create(ctx, *database.Article) error`、`FindByID(ctx, id uint64) (*database.Article, error)`、`UpdateStatus(ctx, id uint64, status string) error`、`IncrementViewCount(ctx, id uint64) error`
  - [x] SubTask 10.2: 创建 `internal/repository/mysql/article.go` 实现接口，走 `idx_article_status` / `idx_article_created` 索引

- [x] Task 11: 实现文章 Logic 与 Handler
  - [x] SubTask 11.1: 创建 `internal/logic/article.go`，实现 `ArticleLogic.SaveDraft(ctx, userID, req request.ArticleDraftReq) (articleID uint64, err error)`、`PublishArticle(ctx, userID, articleID)`（校验归属，UPDATE status='published', published_at=NOW()）、`GetArticleDetail(ctx, articleID)`（返回文章 + 作者信息，UPDATE view_count +1）
  - [x] SubTask 11.2: 在 `internal/dto/request/article.go` 定义 `ArticleDraftReq{Title, BodyMD, CoverURL string, Tags []string}`
  - [x] SubTask 11.3: 在 `internal/dto/response/article.go` 定义 `ArticleDetailResp{ID, Title, CoverURL, BodyMD, Tags, ViewCount, CommentCount, CreatedAt, Author UserCard}`
  - [x] SubTask 11.4: 创建 `internal/handler/article.go`，实现 `ArticleHandler.SaveDraft` / `PublishArticle` / `GetArticleDetail`
  - [x] SubTask 11.5: 创建 `internal/routers/article.go`，注册路由：`POST /article/draft`、`POST /article/publish`（私有组）、`GET /article/detail`（公开组）
  - [x] SubTask 11.6: 在 `internal/handler/enter.go` 新增 `ArticleHandler`；在 `internal/logic/enter.go` 新增 `ArticleLogic`；在 `internal/routers/enter.go` 新增 `ArticleRouter`

## Phase 5: 依赖注入与路由注册

- [x] Task 12: 扩展 Repos 与 LogicDeps，注入新依赖
  - [x] SubTask 12.1: 在 `internal/repository/repos.go` 的 `Repos` 结构体新增 `VideoDraftRepo`、`VideoCommentRepo`、`ArticleCommentRepo`、`DynamicCommentRepo`、`VideoCommentLikeRepo`、`ArticleCommentLikeRepo`、`DynamicCommentLikeRepo`、`ArticleRepo`、`TranscodePublisher` 字段
  - [x] 12.2: 在 `NewRepos` 中构造上述所有 repo，注入 db；TranscodePublisher 复用现有 `publishBuffer`
  - [x] SubTask 12.3: 在 `internal/logic/enter.go` 的 `LogicDeps` 新增对应字段
  - [x] SubTask 12.4: 在 `internal/initialize/router.go` 的 `InitLogicGroup` 调用处注入所有新依赖；构造 `Storage` 实例后注入 `VideoDraftLogic`

- [x] Task 13: 注册新路由
  - [x] SubTask 13.1: 在 `internal/initialize/router.go` 调用 `routerGroup.InitCommentRouter(privateGroup, publicGroup, handlerGroup)`、`routerGroup.InitArticleRouter(privateGroup, publicGroup, handlerGroup)`
  - [x] SubTask 13.2: 在 `VideoRouter.InitVideoRouter` 中新增 `privateGroup.Group("video").POST("draft/upload", handlerGroup.VideoDraftHandler.UploadDraft)` 和 `GET("draft/status", handlerGroup.VideoDraftHandler.GetStatus)`

## Phase 6: 验证

- [ ] Task 14: 编译与基本验证
  - [ ] SubTask 14.1: 在 `server` 目录执行 `go build ./...` 确保编译通过
  - [ ] SubTask 14.2: 在 `server` 目录执行 `go vet ./...` 确保无 vet 错误
  - [ ] SubTask 14.3: 启动 worker 进程，确认日志输出 `transcode consume registered, waiting for messages`
  - [ ] SubTask 14.4: 通过 curl 上传一个测试视频文件，确认返回 video_id，轮询 status 接口能拿到 transcoding → published 状态变化

# Task Dependencies
- Task 2 依赖 Task 1（需要 StorageConfig）
- Task 3、Task 4、Task 7、Task 10 互相独立，可并行
- Task 5 依赖 Task 2、Task 3、Task 4
- Task 6 依赖 Task 2、Task 3、Task 4
- Task 8 依赖 Task 7
- Task 9 依赖 Task 8
- Task 11 依赖 Task 10
- Task 12 依赖 Task 3、Task 4、Task 7、Task 10
- Task 13 依赖 Task 5、Task 9、Task 11、Task 12
- Task 14 依赖 Task 13
