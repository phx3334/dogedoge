package cache

import "fake_tiktok/internal/domain/database"

// UserCacheData 用户缓存组合数据（静态 + 动态），供 logic 层使用。
// Redis 存储层拆分为 user:static 和 user:dynamic 两个 Hash，
// GetUserCache 通过 Pipeline 合并两个 Hash 的结果返回此组合结构。
type UserCacheData struct {
	ID                     string `json:"id"`
	AvatarURL              string `json:"avatar_url"`
	Signature              string `json:"signature"`
	Username               string `json:"username"`
	Address                string `json:"address"`
	VideoCount             int64  `json:"video_count"`
	Birthday               string `json:"birthday"`
	Gender                 string `json:"gender"`
	TotalLikesReceived     int64  `json:"total_likes_received"`
	TotalPlayCount         int64  `json:"total_play_count"`
	Experience             uint64 `json:"experience"`
	PrivacyPublicFavorites bool   `json:"privacy_public_favorites"`
	PrivacyPublicFollowing bool   `json:"privacy_public_following"`
	PrivacyPublicFans      bool   `json:"privacy_public_fans"`
	CoinBalanceTenths      int64  `json:"coin_balance_tenths"`
	ViewHistoryPaused      bool   `json:"view_history_paused"`
	FansCount              int64  `json:"fans_count"`
	FollowingCount         int64  `json:"following_count"`

	// StaticHit 标记静态区缓存是否命中，由 GetUserCache 设置。
	// 调用方可据此决定是否对 static 区做降级回源：
	//   - StaticHit=true：静态区正常，无需回源
	//   - StaticHit=false：静态区未命中（可能被 DeleteUserCache 删除或过期），
	//     需要单独回源 static 区（动态区计数由 HIncrBy 实时维护，不需要回源）
	StaticHit bool `json:"-"`
}

// UserStaticData 用户静态信息，存储在 user:static:{id} Hash
type UserStaticData struct {
	ID                     string `json:"id"`
	Username               string `json:"username"`
	AvatarURL              string `json:"avatar_url"`
	Signature              string `json:"signature"`
	Address                string `json:"address"`
	Birthday               string `json:"birthday"`
	Gender                 string `json:"gender"`
	PrivacyPublicFavorites bool   `json:"privacy_public_favorites"`
	PrivacyPublicFollowing bool   `json:"privacy_public_following"`
	PrivacyPublicFans      bool   `json:"privacy_public_fans"`
	ViewHistoryPaused      bool   `json:"view_history_paused"`
}

// UserDynamicData 用户动态计数，存储在 user:dynamic:{id} Hash
type UserDynamicData struct {
	VideoCount         int64  `json:"video_count"`
	TotalLikesReceived int64  `json:"total_likes_received"`
	TotalPlayCount     int64  `json:"total_play_count"`
	Experience         uint64 `json:"experience"`
	CoinBalanceTenths  int64  `json:"coin_balance_tenths"`
	FansCount          int64  `json:"fans_count"`
	FollowingCount     int64  `json:"following_count"`
}

// UserPlayCountIncrementMsg 用户总播放量增量消息（RabbitMQ）
type UserPlayCountIncrementMsg struct {
	UserID    string `json:"user_id"`
	Increment int64  `json:"increment"`
}

// AccountToUserCacheData 将 database.Account 转换为 UserStaticData 和 UserDynamicData
// 注意：Account 表没有 fans_count 和 following_count，这两个字段由 BackfillUserCache 从 follow 表回填
func AccountToUserCacheData(acc *database.Account) (*UserStaticData, *UserDynamicData) {
	static := &UserStaticData{
		ID:                     acc.ID,
		Username:               acc.Username,
		AvatarURL:              acc.AvatarURL,
		Signature:              acc.Signature,
		Address:                acc.Address,
		Birthday:               acc.Birthday,
		Gender:                 acc.Gender,
		PrivacyPublicFavorites: acc.PrivacyPublicFavorites,
		PrivacyPublicFollowing: acc.PrivacyPublicFollowing,
		PrivacyPublicFans:      acc.PrivacyPublicFans,
		ViewHistoryPaused:      acc.ViewHistoryPaused,
	}
	dynamic := &UserDynamicData{
		VideoCount:         acc.VideoCount,
		TotalLikesReceived: acc.TotalLikesReceived,
		TotalPlayCount:     acc.TotalPlayCount,
		Experience:         acc.Experience,
		CoinBalanceTenths:  acc.CoinBalanceTenths,
		// FansCount 和 FollowingCount 由 BackfillUserCache 从 follow 表查询后设置
	}
	return static, dynamic
}

// MergeUserCacheData 合并静态和动态数据为组合结构
func MergeUserCacheData(static *UserStaticData, dynamic *UserDynamicData) *UserCacheData {
	if static == nil && dynamic == nil {
		return nil
	}
	data := &UserCacheData{}
	if static != nil {
		data.ID = static.ID
		data.Username = static.Username
		data.AvatarURL = static.AvatarURL
		data.Signature = static.Signature
		data.Address = static.Address
		data.Birthday = static.Birthday
		data.Gender = static.Gender
		data.PrivacyPublicFavorites = static.PrivacyPublicFavorites
		data.PrivacyPublicFollowing = static.PrivacyPublicFollowing
		data.PrivacyPublicFans = static.PrivacyPublicFans
		data.ViewHistoryPaused = static.ViewHistoryPaused
	}
	if dynamic != nil {
		data.VideoCount = dynamic.VideoCount
		data.TotalLikesReceived = dynamic.TotalLikesReceived
		data.TotalPlayCount = dynamic.TotalPlayCount
		data.Experience = dynamic.Experience
		data.CoinBalanceTenths = dynamic.CoinBalanceTenths
		data.FansCount = dynamic.FansCount
		data.FollowingCount = dynamic.FollowingCount
	}
	return data
}
