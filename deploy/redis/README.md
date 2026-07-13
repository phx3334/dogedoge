# Redis 混合持久化部署说明

> 路径: `deploy/redis/redis.conf` + `deploy/redis/README.md`
> 配套规范: `.trae/specs/production-hardening/spec.md` (Task 1)

## 1. 混合持久化原理

Redis 7 引入了 **AOF + RDB 混合持久化**（`aof-use-rdb-preamble yes`），在 AOF Rewrite 触发后，
生成的文件不再是纯文本 AOF，而是 **RDB 头 + 增量 AOF 尾** 的二进制 + 文本混合结构。

```
┌──────────────────────────────────────────────────────────┐
│                   appendonly.aof                         │
│  ┌──────────────────────┐  ┌──────────────────────────┐   │
│  │  RDB 快照段           │  │  增量 AOF 段              │   │
│  │  ────────────────    │  │  ─────────────────       │   │
│  │  二进制 RDB 格式       │  │  RESP 文本命令序列        │   │
│  │  rewrite 时刻全量数据  │  │  rewrite 之后的所有写     │   │
│  │  加载速度 ≈ RDB       │  │  恢复精度 ≈ 纯 AOF       │   │
│  └──────────────────────┘  └──────────────────────────┘   │
└──────────────────────────────────────────────────────────┘
```

### 1.1 加载流程

当 Redis 重启并启用 AOF 时：
1. 识别文件头是 RDB 格式（`REDIS` magic + 版本号）
2. **以 RDB 方式快速加载** 头部快照（毫秒级恢复 2GB 数据集）
3. **以 AOF 方式重放** 尾部增量（恢复 rewrite 之间的写操作）

### 1.2 与纯 RDB / 纯 AOF 对比

| 模式 | 启动速度 | 数据安全 | 体积 | 写开销 |
|------|----------|----------|------|--------|
| 纯 RDB | ⚡ 极快（mmap + fork） | 差（分钟级丢失） | 小（压缩） | 低 |
| 纯 AOF everysec | 🐢 慢（文本 replay） | 较好（秒级丢失） | 大（持续增长） | 中 |
| **混合模式**（本配置） | ⚡ 接近 RDB | **较好**（秒级丢失） | 中（rewrite 控制） | 中 |

## 2. `save` 三档参数业务含义

```conf
save 900 1        # 15 分钟内 ≥ 1 次写 → 触发
save 300 10       # 5 分钟内 ≥ 10 次写 → 触发
save 60 10000     # 1 分钟内 ≥ 10 000 次写 → 触发
```

### 业务覆盖场景

- **`900 1`（兜底层）**
  业务含义：低活跃期保证"至多丢失 15 分钟数据"。即便半夜无写，
  也会通过周期性的 cron / 后台任务触发一次 bgsave。

- **`300 10`（中等活跃层）**
  业务含义：日间正常业务（写 ZSet 增量、更新缓存、点赞等）下，5 分钟级别的数据窗口。

- **`60 10000`（高峰层）**
  业务含义：流量高峰、worker 重建 ZSet、批量导入等场景。1 分钟内密集写时
  触发快照，缩短数据窗口。

## 3. AOF Rewrite 触发条件

两个条件 **同时满足** 时触发后台 BGREWRITEAOF：

```
触发 = (aof_size > last_rewrite_size × (1 + auto-aof-rewrite-percentage / 100))
   AND (aof_size > auto-aof-rewrite-min-size)
```

| 条件 | 本配置 | 业务含义 |
|------|--------|----------|
| 百分比 | `100` | AOF 体积比上次 rewrite 后**翻倍** |
| 最小体积 | `64mb` | AOF 文件至少 64MB 才考虑（避免小文件频繁 rewrite） |

### 计算示例

假设 rewrite 后 AOF = 50MB：
- 触发阈值 = 50 × 2 = 100MB
- 但小于 64MB？否，100MB > 64MB → **触发**

假设 rewrite 后 AOF = 30MB：
- 触发阈值 = 30 × 2 = 60MB
- 60MB > 64MB？否 → **不触发**

## 4. 为什么选择 `allkeys-lru`

本项目 Redis 主要用途：

| 数据类型 | 重建成本 | 是否可丢失 |
|----------|----------|------------|
| 用户静态缓存 | 低（DB 现拉） | ✅ 可丢失 |
| 视频静态缓存 | 低（DB 现拉） | ✅ 可丢失 |
| 热度 ZSet | 中（worker 重建） | ✅ 可丢失 |
| 验证码 | 自动 TTL | ✅ 可丢失 |
| 限流计数 | 自动 TTL | ✅ 可丢失 |

**所有数据均可重建或带 TTL**，因此 `allkeys-lru` 是性价比最高的选择：
- 写满时自动淘汰最久未访问的键
- 热点数据（活跃用户、热门视频）天然保留
- 冷数据被淘汰时，下次访问自动从 DB 回源

**其他候选策略不适合的原因**：
- `noeviction` 写满后 OOM 拒绝写入 → 业务降级严重
- `volatile-lru` 仅淘汰带 TTL 的键 → 可能保留大量冷数据

## 5. 容量规划公式

### 5.1 数据量

```
key 总数 N（预估）
平均 value 大小 S（KB）
总内存 = N × S × 1.5  (含 Redis 自身开销、碎片、AOF buffer)
```

例：50 万个用户缓存 × 2KB × 1.5 = 1.5GB → 本配置 2GB 上限够用。

### 5.2 QPS

```
最大 QPS ≈ 100 000（单实例 Redis 极限）
本项目预估峰值 = 5 000（留 20× 余量）
```

### 5.3 内存

```
总内存 = 数据 × 1.5 + 客户端缓冲 + AOF/RDB 临时空间
     ≈ 2GB × 1.5 + 256MB(replica) + 64MB(AOF rewrite) ≈ 3.5GB
```

建议为容器分配 **3-4GB** 的物理内存上限（docker-compose `mem_limit`）。

## 6. 误用风险

### 6.1 Rewrite 期间 IO 抖动

BGREWRITEAOF 后台 fork 子进程，**主进程阻塞 100ms-1s**（取决于数据集大小）。
- 现象：API 请求 P99 突刺
- 缓解：`no-appendfsync-on-rewrite no` 让主进程在 rewrite 期间继续 fsync，
  缩短 fork 阻塞窗口
- 监控：观察 `INFO stats` 中 `latest_fork_usec`

### 6.2 磁盘写满

- AOF 文件持续写入，磁盘满后 Redis **拒绝所有写操作**
- 监控：磁盘使用率告警阈值建议 70%
- 缓解：定时清理 AOF 历史文件 + 监控 `aof_current_size`

### 6.3 maxmemory 误配置

- 内存上限设置过小 → 频繁 LRU 淘汰 → 缓存命中率下降
- 建议：**预留 30% 余量**给 Redis 自身开销和临时分配

### 6.4 密码泄露

- `requirepass` 写在配置文件里，git 提交前请脱敏
- 生产环境通过环境变量注入（`REDIS_PASSWORD`），启动时覆盖默认值

## 7. 与 docker-compose 的对接

### 7.1 当前配置

```yaml
redis:
  image: redis:7-alpine
  ports:
    - "6380:6379"          # 宿主机:容器
  volumes:
    - redis_data:/data                    # 持久化数据卷
    - ./deploy/redis/redis.conf:/usr/local/etc/redis/redis.conf:ro  # 配置文件
  command: ["redis-server", "/usr/local/etc/redis/redis.conf"]
```

### 7.2 端口

| 角色 | 端口 |
|------|------|
| 宿主机 → 容器映射 | 6380 → 6379 |
| 容器内部通信 | 6379 |

### 7.3 卷挂载

| 容器路径 | 宿主机路径 | 用途 |
|----------|------------|------|
| `/usr/local/etc/redis/redis.conf` | `./deploy/redis/redis.conf` (ro) | 配置文件（只读） |
| `/data` | `redis_data` 卷 | RDB + AOF 文件持久化 |

### 7.4 密码传递

- `REDIS_PASSWORD` 环境变量同时驱动：
  1. `redis.conf` 中的 `requirepass`（容器启动读取）
  2. server 端 `config.docker.yaml` 中 `redis.password`
  3. `redis-cli` 健康检查 `-a` 参数
- 三个位置的密码**必须一致**，由 docker-compose 环境变量统一注入

### 7.5 健康检查

```yaml
healthcheck:
  test: ["CMD-SHELL", "redis-cli -a \"$${REDIS_PASSWORD:-123456}\" ping"]
  interval: 5s
  timeout: 5s
  retries: 20
```

启动后验证：
```bash
docker exec -it <redis_container> redis-cli -a 123456 CONFIG GET appendonly
# 期望返回: 1) "appendonly"  2) "yes"

docker exec -it <redis_container> redis-cli -a 123456 CONFIG GET save
# 期望返回: 1) "save"  2) "900 1 300 10 60 10000"
```

## 8. 启动时应用侧校验

API / Worker 进程启动时，`server/internal/initialize/redis.go` 中的
`verifyPersistence` 会自动调用 `CONFIG GET appendonly` / `CONFIG GET save` 校验：

- **AOF 已启用 + RDB 已配置** → 正常启动
- **AOF 未启用 或 RDB 全空** → 输出 `Warn` 日志，**不阻断启动**
  - 提示运维侧确认挂载了 `redis.conf`

应用侧不做 `CONFIG SET` 强制修改，**所有持久化配置由运维侧保证**，
避免运行期修改持久化策略带来的副作用。

## 9. 变更历史

| 版本 | 日期 | 变更 |
|------|------|------|
| 1.0 | 2026-06-04 | 初始混合持久化配置 (Task 1) |
