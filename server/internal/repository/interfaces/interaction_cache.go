package interfaces

import "context"

// InteractionCacheRepository 互动状态缓存接口
//
// 数据结构说明：
//   - 点赞/收藏/关注：SET，member=用户ID，用于 O(1) 判重（SISMEMBER/SADD）
//     SET 不存在 → 缓存未命中；member 存在 → 已操作，不存在 → 未操作
//     SADD 返回值实现原子判重（返回 1=新增，0=已存在）
//   - 投币数：Hash，field="count"，value=数量字符串
type InteractionCacheRepository interface {
	// GetInteractionBatch 通过 Pipeline 一次性查询点赞/收藏/投币/关注缓存
	//
	// 数据结构：
	//   - 点赞/收藏/关注：SISMEMBER 查询 SET 中是否存在该用户
	//   - 投币数：HGet 查询 Hash 中的 count 字段
	//
	// 返回值说明：
	//   - isLiked/isFavorited/isFollowed: 已命中字段的值（未命中字段为零值）
	//   - coinCount: 投币数（未命中为零值）
	//   - hitMask: 位掩码，标记哪些字段命中了缓存
	//     bit0=点赞, bit1=收藏, bit2=投币, bit3=关注
	//     调用方据此决定哪些字段需要降级回源，避免丢弃已命中的缓存值
	//   - err: Pipeline 整体执行错误
	GetInteractionBatch(ctx context.Context, userID string, videoID uint, authorID string) (isLiked, isFavorited, isFollowed bool, coinCount int64, hitMask uint8, err error)

	// ---- 点赞 SET ----
	// IsVideoLikedByUser 使用 SET 判断用户是否已点赞某视频（O(1) 复杂度）
	// SET key 格式: video:liked_users:{videoID}，member=userID
	// 返回 true=已点赞，false=未点赞，error=Redis 错误
	IsVideoLikedByUser(ctx context.Context, userID string, videoID uint) (bool, error)
	// AddUserToVideoLikedSet 将用户加入视频的点赞 SET（标记已点赞）
	// 返回值 added=true 表示本次新增（首次点赞），added=false 表示用户已在 SET 中（重复点赞）
	// 通过 SADD 的返回值实现原子判重，避免 SISMEMBER + SADD 两步非原子的竞态条件
	AddUserToVideoLikedSet(ctx context.Context, userID string, videoID uint) (added bool, err error)

	// ---- 收藏 SET ----
	// IsVideoFavoritedByUser 使用 SET 判断用户是否已收藏某视频（O(1) 复杂度）
	// SET key 格式: video:favorited_users:{videoID}，member=userID
	// 返回 true=已收藏，false=未收藏，error=Redis 错误
	IsVideoFavoritedByUser(ctx context.Context, userID string, videoID uint) (bool, error)
	// AddUserToVideoFavoriteSet 将用户加入视频的收藏 SET（标记已收藏）
	// 返回值 added=true 表示本次新增（首次收藏），added=false 表示用户已在 SET 中（重复收藏）
	AddUserToVideoFavoriteSet(ctx context.Context, userID string, videoID uint) (added bool, err error)

	// ---- 投币 Hash ----
	GetCoinCount(ctx context.Context, userID string, videoID uint) (int64, error)
	SetCoinCount(ctx context.Context, userID string, videoID uint, count int64) error

	// ---- 关注 SET ----
	// IsUserFollowedByUser 使用 SET 判断用户是否已关注某作者（O(1) 复杂度）
	// SET key 格式: user:followers:{followeeID}，member=followerID
	// 返回 true=已关注，false=未关注，error=Redis 错误
	IsUserFollowedByUser(ctx context.Context, followerID, followeeID string) (bool, error)
	// AddUserToFollowersSet 将用户加入关注对象的粉丝 SET（标记已关注）
	// 返回值 added=true 表示本次新增（首次关注），added=false 表示已在 SET 中（重复关注）
	AddUserToFollowersSet(ctx context.Context, followerID, followeeID string) (added bool, err error)

	// ---- 回滚方法（MySQL 写入失败时回滚 Redis SET） ----
	// RemoveUserFromVideoLikedSet 从视频点赞 SET 中移除用户（回滚 SADD 操作）
	RemoveUserFromVideoLikedSet(ctx context.Context, userID string, videoID uint) error
	// RemoveUserFromVideoFavoriteSet 从视频收藏 SET 中移除用户（回滚 SADD 操作）
	RemoveUserFromVideoFavoriteSet(ctx context.Context, userID string, videoID uint) error
	// RemoveUserFromFollowersSet 从关注 SET 中移除用户（回滚 SADD 操作）
	RemoveUserFromFollowersSet(ctx context.Context, followerID, followeeID string) error

	// ---- Pipeline 批量回填 ----
	// BackfillBatch 使用 Redis Pipeline 将多个互动缓存写操作合并为一次原子执行，
	// 避免部分成功导致缓存不一致（如点赞缓存回填成功但关注缓存回填失败）
	BackfillBatch(ctx context.Context, userID string, videoID uint, authorID string, status *InteractionStatus) error
}
