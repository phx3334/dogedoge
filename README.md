# Fake TikTok

一个仿抖音（TikTok）风格的短视频社区后端 + 前端全栈项目。包含用户系统、视频投稿与转码、关注/粉丝、点赞评论、搜索、消息通知以及 AI 助手聊天等完整功能。

## 技术栈

**后端（Go）**
- 框架：[Gin](https://github.com/gin-gonic/gin)
- 数据库：MySQL 8（GORM）
- 缓存：Redis 7（混合持久化 RDB + AOF）
- 消息队列：RabbitMQ 3（视频转码异步任务）
- 搜索：Elasticsearch 8
- 转码：ffmpeg（worker 进程消费转码任务）
- 鉴权：JWT（Access + Refresh 双 Token）
- 配置：YAML + 环境变量覆盖

**前端（Vue 3）**
- 框架：Vue 3 + TypeScript + Vite
- 状态管理：Pinia
- 路由：Vue Router
- 样式：Tailwind CSS
- 播放器：ArtPlayer + 弹幕插件
- HTTP：Axios

**部署**
- 容器化：Docker / Docker Compose
- 镜像仓库：GitHub Container Registry（GHCR）
- CI/CD：GitHub Actions（push 到 `main` 自动构建并部署到云服务器）

## 目录结构

```
fake_tiktok/
├── server/                 # Go 后端
│   ├── cmd/
│   │   ├── api/           # API 服务入口
│   │   └── worker/        # 转码 worker 入口（消费 RabbitMQ + 定时任务）
│   ├── configs/           # 配置（config.docker.yaml）
│   ├── internal/          # 业务代码（handler / logic / repository / initialize）
│   ├── seed/              # 容器内同步到 uploads 卷的静态种子（默认头像等）
│   └── docker-entrypoint.sh
├── web/                   # Vue 3 前端
│   ├── src/               # 源码（api / views / components / composables）
│   ├── public/images/     # 静态资源（含 AI 角色头像）
│   ├── Dockerfile
│   └── nginx.conf
├── deploy/
│   └── redis/             # Redis 混合持久化配置 + 说明
├── Dockerfile             # 多阶段构建 api + worker 镜像
├── docker-compose.yml     # 生产编排（MySQL/Redis/RabbitMQ/ES/api/worker/web）
└── .github/workflows/    # CI/CD 流水线
```

## 快速开始

> 本地开发通常需要 MySQL、Redis、RabbitMQ、Elasticsearch 等基础设施。
> 最简方式是直接拉起 docker-compose（见「生产部署」），或仅用容器提供依赖、本地跑前后端便于调试。

### 1. 后端（本地）

前置：Go 1.26+、本地/容器化的 MySQL 与 Redis。

```bash
cd server

# 准备配置：复制示例并按需修改（或直接使用 configs/config.docker.yaml）
export CONFIG_NAME=config.docker   # 加载 configs/config.<name>.yaml

# 启动 API
go run ./cmd/api

# 另开终端启动转码 worker（需宿主机有 ffmpeg）
go run ./cmd/worker
```

API 默认监听 `:8080`，路由前缀 `/api/v1`，健康检查端点 `/health`。

> 配置项通过 `configs/config.docker.yaml` 提供默认值，且支持 `APP_*` 环境变量覆盖（容器部署时使用）。关键变量见下方「环境变量」。

### 2. 前端（本地）

前置：Node 20+。

```bash
cd web
npm install
npm run dev          # 开发服务器（Vite，默认 5173）
# 或
npm run build        # 类型检查 + 生产构建到 dist/
npm run preview      # 预览构建产物
```

前端通过 `baseURL: '/api/v1'` 访问后端，本地开发时由 Vite proxy（见 `web/vite.config`）或 nginx 反代转发到 API。

## 环境变量

后端所有配置均可在 `config.docker.yaml` 中设置默认值，并被同名 `APP_*` 环境变量覆盖。常用变量：

| 变量 | 说明 | 默认 |
|------|------|------|
| `APP_SERVER_PORT` | API 监听端口 | `8080` |
| `APP_DATABASE_HOST/PORT/USER/PASSWORD/DBNAME` | MySQL 连接 | `mysql/3306/root/feedsystem` |
| `APP_REDIS_HOST/PORT/PASSWORD` | Redis 连接 | `redis/6379` |
| `APP_RABBITMQ_HOST/PORT/USERNAME/PASSWORD` | RabbitMQ 连接 | `rabbitmq/5672/admin` |
| `APP_ELASTICSEARCH_HOST` | ES 地址 | `http://elasticsearch:9200` |
| `APP_JWT_ACCESS/REFRESH_TOKEN_SECRET` | JWT 签名密钥 | 空（生产必填） |
| `APP_EMAIL_HOST/SECRET` | 邮箱验证码 SMTP | QQ 邮箱 |
| `APP_AI_API_KEY/BASE_URL/MODEL` | AI 助手大模型 | DeepSeek |
| `APP_UPLOAD_MAX_FILE_SIZE` | 上传文件大小上限（字节） | `1073741824`（1GB） |
| `APP_TRANSCODE_TIMEOUT` | 转码超时（秒） | `1800` |
| `APP_STORAGE_DRIVER` | 存储驱动（`local` / `qiniu`） | `local` |
| `APP_SKIP_CAPTCHA` | 是否跳过图形验证码（容器部署置 `1`） | `0` |
| `APP_RUN_CRON` | worker 是否启用定时任务 | `false` |

> 详细字段见 `server/configs/config.docker.yaml`，每条均有注释。

## 核心功能

- **用户系统**：注册/登录、JWT 鉴权、邮箱验证码、头像上传、个人主页（粉丝/关注/视频数实时统计）。
- **视频投稿**：大文件分片/表单上传，落库后投递 RabbitMQ 转码任务，worker 调用 ffmpeg 转码 + 自动截帧生成封面，上传后前端轮询状态。上传链路超时统一为 **1 小时**（HTTP 读/写超时、handler context、前端 axios 三者一致）。
- **互动**：点赞、评论、关注/取关（维护 Redis 计数缓存）、收藏夹。
- **社交**：关注流、消息通知、AI 助手聊天（预置「老八」「波奇」等角色，对接 OpenAI 兼容接口，支持流式回复）。
- **搜索**：基于 Elasticsearch 的视频/用户检索。

## 生产部署

### 使用 Docker Compose

1. 在服务器上准备部署目录与 `.env`（不被 git 追踪，集中存放所有密钥与密码）：

   ```bash
   mkdir -p /opt/fake_tiktok
   # 将 docker-compose.yml 与 deploy/ 拷贝到 /opt/fake_tiktok/
   # 创建 /opt/fake_tiktok/.env，至少包含：
   #   GHCR_OWNER=<github 用户名>
   #   MYSQL_ROOT_PASSWORD / MYSQL_DATABASE
   #   REDIS_PASSWORD
   #   RABBITMQ_DEFAULT_USER / RABBITMQ_DEFAULT_PASSWORD
   #   APP_JWT_ACCESS_TOKEN_SECRET / APP_JWT_REFRESH_TOKEN_SECRET
   #   APP_AI_API_KEY
   # 注意：redis 密码需与 deploy/redis/redis.conf 中的 requirepass 保持一致
   ```

2. 拉起全部服务：

   ```bash
   cd /opt/fake_tiktok
   docker compose up -d
   ```

   服务清单：`mysql`、`redis`、`rabbitmq`、`elasticsearch`、`api`（:8080）、`worker`、`web`（:80，nginx 托管静态并反代 `/api/v1`）。

### CI/CD（GitHub Actions）

Push 到 `main` 分支会自动触发 `.github/workflows/deploy.yml`：

1. **build-and-push**：构建 `api` / `worker`（来自根 `Dockerfile` 多阶段 target）与 `web`（来自 `web/Dockerfile`）三个镜像并推送到 GHCR。
2. **deploy**：通过 SSH 将 `docker-compose.yml` 与 `deploy/` 拷到服务器，登录 GHCR 后 `docker compose pull && up -d`。

所需仓库 Secrets：`SERVER_HOST`、`SERVER_USER`、`SERVER_PASSWORD`、`GHCR_PAT`，以及 GITHUB_TOKEN（自动提供）。

## 常用说明

- **AI 角色头像**：放在 `web/public/images/`（及构建产物 `web/dist/images/`），由后端 `server/internal/logic/ai.go` 中的 `AICharacter` 配置路径指向。新增/修改角色需同步更新后端路径与实际图片文件。
- **Redis 持久化**：采用 RDB + AOF 混合模式，详见 `deploy/redis/README.md`。
- **静态资源**：默认头像等种子在 api 容器启动时由 `docker-entrypoint.sh` 从镜像内 `uploads-seed` 同步到 `uploads` 卷（不覆盖用户已上传文件）。

## 许可证

本项目仅用于学习与交流。
