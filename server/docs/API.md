# 仿 B 站项目 后端 API 接口文档

> 供前端开发使用。所有接口已通过 `go build` / `go vet` 静态验证。
>
> **生成时间**：2026-07-05
> **后端版本**：Phase 5 + P1-P3 个人中心模块全部完成

---

## 一、通用约定

### 1.1 基础信息

| 项目 | 值 |
|------|------|
| Base URL | `http://<host>:<port>{RouterPrefix}` |
| RouterPrefix | 由后端配置 `server.router_prefix` 决定（示例：`/api`） |
| 默认端口 | 8080 |
| 内容类型 | `application/json`（除上传接口为 `multipart/form-data`） |
| 字符编码 | UTF-8 |

### 1.2 统一响应格式

所有接口统一返回如下 JSON 结构，HTTP 状态码恒为 200（业务错误通过 code 区分）：

```json
{
  "code": 3,
  "data": {},
  "msg": "success"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| code | int | `3`=成功，`4`=失败 |
| data | any | 业务数据，失败时为 `null` |
| msg  | string | 提示消息 |

鉴权失败时返回：
```json
{ "code": 4, "data": { "reload": true }, "msg": "请先登录" }
```

### 1.3 鉴权方式

- 需登录接口：请求头携带 `Authorization: <access_token>`
- Token 在登录/注册接口响应中获取（`access_token` 字段）
- Token 失效或缺失 → 返回鉴权失败结构（前端应跳转登录页）

### 1.4 路由分组说明

| 分组 | 鉴权 | 说明 |
|------|------|------|
| publicGroup | 无需登录 | 公开可访问 |
| privateGroup | 需 JWT | 普通登录用户接口 |
| adminGroup | 需 JWT + 管理员权限 | 后台管理接口 |

下文每个接口标注 `[公开]` / `[需登录]` / `[管理员]`。

### 1.5 分页约定

分页查询接口统一参数：
- `page`：页码，从 1 开始（默认 1）
- `page_size`：每页条数（默认 20，最大 100）

分页响应统一结构：
```json
{
  "list": [],
  "total": 100,
  "page": 1,
  "page_size": 20
}
```

---

## 二、用户模块 `/user`

### 2.1 注册

`[公开]` `POST /user/register`

**请求体**：
```json
{ "email": "user@example.com", "password": "123456", "code": "123456" }
```

**响应 data**：`{ "access_token": "...", "access_token_expire": 3600, "account": {...} }`

### 2.2 登录

`[公开]` `POST /user/login`

> 该接口有独立限流：8 次/分钟/IP

**请求体**：
```json
{ "email": "user@example.com", "password": "123456", "captcha": "abcd", "captcha_id": "xxx" }
```

**响应 data**：
```json
{
  "access_token": "eyJ...",
  "access_token_expire": 3600,
  "account": {
    "id": "1",
    "username": "用户名",
    "avatar_url": "...",
    "email": "...",
    "experience": 30,
    "coin_balance_tenths": 100
  }
}
```

### 2.3 退出登录

`[需登录]` `POST /user/logout`

### 2.4 个人信息

`[需登录]` `GET /user/info`

### 2.5 修改个人信息

`[需登录]` `PUT /user/changeInfo`

> 限流：20 次/分钟/用户

### 2.6 上传头像

`[需登录]` `POST /user/avatar`

> 限流：10 次/分钟/用户；`multipart/form-data`

### 2.7 忘记密码

`[公开]` `POST /user/forgotPassword`

> 限流：5 次/分钟/IP

### 2.8 用户主页

`[公开]` `GET /user/home?user_id=1`

### 2.9 用户主页视频列表

`[公开]` `GET /user/videos`

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| user_id | string | 是 | 用户 ID |
| page | int | 否 | 默认 1 |
| page_size | int | 否 | 默认 20，最大 100 |

**响应 data**：
```json
{
  "list": [
    { "id": 1, "title": "视频标题", "cover_url": "...", "play_count": 100, "created_at": "2026-07-01T12:00:00Z" }
  ],
  "total": 50,
  "page": 1,
  "page_size": 20
}
```

### 2.10 获取当前用户等级

`[需登录]` `GET /user/level`

**响应 data**：
```json
{
  "level": 2,
  "experience": 80,
  "next_level_exp": 200,
  "max_level_exp": 5000,
  "is_max_level": false
}
```

**等级说明**：

| 等级 | 经验上限 |
|------|---------|
| Lv1 | 50 |
| Lv2 | 200 |
| Lv3 | 500 |
| Lv4 | 1000 |
| Lv5 | 2500 |
| Lv6（满级） | 5000 |

- 每日访问 +10 经验（一天一次）
- 投币 1 个 +20 经验
- 评论 1 次 +5 经验

---

## 三、视频模块 `/video`

### 3.1 视频列表

`[公开]` `GET /video/list`

| 参数 | 类型 | 说明 |
|------|------|------|
| limit | int | 每页条数 |
| cursor | string | 游标（上一页最后一条） |
| zone | string | 可选，分区过滤 |

### 3.2 视频详情

`[公开]` `GET /video/detail?video_id=1`

**响应 data**：
```json
{
  "id": 1,
  "title": "...",
  "description": "...",
  "play_url": "...",
  "cover_url": "...",
  "duration": 120.5,
  "zone": "动画",
  "play_count": 100,
  "likes_count": 50,
  "comment_count": 10,
  "fav_count": 20,
  "coin_count": 5,
  "danmaku_count": 30,
  "comments_closed": false,
  "danmaku_closed": false,
  "created_at": "2026-07-01T12:00:00Z",
  "author": { "id": "1", "username": "...", "avatar_url": "...", "signature": "...", "fans_count": 100 },
  "interaction": {
    "is_liked": false,
    "is_favorited": false,
    "coin_count": 0,
    "is_followed": false
  }
}
```

### 3.3 视频草稿上传（分片）

`[需登录]` `POST /video/draft/upload`

> `multipart/form-data`，支持分片上传

**表单字段**：
| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| title | string | 是 | 标题，最长 255 |
| description | string | 否 | 简介，最长 512 |
| zone | string | 否 | 分区，最长 64 |
| tags | []string | 否 | 标签列表 |
| file | file | 是 | 视频文件分片 |

### 3.4 视频转码状态查询

`[需登录]` `GET /video/draft/status?video_id=1`

**响应 data**：
```json
{
  "status": "published",
  "fail_reason": "",
  "video_url": "https://...",
  "cover_url": "https://..."
}
```

**status 取值**：
- `draft`：草稿已落库，等待 worker 转码
- `transcoding`：正在转码
- `pending_review`：等待审核（暂未启用）
- `published`：转码完成可播放
- `failed`：转码失败

### 3.5 弹幕 WebSocket

`[公开]` `GET /ws/danmaku?video_id=1`

> 升级为 WebSocket 连接，用于实时弹幕推送

---

## 四、互动模块 `/interaction`

### 4.1 视频点赞

`[需登录]` `POST /interaction/video/like`

**请求体**：`{ "video_id": 1 }`

### 4.2 取消点赞

`[需登录]` `POST /interaction/video/unlike`

**请求体**：`{ "video_id": 1 }`

### 4.3 收藏视频

`[需登录]` `POST /interaction/video/favorite`

**请求体**：`{ "video_id": 1, "folder_id": 0 }`

> `folder_id=0` 表示默认收藏夹

### 4.4 取消收藏

`[需登录]` `POST /interaction/video/unfavorite`

**请求体**：`{ "video_id": 1 }`

### 4.5 发送弹幕

`[需登录]` `POST /interaction/video/danmaku`

**请求体**：
```json
{
  "video_id": 1,
  "content": "弹幕内容",
  "video_time": 12.5,
  "color": "#FFFFFF",
  "font_size": "25"
}
```

### 4.6 获取弹幕列表

`[公开]` `GET /interaction/video/danmaku?video_id=1`

### 4.7 关注用户

`[需登录]` `POST /interaction/follow`

**请求体**：`{ "followee_id": "2" }`

### 4.8 取消关注

`[需登录]` `POST /interaction/unfollow`

**请求体**：`{ "followee_id": "2" }`

---

## 五、评论模块 `/comment`

### 5.1 评论列表

`[公开]` `GET /comment/list`

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| target_type | string | 是 | `video` / `article` / `dynamic` |
| target_id | uint64 | 是 | 目标 ID |
| page | int | 否 | 默认 1 |
| page_size | int | 否 | 默认 20 |

### 5.2 回复列表

`[公开]` `GET /comment/replies`

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| target_type | string | 是 | 评论所属类型 |
| parent_id | uint64 | 是 | 父评论 ID |
| page | int | 否 | 默认 1 |
| page_size | int | 否 | 默认 20 |

### 5.3 创建评论

`[需登录]` `POST /comment/create`

**请求体**：
```json
{
  "target_type": "video",
  "target_id": 1,
  "parent_id": 0,
  "content": "评论内容"
}
```

### 5.4 点赞评论

`[需登录]` `POST /comment/like`

**请求体**：`{ "target_type": "video", "comment_id": 1 }`

### 5.5 取消点赞评论

`[需登录]` `POST /comment/unlike`

**请求体**：`{ "target_type": "video", "comment_id": 1 }`

### 5.6 删除评论

`[需登录]` `POST /comment/delete`

**请求体**：`{ "target_type": "video", "comment_id": 1 }`

---

## 六、文章模块 `/article`

### 6.1 文章详情

`[公开]` `GET /article/detail?article_id=1`

### 6.2 保存草稿

`[需登录]` `POST /article/draft`

**请求体**：`{ "title": "...", "content": "...", "tags": [] }`

### 6.3 发布文章

`[需登录]` `POST /article/publish`

**请求体**：`{ "article_id": 1, "title": "...", "content": "...", "tags": [] }`

---

## 七、搜索模块 `/search`

### 7.1 视频搜索

`[公开]` `GET /search/video`

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| keyword | string | 是 | 关键词，1-100 字符 |
| cursor | string | 否 | 游标分页 |
| limit | int | 否 | 每页条数 |

---

## 八、投币模块 `/coin`【新增】

### 8.1 视频投币

`[需登录]` `POST /coin/video`

**请求体**：
```json
{ "video_id": 1, "amount": 1 }
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| video_id | uint | 是 | 视频 ID |
| amount | int | 是 | 投币数量，必须 1 或 2 |

**业务规则**：
- 单用户对单视频最多投 2 个硬币
- 1 硬币 = 10 tenths（用户余额以 0.1 硬币为单位存储）
- 投币后扣减用户余额、视频 `coin_count` 自增、用户经验 +20/硬币
- 已投满 2 个硬币时幂等返回（`added=0`）

**响应 data**：
```json
{
  "added": 1,
  "video_coin_cnt": 5,
  "user_balance": 90
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| added | int | 本次实际新增硬币数（0 表示已投满） |
| video_coin_cnt | int64 | 视频累计收到硬币数 |
| user_balance | int64 | 用户剩余余额（0.1 硬币单位） |

### 8.2 硬币流水查询

`[需登录]` `GET /coin/ledger`

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| page | int | 否 | 默认 1 |
| page_size | int | 否 | 默认 20，最大 100 |
| reason_type | string | 否 | 过滤原因类型 |

**reason_type 取值**：
- `coin_video`：视频投币（负数，支出）
- `daily_login`：每日登录奖励（正数，收入）

**响应 data**：
```json
{
  "list": [
    {
      "id": 1,
      "delta_tenths": -10,
      "reason_type": "coin_video",
      "video_id": 1,
      "created_at": "2026-07-05T12:00:00Z"
    }
  ],
  "total": 50,
  "page": 1,
  "page_size": 20
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| delta_tenths | int64 | 变动量（正=收入，负=支出），0.1 硬币单位 |

---

## 九、收藏夹管理模块 `/favorite`【新增】

### 9.1 列出收藏夹

`[需登录]` `GET /favorite/folders`

**响应 data**：
```json
[
  {
    "id": 1,
    "title": "默认收藏夹",
    "cover_url": "",
    "is_default": true,
    "video_count": 10,
    "created_at": "2026-07-01T12:00:00Z"
  }
]
```

### 9.2 创建收藏夹

`[需登录]` `POST /favorite/folder`

**请求体**：
```json
{ "title": "我的收藏", "cover_url": "https://..." }
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| title | string | 是 | 标题，1-20 字符 |
| cover_url | string | 否 | 封面 URL |

**响应 data**：`{ "folder_id": 2 }`

### 9.3 更新收藏夹

`[需登录]` `PUT /favorite/folder`

**请求体**：
```json
{ "folder_id": 2, "title": "新标题", "cover_url": "https://..." }
```

### 9.4 删除收藏夹

`[需登录]` `DELETE /favorite/folder`

**请求体**：`{ "folder_id": 2 }`

**业务规则**：
- 不允许删除默认收藏夹（`is_default=true`）
- 删除非默认收藏夹时，其中的视频收藏记录一并清除

### 9.5 收藏夹视频列表

`[需登录]` `GET /favorite/folder/videos`

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| folder_id | uint64 | 否 | 收藏夹 ID，0 表示默认收藏夹 |
| page | int | 否 | 默认 1 |
| page_size | int | 否 | 默认 20，最大 100 |

**响应 data**：
```json
{
  "list": [
    {
      "id": 1,
      "up_name": "",
      "title": "视频标题",
      "cover_url": "...",
      "play_count": 100,
      "comment_count": 10,
      "duration": 120.5,
      "created_at": "2026-07-01T12:00:00Z",
      "fav_count": 20
    }
  ],
  "total": 10,
  "page": 1,
  "page_size": 20
}
```

### 9.6 移动收藏到指定收藏夹

`[需登录]` `POST /favorite/move`

**请求体**：
```json
{ "video_id": 1, "folder_id": 2 }
```

---

## 十、关注 / 粉丝列表模块 `/follow`【新增】

### 10.1 粉丝列表

`[公开]` `GET /follow/followers`

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| user_id | string | 是 | 用户 ID |
| page | int | 否 | 默认 1 |
| page_size | int | 否 | 默认 20，最大 100 |

**响应 data**：
```json
{
  "list": [
    {
      "id": "1",
      "username": "用户名",
      "avatar_url": "...",
      "signature": "签名"
    }
  ],
  "total": 100,
  "page": 1,
  "page_size": 20
}
```

### 10.2 关注列表

`[公开]` `GET /follow/following`

参数与响应同 10.1。

---

## 十一、通知模块 `/notification`【新增】

### 11.1 通知列表

`[需登录]` `GET /notification/list`

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| type | string | 否 | 类型过滤（如 `like` / `comment` / `reply`） |
| only_unread | bool | 否 | 仅未读，默认 false |
| page | int | 否 | 默认 1 |
| page_size | int | 否 | 默认 20，最大 100 |

**响应 data**：
```json
{
  "list": [
    {
      "id": 1,
      "type": "like",
      "related_id": "123",
      "sender_names_json": "[\"用户A\",\"用户B\"]",
      "total_likes": 5,
      "comment_preview": "评论内容预览...",
      "payload_json": "{}",
      "is_read": false,
      "created_at": "2026-07-05T12:00:00Z"
    }
  ],
  "total": 20,
  "page": 1,
  "page_size": 20
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| type | string | 通知类型：`like`/`comment`/`reply`/`follow` 等 |
| related_id | string | 关联资源 ID（评论 ID / 视频 ID 等） |
| sender_names_json | string | 触发者用户名列表（JSON 字符串） |
| total_likes | int | 该评论累计被点赞数（聚合通知） |
| comment_preview | string | 评论内容预览 |
| payload_json | string | 扩展载荷（JSON 字符串） |
| is_read | bool | 是否已读 |

### 11.2 未读数

`[需登录]` `GET /notification/unread_count`

**响应 data**：`{ "count": 5 }`

### 11.3 标记单条已读

`[需登录]` `POST /notification/read`

**请求体**：`{ "notification_id": 1 }`

> 幂等：已读时重复调用仍返回成功

### 11.4 全部已读

`[需登录]` `POST /notification/read_all`

### 11.5 静默评论点赞通知

`[需登录]` `POST /notification/mute_like`

**请求体**：`{ "comment_id": 1 }`

> 静默后，该评论后续被点赞不再产生通知

---

## 十二、历史记录模块 `/history`【新增】

### 12.1 视频观看历史

#### 12.1.1 记录观看进度

`[需登录]` `POST /history/video/view`

**请求体**：
```json
{
  "video_id": 1,
  "progress_sec": 60.5,
  "duration_sec": 120.0,
  "device": "web"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| video_id | uint | 是 | 视频 ID |
| progress_sec | float64 | 否 | 当前播放进度（秒） |
| duration_sec | float64 | 否 | 视频总时长（秒） |
| device | string | 否 | 设备类型，默认 `web` |

> 若用户在设置中暂停了观看历史记录，此接口幂等返回成功但不记录

#### 12.1.2 观看历史列表

`[需登录]` `GET /history/video/list`

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| page | int | 否 | 默认 1 |
| page_size | int | 否 | 默认 20，最大 100 |

**响应 data**：
```json
{
  "list": [
    {
      "video_id": 1,
      "progress_sec": 60.5,
      "duration_sec": 120.0,
      "device": "web",
      "viewed_at": "2026-07-05T12:00:00Z"
    }
  ],
  "total": 50,
  "page": 1,
  "page_size": 20
}
```

#### 12.1.3 删除单条观看历史

`[需登录]` `DELETE /history/video`

**请求体**：`{ "video_id": 1 }`

#### 12.1.4 清空观看历史

`[需登录]` `POST /history/video/clear`

### 12.2 文章阅读历史

#### 12.2.1 阅读历史列表

`[需登录]` `GET /history/article/list`

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| page | int | 否 | 默认 1 |
| page_size | int | 否 | 默认 20，最大 100 |

**响应 data**：
```json
{
  "list": [
    {
      "article_id": 1,
      "device": "web",
      "viewed_at": "2026-07-05T12:00:00Z"
    }
  ],
  "total": 20,
  "page": 1,
  "page_size": 20
}
```

#### 12.2.2 删除单条阅读历史

`[需登录]` `DELETE /history/article`

**请求体**：`{ "article_id": 1 }`

### 12.3 搜索历史

#### 12.3.1 保存搜索历史

`[需登录]` `POST /history/search`

**请求体**：`{ "keyword": "Vue 教程" }`

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| keyword | string | 是 | 搜索关键词，1-100 字符 |

> 同关键词（去空格+转小写后）只保留最新一条

#### 12.3.2 搜索历史列表

`[需登录]` `GET /history/search/list?limit=20`

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| limit | int | 否 | 返回条数，默认 20，最大 100 |

**响应 data**：
```json
[
  { "keyword": "Vue 教程", "updated_at": "2026-07-05T12:00:00Z" }
]
```

#### 12.3.3 删除单条搜索历史

`[需登录]` `DELETE /history/search`

**请求体**：`{ "keyword": "Vue 教程" }`

#### 12.3.4 清空搜索历史

`[需登录]` `POST /history/search/clear`

---

## 十三、用户动态模块 `/dynamic`【新增】

### 13.1 发布动态

`[需登录]` `POST /dynamic/create`

**请求体**：
```json
{
  "title": "动态标题",
  "content": "动态内容",
  "images": ["https://img1.jpg", "https://img2.jpg"]
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| title | string | 否 | 标题，最长 20 字符 |
| content | string | 否 | 内容，最长 233 字符 |
| images | []string | 否 | 图片 URL 列表，最多 9 张（超出截断） |

**响应 data**：`{ "dynamic_id": 1 }`

### 13.2 用户动态列表

`[公开]` `GET /dynamic/user`

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| user_id | string | 是 | 用户 ID |
| page | int | 否 | 默认 1 |
| page_size | int | 否 | 默认 20，最大 100 |

> 若调用方已登录，响应中 `is_liked` 字段反映当前用户是否点赞该动态；未登录时 `is_liked` 恒为 false

**响应 data**：
```json
{
  "list": [
    {
      "id": 1,
      "user_id": "1",
      "title": "动态标题",
      "content": "动态内容",
      "images_json": "[\"https://img1.jpg\"]",
      "like_count": 5,
      "comment_count": 2,
      "is_liked": false,
      "created_at": "2026-07-05T12:00:00Z"
    }
  ],
  "total": 10,
  "page": 1,
  "page_size": 20
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| images_json | string | 图片 URL 列表（JSON 字符串，前端需 `JSON.parse`） |
| is_liked | bool | 当前用户是否已点赞（需登录） |

### 13.3 关注用户动态流

`[需登录]` `GET /dynamic/feed`

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| page | int | 否 | 默认 1 |
| page_size | int | 否 | 默认 20，最大 100 |

**响应**：同 13.2，返回当前用户关注的所有用户的最新动态（按 `created_at` 倒序）

### 13.4 点赞动态

`[需登录]` `POST /dynamic/like`

**请求体**：`{ "dynamic_id": 1 }`

> 幂等：已点赞时重复调用仍返回成功，`like_count` 不重复增加

### 13.5 取消点赞动态

`[需登录]` `DELETE /dynamic/like?dynamic_id=1`

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| dynamic_id | uint64 | 是 | 动态 ID |

> 幂等：未点赞时调用仍返回成功

---

## 十四、每日任务模块 `/daily`【新增】

### 14.1 触发每日登录奖励

`[需登录]` `POST /daily/login`

**响应 data**：
```json
{ "rewarded": true }
```

| 字段 | 类型 | 说明 |
|------|------|------|
| rewarded | bool | `true`=今日首次访问已发放奖励；`false`=今日已访问过 |

**业务规则**：
- 按 Asia/Shanghai 时区切日（UTC+8）
- 当天首次调用：+10 经验、写入 `daily_login` 流水（+0.5 硬币）
- 当天再次调用：幂等返回 `rewarded=false`，不重复发放

**建议前端调用时机**：用户登录后立即调用一次

### 14.2 查询今日任务完成情况

`[需登录]` `GET /daily/today`

**响应 data**：
```json
{
  "level": {
    "level": 2,
    "experience": 80,
    "next_level_exp": 200,
    "max_level_exp": 5000,
    "is_max_level": false
  },
  "task": {
    "id": 1,
    "user_id": 1,
    "task_date": "2026-07-05",
    "login_done": true,
    "watch_done": false,
    "created_at": "2026-07-05T00:00:00Z",
    "updated_at": "2026-07-05T12:00:00Z"
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| level | UserLevelResp | 用户等级信息 |
| task.login_done | bool | 今日是否已完成登录任务 |
| task.watch_done | bool | 今日是否已完成观看任务 |

---

## 十五、基础模块 `/base`

### 15.1 获取图形验证码

`[公开]` `POST /base/captcha`

> 限流：15 次/分钟/IP

**响应 data**：
```json
{
  "captcha_id": "xxx",
  "pic_path": "data:image/png;base64,..."
}
```

### 15.2 发送邮箱验证码

`[公开]` `POST /base/sendEmainCode`

> 限流：5 次/分钟/IP

**请求体**：
```json
{ "email": "user@example.com", "captcha": "abcd", "captcha_id": "xxx" }
```

---

## 十六、管理员模块 `/user`（admin 组）

### 16.1 用户列表

`[管理员]` `GET /user/list`

### 16.2 冻结用户

`[管理员]` `PUT /user/freeze`

### 16.3 解冻用户

`[管理员]` `PUT /user/unfreeze`

### 16.4 登录记录

`[管理员]` `GET /user/loginList`

---

## 十七、健康检查

`GET /health`

> 不走任何中间件，供容器编排系统探测

**响应**：
- 健康：`{ "status": "healthy" }`
- 不健康：`{ "status": "unhealthy", "mysql": "..." }` 或 `{ "status": "unhealthy", "redis": "..." }`

---

## 十八、错误码与提示汇总

### 18.1 通用错误提示

| 场景 | 提示 |
|------|------|
| 未登录 | `未登录` |
| 参数校验失败 | `参数错误：xxx` |
| 服务繁忙（熔断/限流） | `服务繁忙，请稍后重试` |
| 资源不存在 | `xxx不存在` |
| 无权限 | `无权限` |

### 18.2 投币模块

| 场景 | 提示 |
|------|------|
| amount 非 1 或 2 | `投币数量必须为 1 或 2` |
| 投币失败 | `投币失败，请稍后重试` |
| 余额扣减失败 | `投币失败，请稍后重试` |

### 18.3 收藏夹模块

| 场景 | 提示 |
|------|------|
| 删除默认收藏夹 | `无法删除：收藏夹不存在、不属于您或为默认收藏夹` |
| 创建失败 | `创建收藏夹失败` |

### 18.4 动态模块

| 场景 | 提示 |
|------|------|
| 标题过长 | `参数错误：标题最长 20 字符，内容最长 233 字符` |

---

## 附录 A：数据结构速查

### Account（用户账户）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | string | 用户 ID |
| username | string | 用户名 |
| avatar_url | string | 头像 URL |
| signature | string | 个性签名 |
| experience | uint64 | 经验值 |
| coin_balance_tenths | int64 | 硬币余额（0.1 硬币单位，100=10 硬币） |
| video_count | int64 | 视频数 |
| fans_count | int64 | 粉丝数 |
| following_count | int64 | 关注数 |
| view_history_paused | bool | 是否暂停观看历史记录 |

### Video（视频）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 视频 ID |
| title | string | 标题 |
| cover_url | string | 封面 URL |
| play_url | string | 播放 URL |
| duration | float64 | 时长（秒） |
| play_count | int64 | 播放数 |
| likes_count | int64 | 点赞数 |
| comment_count | int64 | 评论数 |
| fav_count | uint64 | 收藏数 |
| coin_count | uint64 | 投币数 |
| danmaku_count | uint64 | 弹幕数 |

### 等级系统

| 等级 | 经验范围 | 经验上限 |
|------|---------|---------|
| Lv1 | 0-49 | 50 |
| Lv2 | 50-199 | 200 |
| Lv3 | 200-499 | 500 |
| Lv4 | 500-999 | 1000 |
| Lv5 | 1000-2499 | 2500 |
| Lv6（满级） | ≥2500 | 5000 |

经验获取方式：
- 每日访问：+10（每天一次）
- 投币：+20/硬币
- 评论：+5/次

---

## 附录 B：已完成的模块清单

| 模块 | 状态 | 路由前缀 |
|------|------|---------|
| 用户 | ✅ | `/user` |
| 视频 + 草稿上传 + 转码 | ✅ | `/video` |
| 互动（点赞/收藏/关注/弹幕） | ✅ | `/interaction` |
| 评论（多级） | ✅ | `/comment` |
| 文章 | ✅ | `/article` |
| 搜索 | ✅ | `/search` |
| 基础（验证码/邮箱） | ✅ | `/base` |
| 投币 + 流水 | ✅ 新增 | `/coin` |
| 收藏夹管理 | ✅ 新增 | `/favorite` |
| 关注/粉丝列表 | ✅ 新增 | `/follow` |
| 通知收件箱 | ✅ 新增 | `/notification` |
| 历史记录（观看/阅读/搜索） | ✅ 新增 | `/history` |
| 用户动态 | ✅ 新增 | `/dynamic` |
| 每日任务 + 等级 | ✅ 新增 | `/daily` |
| 用户主页 + 等级 | ✅ 新增 | `/user/videos`、`/user/level` |
| 健康检查 | ✅ | `/health` |
| WebSocket 弹幕 | ✅ | `/ws/danmaku` |

---

## 附录 C：已移除的功能

按需求，以下功能**已从后端移除**，前端无需对接：

- ❌ 黑名单（UserBlock）相关所有接口
- ❌ 稍后再看（WatchLater）相关所有接口
