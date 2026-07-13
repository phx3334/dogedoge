package redis

import (
	"math/rand"
	"time"
)

// 本文件定义 Redis 中使用的键名常量和过期时间。
// 键名分为两类：
//   - 模块键（如 VideoStaticHashKey = "video:static"）：配合 BuildKey(module, key) 使用
//   - 完整键模板（如 VideoGlobalPopularityKey = "video:global:popularity"）：直接作为 Redis key

const (
	// ---- 全局视频热榜 ----

	// VideoGlobalPopularityKey 全局热度榜，ZSet 成员格式 {video_id}，score=热度值
	VideoGlobalPopularityKey = "video:global:popularity"
	// VideoGlobalTimeKey 全局时间榜，ZSet 成员格式 {video_id}，score=发布 Unix 时间戳
	VideoGlobalTimeKey = "video:global:time"

	// ---- 作者视频热榜 ----

	// VideoAuthorPopularityKey 作者热度榜，需 fmt.Sprintf 填入 author_id
	// 格式示例: "video:author:uuid-xxx:popularity"
	VideoAuthorPopularityKey = "video:author:%s:popularity"
	// VideoAuthorTimeKey 作者时间榜，需 fmt.Sprintf 填入 author_id
	VideoAuthorTimeKey = "video:author:%s:time"

	// ---- 视频缓存 Hash（使用 BuildKey 构建完整键名） ----
	//
	// 这些是模块键，实际 Redis key 格式为 {KeyPrefix}:{module}:{video_id}
	// 例如: Dogedoge:v1:video:static:42
	//
	// 以下三类 Hash 由定时任务和请求回源时填充：

	// VideoStaticHashKey 视频静态信息 Hash 的 module 部分
	// Hash field: play_url, cover_url, duration, author_id, title, description, comments_closed, comments_curated, danmaku_closed, created_at
	VideoStaticHashKey = "video:static"

	// VideoDynamicHashKey 视频动态计数 Hash 的 module 部分
	// Hash field: play_count, comment_count, likes_count, fav_count, coin_count, danmaku_count
	VideoDynamicHashKey = "video:dynamic"

	// VideoEmptyHashKey 空对象 Hash 的 module 部分，防止缓存穿透
	// Hash field: empty（值为 "1"）
	VideoEmptyHashKey = "video:empty"

	// ---- 发布视频 ZSet（游标分页用） ----

	// PublishedVideoZSetKey 全局发布视频 ZSet 的 module 部分
	// 成员为 video_id 字符串，score 为热度分（由定时任务计算）
	PublishedVideoZSetKey = "video:published"

	// PublishedVideoZSetMaxSize ZSet 存储上限个数，超过此数量裁剪低分成员
	PublishedVideoZSetMaxSize = 1000

	// ---- 用户缓存 ----

	// UserStaticHashKey 用户静态信息 Hash 的 module 部分
	UserStaticHashKey = "user:static"

	// UserDynamicHashKey 用户动态计数 Hash 的 module 部分
	// Hash field: video_count, total_likes_received, total_play_count, experience, coin_balance_tenths, fans_count, following_count
	UserDynamicHashKey = "user:dynamic"

	// UserEmptyHashKey 用户空对象标记的 module 部分，防止缓存穿透
	UserEmptyHashKey = "user:empty"

	// ---- 互动缓存 ----
	// 投币数使用 Hash 存储，field="count"，value=投币数量字符串
	// 点赞/收藏/关注使用 SET 存储，member=userID，用于 O(1) 判重（SISMEMBER/SADD）

	VideoCoinHashKey = "video:coin" // 投币数 Hash

	// ---- 互动 SET 判重 ----
	// SET 数据结构：member=用户ID，O(1) 判重
	// SADD 返回值实现原子判重（返回 1=新增，0=已存在）

	// VideoLikedUsersSetKey 视频点赞用户集合 SET 的 module 部分
	// key 格式: {prefix}:video:liked_users:{videoID}
	// member=userID，用于 O(1) 判断用户是否已点赞
	VideoLikedUsersSetKey = "video:liked_users"

	// VideoFavoritedUsersSetKey 视频收藏用户集合 SET 的 module 部分
	// key 格式: {prefix}:video:favorited_users:{videoID}
	// member=userID，用于 O(1) 判断用户是否已收藏
	VideoFavoritedUsersSetKey = "video:favorited_users"

	// UserFollowersSetKey 用户粉丝集合 SET 的 module 部分
	// key 格式: {prefix}:user:followers:{followeeID}
	// member=followerID，用于 O(1) 判断用户是否已关注某人
	UserFollowersSetKey = "user:followers"

	// ---- 弹幕 ----

	DanmakuChannelPrefix = "danmaku:room"
	DanmakuCacheKey      = "danmaku:cache"

	// ---- 消息队列 ----

	PlayCountQueueName     = "video:play_count_increment"
	UserPlayCountQueueName = "user:play_count_increment"
	// VideoLikeCountQueueName 视频点赞数增量队列
	// worker 消费后延迟 3 秒批量聚合写入 MySQL videos.likes_count
	VideoLikeCountQueueName = "video:like_count_increment"
	// TranscodeQueueName 视频转码任务队列
	// worker 消费后调用 ffmpeg 转码为 H.264 MP4，上传到 Storage，更新 video_url/cover_url/status
	TranscodeQueueName = "mini_bili_transcode"
)

// 各缓存类型的过期时间配置
const (
	// VideoStaticHashExpire 视频静态信息缓存 3 天
	// 静态信息变更频率极低，由定时任务每日全量重建
	VideoStaticHashExpire = 3 * 24 * time.Hour

	// VideoDynamicHashExpire 视频动态计数缓存不过期
	// 动态数据由业务写操作实时更新，无需自动过期
	VideoDynamicHashExpire = 0

	// VideoEmptyHashExpire 空对象缓存 1 分钟
	// 短过期防止缓存穿透，同时允许数据被补充后快速失效
	VideoEmptyHashExpire = 1 * time.Minute

	// PublishedVideoZSetExpire 发布视频 ZSet 不过期
	// ZSet 由定时任务每 1 分钟全量重建，不需要自动过期
	PublishedVideoZSetExpire = 0

	// UserStaticHashExpire 用户静态信息缓存 3 天
	// 与视频静态信息保持一致，由定时任务每日全量重建
	UserStaticHashExpire = 3 * 24 * time.Hour

	// UserDynamicHashExpire 用户动态计数缓存不过期
	// 动态数据由业务写操作实时更新，无需自动过期
	UserDynamicHashExpire = 0

	// UserEmptyHashExpire 用户空对象缓存 1 分钟
	UserEmptyHashExpire = 1 * time.Minute

	// InteractionCacheExpire 互动状态缓存 7 天
	InteractionCacheExpire = 7 * 24 * time.Hour
)

// TTLJitter 为基础 TTL 添加 [0, maxJitter) 范围的随机抖动，防止同类缓存在同一时刻集体过期引发雪崩。
// 动态计数缓存（VideoDynamicHashExpire=0、UserDynamicHashExpire=0）不过期，不需要抖动。
func TTLJitter(base time.Duration, maxJitter time.Duration) time.Duration {
	if base <= 0 {
		return base // 不过期或无效值，直接返回
	}
	return base + time.Duration(rand.Int63n(int64(maxJitter)))
}
