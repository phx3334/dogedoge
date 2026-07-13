package logic

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"fake_tiktok/internal/pkg"
	"time"

	"fake_tiktok/internal/breaker"
	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/dto/cache"
	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/dto/response"

	"github.com/elastic/go-elasticsearch/v8/typedapi/types"
	"go.uber.org/zap"
)

type SearchLogic struct {
	deps *LogicDeps
}

func NewSearchLogic(deps *LogicDeps) *SearchLogic {
	return &SearchLogic{deps: deps}
}

// SearchVideos 按关键词搜索视频，使用加权评分和游标分页
//
// 搜索流程三步走（职责分离）：
//   - 第 1 步（ES）：关键词匹配，返回视频 ID 列表 + 分页游标
//   - 第 2 步（Redis）：批量查缓存，组装视频详情（标题、封面、作者名等）
//   - 第 3 步（MySQL）：Redis 未命中的 ID 从 MySQL 回源并回填 Redis
//
// 熔断保护：
//   - ES 熔断：ES 持续不可用时快速失败，返回错误提示
//   - Redis 熔断：Redis 不可用时跳过缓存，直接走 MySQL 降级
//   - MySQL 熔断：MySQL 也不可用时返回空结果
//
// 信号量：
//   - 仅 MySQL 读操作加信号量（MySQLReadSem），防止超出连接池容量
//   - ES 和 Redis 不需要信号量：ES 连接池由官方 SDK 管理，Redis 使用 Pipeline 批量操作
func (s *SearchLogic) SearchVideos(ctx context.Context, req request.SearchVideoReq) ([]response.HomeVideoInfo, string, error) {
	// ---- 解析游标 ----
	// base64 编码的 JSON 数组 [score, id]
	// 游标损坏时记录警告日志并返回错误，避免静默从第一页重新搜索导致用户看到重复结果
	var cursor []types.FieldValue
	if req.Cursor != "" {
		decoded, err := base64.StdEncoding.DecodeString(req.Cursor)
		if err != nil {
			s.deps.Logger.Warn("游标 base64 解码失败，游标可能已损坏", zap.String("cursor", req.Cursor), zap.Error(err))
			return nil, "", fmt.Errorf("invalid cursor: base64 decode failed: %w", err)
		}
		if err := json.Unmarshal(decoded, &cursor); err != nil {
			s.deps.Logger.Warn("游标 JSON 反序列化失败，游标可能已损坏", zap.String("cursor", req.Cursor), zap.Error(err))
			return nil, "", fmt.Errorf("invalid cursor: json unmarshal failed: %w", err)
		}
	}

	limit := req.Limit
	if limit <= 0 || limit > 30 {
		limit = 10
	}

	// ---- 第 1 步：ES 关键词匹配，返回视频 ID 列表 ----
	// ES 熔断保护：ES 持续不可用时快速失败，不阻塞后续请求
	videoIDs, nextCursor, err := s.esSearch(ctx, req.Keyword, cursor, limit)
	if err != nil {
		if errors.Is(err, breaker.ErrCircuitOpen) {
			s.deps.Logger.Warn("ES 熔断器开启，搜索不可用", zap.String("keyword", req.Keyword))
			return nil, "", fmt.Errorf("搜索服务暂时不可用，请稍后重试")
		}
		return nil, "", err
	}

	if len(videoIDs) == 0 {
		return []response.HomeVideoInfo{}, "", nil
	}

	// ---- 第 2 步：Redis 批量组装视频详情 ----
	// Redis 熔断保护：Redis 不可用时降级到 MySQL 全量回源
	videos := s.assembleFromRedis(ctx, videoIDs)

	// ---- 第 3 步：MySQL 兜底回源 ----
	// 对 Redis 未命中的视频 ID，从 MySQL 回源并回填 Redis
	// MySQL 熔断 + 信号量保护：防止连接池耗尽
	s.backfillFromMySQL(ctx, videoIDs, videos)

	// ---- 编码下一页游标 ----
	var nextCursorStr string
	if nextCursor != nil {
		// 修复：检查 json.Marshal 错误，序列化失败时 nextCursorStr 为空，
		// 客户端会从第一页重新搜索，导致重复结果
		encoded, marshalErr := json.Marshal(nextCursor)
		if marshalErr != nil {
			s.deps.Logger.Warn("序列化游标失败，客户端将从第一页重新搜索",
				zap.Error(marshalErr))
		}
		nextCursorStr = base64.StdEncoding.EncodeToString(encoded)
	}

	// ---- 按 ES 返回顺序组装最终结果 ----
	// 保持 ES 的相关性排序，Redis/MySQL 回源结果按 ID 索引
	result := make([]response.HomeVideoInfo, 0, len(videoIDs))
	for _, idStr := range videoIDs {
		vid := pkg.MustParseUint(idStr)
		if v, ok := videos[vid]; ok {
			result = append(result, v)
		}
	}

	return result, nextCursorStr, nil
}

// esSearch 第 1 步：ES 关键词匹配
// ES 熔断保护：ES 持续不可用时快速失败
func (s *SearchLogic) esSearch(ctx context.Context, keyword string, cursor []types.FieldValue, limit int) ([]string, []types.FieldValue, error) {
	var ids []string
	var nextCursor []types.FieldValue
	esErr := s.deps.Breakers.ES.Execute(func() error {
		var err error
		ids, nextCursor, err = s.deps.VideoSearchRepo.SearchVideos(ctx, keyword, cursor, limit)
		return err
	})
	if esErr != nil {
		return nil, nil, esErr
	}
	return ids, nextCursor, nil
}

// assembleFromRedis 第 2 步：Redis 批量组装视频详情
// Redis 熔断保护：Redis 不可用时返回空 map，由第 3 步 MySQL 兜底
func (s *SearchLogic) assembleFromRedis(ctx context.Context, videoIDs []string) map[uint]response.HomeVideoInfo {
	// 将 string ID 转换为 uint ID（Redis 缓存以 uint 为 key）
	uintIDs := make([]uint, 0, len(videoIDs))
	for _, idStr := range videoIDs {
		if vid := pkg.MustParseUint(idStr); vid > 0 {
			uintIDs = append(uintIDs, vid)
		}
	}
	if len(uintIDs) == 0 {
		return make(map[uint]response.HomeVideoInfo)
	}

	// Redis 批量查缓存（通过熔断器保护）
	// GetVideoCache 返回 error，闭包将错误返回给熔断器，
	// 使熔断器能感知 Redis 不可用并累计失败次数
	var cacheMap map[uint]*cache.VideoCacheData
	redisErr := s.deps.Breakers.Redis.Execute(func() error {
		var err error
		cacheMap, _, err = s.deps.VideoCacheRepo.GetVideoCache(ctx, uintIDs)
		return err
	})
	if redisErr != nil {
		if errors.Is(redisErr, breaker.ErrCircuitOpen) {
			s.deps.Logger.Warn("Redis 熔断器开启，跳过缓存组装，走 MySQL 降级")
		} else {
			s.deps.Logger.Warn("Redis 缓存查询失败，走 MySQL 降级", zap.Error(redisErr))
		}
		return make(map[uint]response.HomeVideoInfo)
	}

	// 将缓存数据转换为响应结构
	videos := make(map[uint]response.HomeVideoInfo, len(uintIDs))
	for _, vid := range uintIDs {
		data, ok := cacheMap[vid]
		if !ok || data.IsEmpty {
			continue
		}
		var createdAt time.Time
		if !data.CreatedAt.IsZero() {
			createdAt = data.CreatedAt
		}
		videos[vid] = response.HomeVideoInfo{
			ID:           vid,
			UpName:       data.AuthorName,
			UpAvatar:     data.AuthorAvatar,
			Title:        data.Title,
			CoverURL:     data.CoverURL, 
			PlayCount:    data.PlayCount,
			CommentCount: data.CommentCnt,
			Duration:     data.Duration,
			CreatedAt:    createdAt,
			FavCount:     data.FavCnt,
		}
	}

	return videos
}

// backfillFromMySQL 第 3 步：MySQL 兜底回源
// 对 Redis 未命中的视频 ID，从 MySQL 查询并回填 Redis
// MySQL 熔断 + 信号量保护：防止连接池耗尽
func (s *SearchLogic) backfillFromMySQL(ctx context.Context, videoIDs []string, videos map[uint]response.HomeVideoInfo) {
	// 收集 Redis 未命中的视频 ID
	var missedIDs []uint
	for _, idStr := range videoIDs {
		vid := pkg.MustParseUint(idStr)
		if vid > 0 {
			if _, ok := videos[vid]; !ok {
				missedIDs = append(missedIDs, vid)
			}
		}
	}
	if len(missedIDs) == 0 {
		return
	}

	// MySQL 信号量：限制并发读请求数，防止超出连接池容量
	// ES 和 Redis 不需要信号量：ES 连接池由 SDK 管理，Redis 使用 Pipeline
	if err := s.deps.Breakers.MySQLReadSem.Acquire(ctx, 1); err != nil {
		s.deps.Logger.Warn("MySQL 读信号量获取失败，跳过回源", zap.Error(err))
		return
	}
	defer s.deps.Breakers.MySQLReadSem.Release(1)

	// MySQL 熔断保护：MySQL 持续不可用时快速失败，不阻塞请求
	// 与 video.go 保持一致：在熔断器闭包内调用 VideoRepo.FindPublishedVideosByIDs 查 MySQL，
	// 将真实错误返回给熔断器，使熔断器能感知 MySQL 不可用并累计失败次数
	// 查询成功后，将结果传给 BackfillRepo.BackfillVideoCache 回填 Redis 缓存
	var dbVideos []database.Video
	breakerErr := s.deps.Breakers.MySQL.Execute(func() error {
		var err error
		dbVideos, err = s.deps.VideoRepo.FindPublishedVideosByIDs(ctx, missedIDs)
		return err
	})
	if breakerErr != nil {
		if errors.Is(breakerErr, breaker.ErrCircuitOpen) {
			s.deps.Logger.Warn("MySQL 熔断器开启，跳过搜索结果回源")
		} else {
			s.deps.Logger.Warn("MySQL 回源搜索结果失败", zap.Error(breakerErr))
		}
		return
	}

	// 将 MySQL 查询结果传给 BackfillRepo 回填 Redis 缓存
	// 注意：必须传入 dbVideos 而非 nil，否则所有 missedIDs 会被错误标记为空对象
	cacheMap := make(map[uint]*cache.VideoCacheData)
	s.deps.BackfillRepo.BackfillVideoCache(ctx, dbVideos, missedIDs, cacheMap)

	// 将回源结果补充到 videos map
	for _, vid := range missedIDs {
		if data, ok := cacheMap[vid]; ok && !data.IsEmpty {
			var createdAt time.Time
			if !data.CreatedAt.IsZero() {
				createdAt = data.CreatedAt
			}
			videos[vid] = response.HomeVideoInfo{
				ID:           vid,
				UpName:       data.AuthorName,
				UpAvatar:     data.AuthorAvatar,
				Title:        data.Title,
				CoverURL:     data.CoverURL,
				PlayCount:    data.PlayCount,
				CommentCount: data.CommentCnt,
				Duration:     data.Duration,
				CreatedAt:    createdAt,
				FavCount:     data.FavCnt,
			}
		}
	}
}
