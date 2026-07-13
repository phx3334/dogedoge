package mysql

import (
	"context"
	"fmt"

	"encoding/json"
	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/dto/cache"
	"fake_tiktok/internal/dto/response"
	"fake_tiktok/internal/repository/interfaces"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var _ interfaces.BackfillRepository = (*BackfillRepo)(nil)

// BackfillRepo 统一管理所有「缓存未命中 → 查 MySQL → 回填 Redis」的降级回源逻辑
type BackfillRepo struct {
	accountRepo          interfaces.AccountRepository
	videoCacheRepo       interfaces.VideoCacheRepository
	userCacheRepo        interfaces.UserCacheRepository
	interactionRepo      interfaces.InteractionRepository
	interactionCacheRepo interfaces.InteractionCacheRepository
	danmakuRepo          interfaces.DanmakuRepository
	danmakuCacheRepo     interfaces.DanmakuCacheRepository
	logger               *zap.Logger
}

func NewBackfillRepo(
	accountRepo interfaces.AccountRepository,
	videoCacheRepo interfaces.VideoCacheRepository,
	userCacheRepo interfaces.UserCacheRepository,
	interactionRepo interfaces.InteractionRepository,
	interactionCacheRepo interfaces.InteractionCacheRepository,
	danmakuRepo interfaces.DanmakuRepository,
	danmakuCacheRepo interfaces.DanmakuCacheRepository,
	logger *zap.Logger,
) *BackfillRepo {
	return &BackfillRepo{
		accountRepo:          accountRepo,
		videoCacheRepo:       videoCacheRepo,
		userCacheRepo:        userCacheRepo,
		interactionRepo:      interactionRepo,
		interactionCacheRepo: interactionCacheRepo,
		danmakuRepo:          danmakuRepo,
		danmakuCacheRepo:     danmakuCacheRepo,
		logger:               logger,
	}
}

// BackfillVideoCache 从 MySQL 回源视频信息并回填 Redis 缓存
func (r *BackfillRepo) BackfillVideoCache(ctx context.Context, dbVideos []database.Video, missedIDs []uint, cacheMap map[uint]*cache.VideoCacheData) {
	dbMap := make(map[uint]database.Video, len(dbVideos))
	for _, vid := range dbVideos {
		dbMap[vid.ID] = vid
	}

	// 收集缺失的作者 ID，批量查询作者名+头像
	authorIDs := r.collectMissingAuthorIDs(dbMap, missedIDs)
	cardMap := r.LookupAuthorCards(ctx, authorIDs)

	var writeItems []*cache.VideoCacheData
	var emptyIDs []uint

	for _, vid := range missedIDs {
		dbVid, ok := dbMap[vid]
		if !ok {
			// MySQL 中也不存在，标记为空对象防止缓存穿透
			emptyIDs = append(emptyIDs, vid)
			cacheMap[vid] = &cache.VideoCacheData{IsEmpty: true}
			continue
		}

		card := cardMap[dbVid.AuthorID]
		// 使用转换函数，消除 cacheMap 和 writeItems 的重复字段赋值
		cacheMap[vid] = cache.VideoToCacheData(&dbVid, card.Username, card.AvatarURL)
		writeItems = append(writeItems, cache.VideoToCacheData(&dbVid, card.Username, card.AvatarURL))
	}

	r.videoCacheRepo.WriteVideoCache(ctx, writeItems, emptyIDs)
}

// BackfillInteractionCache 从 MySQL 回源互动状态并回填 Redis 缓存
// 无论正负结果都回填：已点赞写 "1"，未点赞写 "0"，防止缓存穿透
func (r *BackfillRepo) BackfillInteractionCache(ctx context.Context, userID string, videoID uint, authorID string) *interfaces.InteractionStatus {
	status, err := r.interactionRepo.GetUserVideoInteraction(ctx, userID, videoID)
	if err != nil {
		return nil
	}
	if status == nil {
		return nil
	}

	// 查询关注状态
	if authorID != "" {
		followed, err := r.interactionRepo.IsUserFollowed(ctx, userID, authorID)
		if err == nil {
			status.IsFollowed = followed
		}
	}

	// 修复：使用 Redis Pipeline 将多个写操作合并为一次原子执行，
	// 避免部分成功导致缓存不一致（如点赞缓存回填成功但关注缓存回填失败）
	if err := r.interactionCacheRepo.BackfillBatch(ctx, userID, videoID, authorID, status); err != nil {
		r.logger.Warn("Pipeline 回填互动缓存失败", zap.Error(err))
	}

	return status
}

// FallbackCheckLike Redis 不可用时的降级点赞判重路径
// 通过 MySQL VideoLike 表判断用户是否已点赞，如果未点赞则写入点赞记录
//
// 此方法只做纯 MySQL 操作，熔断器和信号量由 logic 层负责包装
//
// 返回值：
//   - created=true：真正新建了记录（首次点赞），调用方应发布 MQ 增量
//   - created=false：记录已存在（SET 过期后重复点赞），调用方不应发布 MQ 增量
//   - err：MySQL 查询或写入失败
func (r *BackfillRepo) FallbackCheckLike(ctx context.Context, userID string, videoID uint) (bool, error) {
	// 降级查 MySQL：如果已点赞则幂等返回
	status, err := r.interactionRepo.GetUserVideoInteraction(ctx, userID, videoID)
	if err != nil {
		return false, fmt.Errorf("MySQL 查询点赞状态失败: %w", err)
	}
	if status.IsLiked {
		// MySQL 中已有点赞记录，幂等返回
		// created=false：不是新建的，调用方不应发布 MQ 增量
		return false, nil
	}

	// MySQL 中也无点赞记录，写入点赞记录
	// CreateVideoLike 返回 created=true 表示真正新建
	created, err := r.interactionRepo.CreateVideoLike(ctx, userID, videoID)
	if err != nil {
		return false, fmt.Errorf("MySQL 写入点赞记录失败: %w", err)
	}

	return created, nil
}

// BackfillUserCache 从 MySQL 回源用户信息（含粉丝数/关注数）并回填 Redis 缓存
//
// 返回值：
//   - data: 用户缓存数据（成功时返回，失败时返回 nil）
//   - err: 错误信息（用于上层记录日志，但不阻塞业务返回）
func (r *BackfillRepo) BackfillUserCache(ctx context.Context, userID string) (*cache.UserCacheData, error) {
	account, err := r.accountRepo.FindByID(ctx, userID)
	if err != nil {
		// 记录错误日志，便于排查 MySQL 故障
		return nil, fmt.Errorf("find account by id %s: %w", userID, err)
	}
	if account == nil {
		// 用户不存在（已注销或 ID 错误），不写缓存
		return nil, nil
	}

	static, dynamic := cache.AccountToUserCacheData(account)

	// 从 follow 表查询粉丝数和关注数
	fansCount, fansErr := r.interactionRepo.GetFansCount(ctx, userID)
	if fansErr == nil {
		dynamic.FansCount = fansCount
	}
	followingCount, followingErr := r.interactionRepo.GetFollowingCount(ctx, userID)
	if followingErr == nil {
		dynamic.FollowingCount = followingCount
	}

	data := cache.MergeUserCacheData(static, dynamic)

	// 回填 Redis 缓存
	// BatchWriteUserCache 内部已经记录错误，这里无需重复打日志
	r.userCacheRepo.BatchWriteUserCache(ctx, []cache.UserCacheData{
		*data,
	})

	return data, nil
}

// LookupAuthorNames 批量查询作者名称（供 mysqlFallback 等场景复用）
func (r *BackfillRepo) LookupAuthorNames(ctx context.Context, authorIDs []string) map[string]string {
	return r.lookupAuthorNamesByIDs(ctx, authorIDs)
}

// LookupAuthorCards 批量查询作者的用户名+头像 URL。
// 用于评论列表等需要展示用户卡片的场景；QueryError 时返回空 map（不阻塞业务）。
func (r *BackfillRepo) LookupAuthorCards(ctx context.Context, authorIDs []string) map[string]interfaces.AuthorCardInfo {
	if len(authorIDs) == 0 {
		return make(map[string]interfaces.AuthorCardInfo)
	}
	accounts, err := r.accountRepo.FindByIDs(ctx, authorIDs)
	if err != nil {
		return make(map[string]interfaces.AuthorCardInfo)
	}
	out := make(map[string]interfaces.AuthorCardInfo, len(accounts))
	for _, a := range accounts {
		out[a.ID] = interfaces.AuthorCardInfo{
			Username:   database.DisplayUsername(a),
			AvatarURL:  a.AvatarURL,
			Experience: a.Experience,
		}
	}
	return out
}

func (r *BackfillRepo) BackfillDanmakuCache(ctx context.Context, videoID uint64) ([]*response.DanmakuItem, error) {
	danmakus, err := r.danmakuRepo.FindByVideoID(ctx, videoID)
	if err != nil {
		return nil, err
	}
	members := make([]redis.Z, 0, len(danmakus))
	for _, dm := range danmakus {
		jsonData, _ := json.Marshal(response.DanmakuItem{
			ID:        dm.ID,
			Content:   dm.Content,
			VideoTime: dm.VideoTime,
			Color:     dm.Color,
			FontSize:  dm.FontSize,
			Mode:      danmakuTypeToMode(dm.Type),
			UserID:    dm.UserID,
			CreatedAt: dm.CreatedAt.Unix(),
		})
		members = append(members, redis.Z{
			Score:  float64(dm.CreatedAt.Unix()),
			Member: jsonData,
		})
	}
	r.danmakuCacheRepo.WriteDanmakuCache(ctx, videoID, members)
	items := make([]*response.DanmakuItem, 0, len(danmakus))
	for _, member := range members {
		var item response.DanmakuItem
		if err := json.Unmarshal(member.Member.([]byte), &item); err != nil {
			return nil, err
		}
		items = append(items, &item)
	}
	return items, nil
}

// danmakuTypeToMode 将 DB Type 字段（scroll/top/bottom）转换为前端弹幕模式（0/1/2）
func danmakuTypeToMode(t string) int {
	switch t {
	case "top":
		return 1
	case "bottom":
		return 2
	default:
		return 0
	}
}

// collectMissingAuthorIDs 从缺失的视频 ID 中收集不重复的作者 ID
func (r *BackfillRepo) collectMissingAuthorIDs(dbMap map[uint]database.Video, missedIDs []uint) []string {
	seen := make(map[string]bool)
	var ids []string
	for _, vid := range missedIDs {
		dbVid, ok := dbMap[vid]
		if !ok || dbVid.AuthorID == "" {
			continue
		}
		if seen[dbVid.AuthorID] {
			continue
		}
		seen[dbVid.AuthorID] = true
		ids = append(ids, dbVid.AuthorID)
	}
	return ids
}

// lookupAuthorNamesByIDs 批量查询作者名称
func (r *BackfillRepo) lookupAuthorNamesByIDs(ctx context.Context, authorIDs []string) map[string]string {
	if len(authorIDs) == 0 {
		return make(map[string]string)
	}

	accounts, err := r.accountRepo.FindByIDs(ctx, authorIDs)
	if err != nil {
		return make(map[string]string)
	}

	nameMap := make(map[string]string, len(accounts))
	for _, account := range accounts {
		nameMap[account.ID] = database.DisplayUsername(account)
	}
	return nameMap
}
