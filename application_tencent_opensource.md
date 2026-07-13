# 腾讯开源课题实战 · 项目申请书

> 申请人：________  
> 申请日期：2026-07-07  
> 期望参与项目：________（请填写具体课题，例如 Gin / Vue 3 / WebSocket / Elasticsearch 等方向）

---

## 一、时间规划

> "腾讯开源课题实战"为期约 5 周（8 月 1 日 ~ 9 月 10 日），组委会将在 8 月初、8 月底联系项目导师获取实战情况与评价。建议以"周"为单位规划如下内容。

### 第 1 周（8.1 - 8.7）｜熟悉项目 & 跑通本地环境
- 阅读项目 README、CONTRIBUTING、架构文档与核心模块源码。
- 复现 issue 环境，搭建本地开发环境（Docker Compose / 依赖安装 / 调试配置）。
- 认领第一个 `good first issue` 或导师指派任务，完成首次 PR 流程跑通。

### 第 2 周（8.8 - 8.14）｜深入核心模块 + 完成 1 个中等 issue
- 深入项目目标模块（如 WebSocket Hub / 缓存层 / 路由中间件 / 弹幕调度等）。
- 完成 1 个中等难度 issue 的功能开发 + 单元测试 + 自测用例。
- 整理学习笔记与代码 review 记录，向导师提交周报。

### 第 3 周（8.15 - 8.21）｜承担独立 feature 或性能优化
- 独立设计 1 个 feature / 优化点（如：熔断器信号量调整、并发安全加固、API 性能优化等）。
- 输出 RFC / 设计说明，提交 PR 并跟进 review 意见。
- 配合导师进行 8 月底的实战情况评估。

### 第 4 周（8.22 - 8.28）｜完成第二个 issue + 补充测试
- 完成第二个 issue（含必要单测、E2E 测试）。
- 修复 review 中暴露的细节问题，确保 CI 全绿。
- 完善使用文档 / 注释 / 演示 Demo。

### 第 5 周（8.29 - 9.10）｜结项交付 & 社区贡献沉淀
- 合并所有 PR，整理 commit log 与 issue 闭环。
- 撰写实战总结博客（技术方案 + 踩坑记录 + 后续规划）。
- 将改进点同步到社区群组 / 论坛，完成最终交付。

---

### 期望获得的成长 / 收益

1. **深入工业级开源项目的协作流程**：完整体验 `issue → fork → PR → review → merge` 的开源协作闭环，理解大型项目在 CI / 测试 / 文档 / 兼容性方面的工程化要求。
2. **提升对所选技术栈的源码级理解**：从"会用 API"上升到"理解设计取舍"，能够独立定位和修复较深层的 bug。
3. **锻炼远程协作与异步沟通能力**：与项目导师、Mentor、Maintainer 进行英文 / 中文技术沟通，提交规范的 commit message 与 PR description。
4. **产出可写进简历的开源贡献记录**：获得 2 个以上被合入的 PR、1 篇技术博客，建立个人 GitHub 主页的持续输出。
5. **结识同辈优秀开发者**：与同期入围同学交流、互相 review，结识社区 Maintainer，为后续内推 / 合作打基础。

### 期望熟悉的技术 / 模块

> 请根据所申请项目自行填写，以下为示例：

- **后端方向**：Gin 框架中间件链路 / WebSocket 房间化管理 / Redis 缓存设计与一致性 / RabbitMQ 消息可靠性投递 / Elasticsearch 查询优化 / 熔断与限流落地。
- **前端方向**：Vue 3 响应式原理 / Pinia 状态管理 / Vite 构建优化 / 组件库封装 / TypeScript 高级类型 / 视频 / 弹幕播放内核。
- **通用工程**：Docker 容器化、CI/CD（GitHub Actions）、压测与性能 profiling、日志与可观测性、单元测试与 mock 框架。

---

## 二、开源经历

> 此模块为了让项目导师对每一位申请学生 & 开发者的技术实践能力有更清晰的认识。

### 本次自己参与的 issue 链接和心得体会

> 如已有贡献请填写，本项目内可填写仿 B 站仓库（[示例](https://github.com/yourname/fake_tiktok)）中的 issue / PR。

- 暂无外部开源 issue 参与记录，但具备强烈的开源贡献意愿，已为本次实战做好时间与精力准备。
- 期望在本次课题中完成 2~3 个 issue 闭环，作为首次正式的开源贡献起点。

### 曾经参与其他开源项目，如研发、运营等贡献

- 暂无外部社区 PR 经验，但长期使用并关注以下开源项目：Gin、Vue 3、Element Plus、ArtPlayer、gorm/redis/rabbitmq/amqp091-go 等，并在自学中通过阅读源码（如 `gorilla/websocket` 的 hub.go）做笔记、画时序图。

### 虽然未参与开源贡项目，但是具备的上手能力或开发经验

> 以下为我独立完成的"仿 B 站"全栈项目，可作为上手能力佐证：

**项目名称**：仿 B 站视频社区平台（Fake Bilibili）  
**项目地址**：https://github.com/yourname/fake_tiktok  
**项目简介**：独立设计并实现一个含投稿、实时弹幕、多级评论、动态广场、个人中心等核心场景的视频社区 Web 应用，对标 B 站 UI。

**技术栈**：Go 1.21 + Gin + GORM + MySQL + Redis + RabbitMQ + Elasticsearch + WebSocket；Vue 3 + TypeScript + Vite + Pinia + Tailwind CSS + ArtPlayer。

**具备的工程能力**：

1. **后端架构能力**：基于 Gin + GORM 分层架构（Handler / Logic / Repository），实现 10+ 业务模块、100+ RESTful 接口；Viper 多环境配置 + Zap 结构化日志 + Lumberjack 日志切割。
2. **高可用设计能力**：
   - 自研 **Redis / MySQL 双熔断器**，信号量与 `MaxOpenConns` 对齐（70% 读 + 30% 写），配合令牌桶限流防止雪崩。
   - 视频 / 用户 / 互动数据走 **Cache-Aside** 模式，热点数据 Redis 一级缓存 + 异步回源 MySQL。
   - 视频转码走 **RabbitMQ** 异步消费，API / Worker 进程解耦，失败重试 + 死信队列。
3. **并发编程能力**：基于 `gorilla/websocket` 实现弹幕房间化管理（`map[videoID]map[*Client]bool` + `sync.RWMutex`），采用"读锁拷贝快照 → 释放锁 → 无锁遍历"模式避免并发 map 写入 panic，支撑 1000+ 并发弹幕连接。
4. **搜索与存储能力**：Elasticsearch 8 集成 IK 分词实现视频全文检索；分片上传 + MD5 秒传 + 转码状态轮询。
5. **安全工程化能力**：JWT 双 Token（Access + Refresh，Refresh 存 Redis 拉黑）、bcrypt 密码哈希、邮箱验证码（Redis HSET + EXPIRE）、`base64Captcha` 图形验证码；Docker Compose 一键编排，uploads bind mount 持久化。
6. **调试与 Bug 修复能力**：独立排查并修复 11 个前端 `res.list` 空值崩溃 bug（通过 `|| []` 双重保护）、后端 JWT 类型断言 500 错误、并发 map panic 等线上问题。

**项目数据**：1.2w+ 行 Go、8k+ 行 TS / Vue，Git 提交 200+ 次，独立完成架构设计 / 开发 / 部署 / 联调全流程。

### 若以上经验都没有涉及，需要着重体现自己对项目使用技术栈的掌握程度

- 已系统阅读过 Gin、Vue 3、Redis、RabbitMQ、Elasticsearch 等核心组件源码或官方文档，能独立写 demo 验证。
- 熟悉 Git 协作流程：`git rebase` / `git cherry-pick` / 解决冲突 / 写规范的 commit message（Conventional Commits）。
- 熟练使用 Docker / Docker Compose 进行本地开发与部署，了解 GitHub Actions / GitLab CI 的基本配置。
- 具备良好的英文文档阅读能力，能直接消化 RFC / SPEC 文档（如 WebSocket RFC 6455、JWT RFC 7519）。

---

## 三、其他资料

### 学生 & 开发者个人简历

- **简历文件**：见附件（PDF）。
- **简历在线链接**：https://yourname.github.io/resume/
- **GitHub**：https://github.com/yourname
- **个人博客 / 技术笔记**：https://blog.yourname.com（可选）

> 简历中将重点突出本次仿 B 站项目的**后端架构能力**、**高可用设计**、**并发编程**与**Bug 定位**经验。

### 其他资料

1. **项目演示**：
   - 在线 Demo：http://your-demo.com（可选）
   - 演示视频：见附件或链接
   - 截图合集：见附件
2. **技术博客**（可选）：
   - 《自研双熔断器设计：信号量与连接池的协同》
   - 《WebSocket 房间化管理：读锁拷贝快照方案》
   - 《前后端 Token 自动刷新：从一次 500 错误说起》
3. **个人开源项目**：
   - 仿 B 站视频社区：https://github.com/yourname/fake_tiktok
   - 其他 demo：见 GitHub 主页
4. **参与意愿与社区贡献承诺**：
   - 愿意在课题期间保证每周 ≥ 20 小时的投入；
   - 课题结束后仍会以 Contributor 身份继续参与 issue 修复与 review；
   - 计划将实战成果整理为博客 / 校园技术分享，反哺社区。

---

## 申请感言（可选）

> 腾讯开源课题实战是我从"自学者"走向"开源贡献者"的关键一步。我已经在仿 B 站项目中独立完成了从架构设计、编码实现、容器部署到线上调优的全流程，但真正进入工业级开源项目、参与 Mentor 的 review、与社区 Maintainer 协作，仍是我非常期待的成长机会。  
>   
> 我相信扎实的后端基础、对 Gin / Vue 3 / WebSocket / 缓存体系的理解，以及对"用户可观测、可维护、可演进"工程文化的认同，能让我在课题中快速贡献价值。也希望借此机会结识更多优秀的同学与社区前辈，在开源的世界里持续走下去。  
>   
> 感谢评审老师与项目导师的审阅，期待入围！

---

**申请人签字**：________________  **日期**：________________
