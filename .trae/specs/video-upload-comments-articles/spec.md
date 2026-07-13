# 视频上传/转码、文章投稿与多级评论 Spec

## Why
项目当前缺少三类核心 UGC 后端能力：
1. **视频投稿闭环**：`Video` 表已存在 `Status`/`DraftRawPath`/`DraftCoverPath`/`FailReason` 字段，但缺少接收上传、触发转码、异步产出可播放 URL 的完整链路；现有 `rabbitmq.Connection` + `PublishBuffer` 弹性连接基础设施可复用，但尚无 transcode 队列和消费者。
2. **多级评论**：`Comment` / `ArticleComment` / `DynamicComment` 三张表已通过 `ParentID` + `Level` 字段定义了多级评论结构，但 `InteractionHandler.CommentVideo` / `DeleteComment` 仍是 `"not implemented"` 桩，文章与动态评论完全无 API。
3. **专栏文章投稿**：`Article` 表已存在 `Status`/`BodyMD`/`Title`/`CoverURL` 等字段，但无创建/发布/查询接口。

本 Spec 聚焦后端实现，遵循现有「Handler → Logic → Repository（interfaces + mysql/redis/rabbitmq）」分层与已有熔断 / 信号量 / singleflight / 原子写入模式。

## What Changes
- **新增** `internal/config/loadconfig.go` 中 `TranscodeConfig`、`StorageConfig`、`FFmpegConfig` 三个子配置块；`UploadConfig` 新增 `TempUploadDir` 字段
- **新增** `internal/pkg/storage/` 包：定义 `Storage` 接口 + `LocalStorage` + `OSSStorage`（按配置选择实现，本地开发用 LocalStorage，生产用 OSS）
- **新增** `internal/handler/video_draft.go`：`POST /video/draft/upload`（multipart 接收 + 元数据）+ `GET /video/draft/status`
- **新增** `internal/logic/video_draft.go`：原子写入临时目录、插入 `status=draft` 记录、发布转码消息
- **新增** `internal/repository/rabbitmq/transcode.go`：`TranscodePublisher`（走现有 `PublishBuffer`，队列名 `mini_bili_transcode`）
- **新增** `internal/repository/redis/keys.go` 中 `TranscodeQueueName = "mini_bili_transcode"` 常量
- **新增** `worker/transcode.go`：消费者（ffmpeg 转码 H.264 MP4 → 可选 ffmpeg 截帧生成封面 → 上传 Storage → 更新 DB `video_url`/`cover_url`/`status`/`fail_reason`）
- **扩展** `internal/repository/interfaces/video.go`：新增 `VideoDraftRepository` 接口（CreateDraft / UpdateTranscodeResult / FindDraftByID）
- **扩展** `internal/repository/mysql/video.go`：实现上述接口
- **扩展** `internal/repository/interfaces/comment.go`（新文件）：定义 `CommentRepository` / `ArticleCommentRepository` / `DynamicCommentRepository` 三套接口（CRUD + 多级查询 + 点赞）
- **新增** `internal/repository/mysql/comment.go`：三套接口的 MySQL 实现
- **新增** `internal/handler/comment.go`：视频/文章/动态评论的统一 handler（按 `target_type` 区分）
- **新增** `internal/logic/comment.go`：多级评论业务逻辑（校验 `parent_id`、`Level` 自动 +1、最多 3 级、计数同步、通知）
- **新增** `internal/handler/article.go`：`POST /article/draft`、`POST /article/publish`、`GET /article/detail`
- **新增** `internal/logic/article.go`：文章投稿逻辑（草稿/发布/详情）
- **扩展** `internal/repository/interfaces/article.go`（新文件）+ `internal/repository/mysql/article.go`（新文件）
- **扩展** `internal/routers/video.go`：注册 draft 路由（私有组）
- **新增** `internal/routers/comment.go` + `internal/routers/article.go`：注册路由
- **扩展** `internal/initialize/router.go`：注入新依赖（Storage / VideoDraftRepo / CommentRepo / ArticleRepo / TranscodePublisher）
- **扩展** `internal/logic/enter.go` + `internal/repository/repos.go` + `internal/handler/enter.go`：新增字段
- **扩展** `cmd/worker/main.go`：启动 transcode 消费者 goroutine
- **扩展** `configs/config.compose-local.yaml` + `configs/config.docker.yaml`：新增配置项
- **BREAKING**：`InteractionRouter` 中现有的 `POST /interaction/video/comment` 和 `POST /interaction/video/delete` 桩方法将被移除，由新的 `/comment/*` 路由替代

## Impact
- Affected specs:
  - `implement-video-detail`：视频详情响应中的 `comment_count` 字段将由评论模块维护
  - `production-hardening`：复用 `rabbitmq.Connection` + `PublishBuffer` + 优雅停机机制；新增 transcode 消费者纳入 worker 进程
  - `user-home-danmaku-control`：用户主页 `total_likes_received` 计数包含评论点赞
- Affected code:
  - `internal/config/loadconfig.go` — 新增配置结构
  - `internal/pkg/storage/` — 新增存储抽象
  - `internal/handler/video_draft.go` / `comment.go` / `article.go` — 新增
  - `internal/logic/video_draft.go` / `comment.go` / `article.go` — 新增
  - `internal/repository/interfaces/video.go` — 扩展
  - `internal/repository/interfaces/comment.go` / `article.go` — 新增
  - `internal/repository/mysql/video.go` — 扩展
  - `internal/repository/mysql/comment.go` / `article.go` — 新增
  - `internal/repository/rabbitmq/transcode.go` — 新增
  - `internal/repository/redis/keys.go` — 新增队列常量
  - `internal/repository/repos.go` — 新增字段
  - `internal/logic/enter.go` — 新增 LogicDeps 字段
  - `internal/handler/enter.go` — 新增 handler 字段
  - `internal/routers/video.go` — 注册 draft 路由
  - `internal/routers/comment.go` / `article.go` — 新增
  - `internal/routers/interaction.go` — 移除评论桩路由
  - `internal/initialize/router.go` — 注入新依赖
  - `cmd/worker/main.go` — 启动 transcode 消费者
  - `configs/config.compose-local.yaml` / `configs/config.docker.yaml` — 新增配置

## ADDED Requirements

### Requirement: 视频草稿上传
系统 SHALL 提供 `POST /video/draft/upload` 接口（私有路由，需登录），接收 multipart/form-data 上传视频文件 + 可选封面 + 元数据（title、description、zone、tags），保存到本地临时目录，插入一条 `status=draft` 的 `Video` 记录，发布转码消息到 RabbitMQ `mini_bili_transcode` 队列，返回 `video_id` 供前端轮询。

#### Scenario: 已登录用户上传视频
- **WHEN** 已登录用户（Role=User 或 Admin）通过 multipart 上传视频文件（含 title/description/zone/tags）到 `POST /video/draft/upload`
- **THEN** 服务端校验文件大小（≤ `upload.max_file_size`，默认 1GB）和扩展名（mp4/mov/avi/mkv/flv），原子写入 `{TempUploadDir}/{userID}/{uuid}.tmp` 后 rename 为最终文件，插入 `Video{Status:"draft", AuthorID:userID, DraftRawPath:path, DraftCoverPath:userCoverPath, Title, Description, Zone, TagsJSON}`，通过 `TranscodePublisher.Publish` 发送 `TranscodeMsg{VideoID, DraftRawPath, DraftCoverPath, UserID}` 到 `mini_bili_transcode` 队列，返回 `{video_id: 42}`，HTTP 200

#### Scenario: 未登录用户上传视频
- **WHEN** 未登录用户（Role=Guest 或无 JWT）请求 `POST /video/draft/upload`
- **THEN** JWTAuth 中间件返回 401，业务逻辑不执行

#### Scenario: 文件过大
- **WHEN** 上传文件大小超过 `upload.max_file_size`
- **THEN** 返回业务错误码「文件超过大小限制」，HTTP 200（项目惯例业务码错误也走 200）

#### Scenario: 不支持的扩展名
- **WHEN** 上传文件扩展名不在白名单
- **THEN** 返回业务错误码「不支持的视频格式」

### Requirement: 视频转码状态查询
系统 SHALL 提供 `GET /video/draft/status?video_id=` 接口，返回当前视频的 `status`（draft/transcoding/pending_review/published/failed）和 `fail_reason`。

#### Scenario: 转码进行中
- **WHEN** 前端轮询 `GET /video/draft/status?video_id=42`，此时 worker 正在转码
- **THEN** 返回 `{status:"transcoding", fail_reason:""}`，HTTP 200

#### Scenario: 转码失败
- **WHEN** 转码失败后前端轮询
- **THEN** 返回 `{status:"failed", fail_reason:"ffmpeg exit code 1: ..."}`，HTTP 200

#### Scenario: 转码完成
- **WHEN** 转码完成且未配置人工审核时
- **THEN** 返回 `{status:"published", fail_reason:""}`，HTTP 200

### Requirement: 视频异步转码消费者
Worker 进程 SHALL 消费 `mini_bili_transcode` 队列，对每条消息执行：ffmpeg 转码为 H.264 MP4 → 若 `DraftCoverPath` 为空则 ffmpeg 截帧生成 JPG 封面 → 上传视频和封面到 Storage（LocalStorage 或 OSS）→ 更新 DB `video_url` / `cover_url` / `duration_sec` / `status`。失败时设置 `status=failed` + `fail_reason`。

#### Scenario: 正常转码流程
- **WHEN** Worker 消费到 `TranscodeMsg{VideoID:42, DraftRawPath:"./temp/xxx.mp4"}`
- **THEN** 调用 `ffmpeg -i {draft} -c:v libx264 -preset medium -crf 23 -c:a aac {output}.mp4` 转码，调用 `ffprobe` 获取时长，若 `DraftCoverPath` 为空则 `ffmpeg -ss {mid} -i {draft} -frames:v 1 {cover}.jpg` 截帧，通过 `Storage.Put` 上传两个文件得到公开 URL，UPDATE videos SET video_url=?, cover_url=?, duration_sec=?, status='published' WHERE id=42，Ack 消息

#### Scenario: 转码失败
- **WHEN** ffmpeg 返回非零退出码
- **THEN** UPDATE videos SET status='failed', fail_reason='ffmpeg exit code N: {stderr_tail_2000_chars}' WHERE id=42，Ack 消息（不重试，避免死循环）

#### Scenario: Worker 重启后恢复消费
- **WHEN** Worker 进程在转码过程中崩溃，重启后
- **THEN** 通过 `Connection.WaitReady` + 自动重订阅恢复消费；status=draft 但未 ack 的消息由 broker 重新投递

### Requirement: 存储抽象层
系统 SHALL 提供统一的 `Storage` 接口，支持 `LocalStorage`（开发默认）和 `OSSStorage`（生产）两种实现，按配置自动选择。

#### Scenario: 配置 LocalStorage
- **WHEN** `storage.driver = "local"`（默认）
- **THEN** `Storage.Put(ctx, key, reader)` 将文件写入 `{upload.path}/{key}`，返回 `{key}` 作为访问 URL（前端通过 `/uploads/{key}` 静态服务访问）

#### Scenario: 配置 OSS
- **WHEN** `storage.driver = "oss"` 且配置了 endpoint/bucket/access_key/secret_key
- **THEN** `Storage.Put` 通过 S3 兼容协议上传到 OSS，返回 `{bucket-endpoint}/{key}` 公开 URL

### Requirement: 多级评论创建
系统 SHALL 提供 `POST /comment/create` 接口（私有路由），支持对视频/文章/动态发表评论或回复。通过 `target_type`（video/article/dynamic）+ `target_id` 定位目标，`parent_id` 可选（0 表示顶级评论）。最多支持 3 级评论：顶级 (Level=1) → 回复 (Level=2) → 回复的回复 (Level=3)。

#### Scenario: 发表顶级评论
- **WHEN** 已登录用户 POST `/comment/create`，body 包含 `target_type:"video"`, `target_id:42`, `parent_id:0`, `content:"好视频"`
- **THEN** 校验目标存在且 `comments_closed=false`，插入 `Comment{VideoID:42, UserID, ParentID:0, Level:1, Content, Approved:true}`（非精选模式默认 approved），UPDATE videos SET comments_count = comments_count + 1，返回 `{comment_id: 100}`，HTTP 200

#### Scenario: 回复他人评论
- **WHEN** 已登录用户 POST `/comment/create`，body 包含 `parent_id:100`
- **THEN** 查询父评论，若父评论 `Level >= 3` 则返回错误「超过最大回复层级」，否则插入 `Comment{..., ParentID:100, Level: parent.Level+1, ...}`，UPDATE videos SET comments_count = comments_count + 1

#### Scenario: 评论区已关闭
- **WHEN** 目标视频 `comments_closed=true`
- **THEN** 返回业务错误「评论区已关闭」

#### Scenario: 评论精选模式
- **WHEN** 目标视频 `comments_curated=true`
- **THEN** 新评论 `Approved=false`，不增加 `comments_count`，等待 UP 主精选后才公开

### Requirement: 多级评论查询
系统 SHALL 提供 `GET /comment/list` 接口（公开路由），返回指定目标的评论列表（分页）。响应包含顶级评论 + 每条顶级评论的前 N 条回复（默认 3 条），回复支持「查看更多回复」二次拉取。

#### Scenario: 拉取视频评论列表
- **WHEN** 客户端 GET `/comment/list?target_type=video&target_id=42&page=1&page_size=20`
- **THEN** 返回 `[{id, user:{id,name,avatar}, content, like_count, created_at, ip_location, reply_count, replies:[前3条回复]}]`，按 `pinned DESC, like_count DESC, created_at DESC` 排序，仅返回 `Approved=true` 的评论，HTTP 200

#### Scenario: 拉取单条评论的回复
- **WHEN** 客户端 GET `/comment/replies?comment_id=100&page=1&page_size=20`
- **THEN** 返回该评论的所有子回复（Level=2），按 `created_at ASC` 排序

### Requirement: 评论点赞
系统 SHALL 提供 `POST /comment/like` 和 `POST /comment/unlike` 接口（私有路由），记录用户对评论的点赞。

#### Scenario: 首次点赞
- **WHEN** 已登录用户 POST `/comment/like`，body 包含 `comment_id:100` + `target_type:"video"`
- **THEN** 插入 `CommentLike{UserID, CommentID:100}`，UPDATE comments SET like_count = like_count + 1，返回成功

#### Scenario: 重复点赞（幂等）
- **WHEN** 用户对已点赞的评论再次 POST `/comment/like`
- **THEN** 不插入重复记录，不重复 +1，返回成功

### Requirement: 评论删除
系统 SHALL 提供 `POST /comment/delete` 接口（私有路由），仅允许评论作者或视频/文章 UP 主删除。

#### Scenario: 作者删除自己评论
- **WHEN** 评论作者 POST `/comment/delete`，body 包含 `comment_id:100`
- **THEN** 软删除或硬删除评论，UPDATE comments_count -1，返回成功

#### Scenario: 非作者删除他人评论
- **WHEN** 普通用户尝试删除他人评论，且不是目标 UP 主
- **THEN** 返回业务错误「无权限删除」

### Requirement: 文章投稿
系统 SHALL 提供文章草稿与发布接口。`POST /article/draft` 保存草稿（status=draft），`POST /article/publish` 发布（status=pending_review 或 published）。

#### Scenario: 保存草稿
- **WHEN** 已登录用户 POST `/article/draft`，body 包含 `title`、`body_md`、`cover_url`、`tags`
- **THEN** 插入 `Article{UserID, Title, BodyMD, CoverURL, TagsJSON, Status:"draft"}`，返回 `{article_id}`，HTTP 200

#### Scenario: 发布文章
- **WHEN** 已登录用户 POST `/article/publish`，body 包含 `article_id`
- **THEN** 校验文章归属，UPDATE articles SET status='published', published_at=NOW() WHERE id=?，返回成功

#### Scenario: 非作者尝试发布
- **WHEN** 用户尝试发布非自己的文章
- **THEN** 返回业务错误「无权限」

### Requirement: 文章详情查询
系统 SHALL 提供 `GET /article/detail?article_id=` 接口（公开路由），返回文章完整内容 + 作者信息。

#### Scenario: 查询已发布文章
- **WHEN** 客户端 GET `/article/detail?article_id=42`
- **THEN** 返回文章基本信息（title、cover_url、body_md、tags、view_count、comment_count、created_at）+ 作者信息（id、username、avatar），UPDATE articles SET view_count = view_count + 1，HTTP 200

#### Scenario: 查询未发布文章
- **WHEN** 客户端查询 status != published 的文章且非作者本人
- **THEN** 返回业务错误「文章不存在」

## MODIFIED Requirements

### Requirement: InteractionRouter 评论路由
`InteractionRouter` SHALL 移除 `POST /interaction/video/comment` 和 `POST /interaction/video/delete` 两个桩路由，由统一的 `/comment/*` 路由组替代。**BREAKING**：路由路径变化，前端需迁移。

### Requirement: VideoRepository 扩展
`VideoRepository` 接口 SHALL 新增 `CreateDraft`、`UpdateTranscodeResult`、`FindDraftByID` 三个方法用于草稿上传与转码结果更新。

### Requirement: Worker 进程职责
Worker 进程 SHALL 在现有播放量/用户播放量/点赞数消费者基础上，新增 transcode 消费者 goroutine。transcode 消费者复用 `rabbitmq.Connection` 弹性连接，遵循现有「外层 WaitReady + 重订阅 / 内层消息处理」双层循环模式。

## REMOVED Requirements

### Requirement: InteractionHandler.CommentVideo / DeleteComment 桩方法
**Reason**: 这些桩方法返回 `"not implemented"`，实际功能由新的 `CommentHandler` 接管，支持视频/文章/动态三类目标。
**Migration**: 移除 `interaction.go` 中的 `CommentVideo` / `DeleteComment` 方法及对应路由；前端调用方迁移到 `/comment/*` 新路由。
