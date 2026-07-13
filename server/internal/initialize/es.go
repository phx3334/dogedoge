package initialize

import (
	"context"
	"fake_tiktok/internal/config"
	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/domain/es"
	"fake_tiktok/internal/repository/interfaces"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/typedapi/types"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func ConnectEs(cfg *config.Config) *elasticsearch.TypedClient {
	esCfg := cfg.Elasticsearch
	esClientCfg := elasticsearch.Config{
		Addresses: []string{esCfg.Host},
		Username:  esCfg.Username,
		Password:  esCfg.Password,
	}
	client, err := elasticsearch.NewTypedClient(esClientCfg)
	if err != nil {
		// 修复：创建客户端失败时记录错误并返回 nil，而非直接退出进程
		// 调用方应检查返回值，ES 不可用时搜索功能降级
		zap.L().Error("创建 Elasticsearch 客户端失败，搜索功能将不可用", zap.Error(err))
		return nil
	}
	return client
}

// InitEsIndex 初始化 Elasticsearch 索引，确保 VideoIndex 和 LogIndex 存在。
// 使用带 30s 超时的 context，避免 ES 不可用时初始化过程无限阻塞。
func InitEsIndex(esRepo interfaces.SearchIndexRepository) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ensureIndex(ctx, esRepo, es.GetVideoIndex(), es.GetVideoMapping())
	ensureIndex(ctx, esRepo, es.LogIndex(), es.LogMapping())
}

// ensureIndex 确保指定的 ES 索引存在，不存在则创建。
// 使用调用方传入的 context 支持超时控制。
func ensureIndex(ctx context.Context, esRepo interfaces.SearchIndexRepository, name string, mapping *types.TypeMapping) {
	// 修复：ES 索引检查失败时重试3次，而非直接退出进程
	// 生产环境中 ES 可能短暂不可用，不应阻止整个应用启动
	var exists bool
	var lastErr error
	for retry := 0; retry < 3; retry++ {
		exists, lastErr = esRepo.IndexExists(ctx, name)
		if lastErr == nil {
			break
		}
		zap.L().Warn("检查索引是否存在失败，正在重试",
			zap.String("index", name), zap.Int("retry", retry+1), zap.Error(lastErr))
		time.Sleep(time.Second * time.Duration(retry+1))
	}
	if lastErr != nil {
		zap.L().Error("检查索引是否存在失败，搜索功能将降级",
			zap.String("index", name), zap.Error(lastErr))
		// 不再 os.Exit(1)，允许应用在 ES 不可用时继续启动
		return
	}

	if exists {
		zap.L().Info("ES index already exists, skipping", zap.String("index", name))
		return
	}
	zap.L().Info("Creating ES index...", zap.String("index", name))
	if err := esRepo.CreateIndex(ctx, name, mapping); err != nil {
		// 修复：创建索引失败时也改为重试降级策略，而非直接退出进程
		zap.L().Error("创建 ES 索引失败，搜索功能将降级",
			zap.String("index", name), zap.Error(err))
		return
	}
	zap.L().Info("ES index created successfully", zap.String("index", name))
}

// BackfillEsVideoIndex 全量回填已发布视频到 ES 索引。
// 在应用启动时调用一次，把 MySQL 中 status=published 的视频同步到 ES，
// 保证存量视频可被搜索到（新视频由 worker 转码完成后实时索引）。
//
// 幂等：ES Index 以视频 ID 为文档 _id，重复写入会覆盖，不会产生重复文档。
// ES 不可用时跳过，不影响应用启动。
func BackfillEsVideoIndex(db *gorm.DB, videoSearchRepo interfaces.VideoSearchRepository, logger *zap.Logger) {
	if videoSearchRepo == nil {
		logger.Info("ES video search repo not available, skip backfill")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 查询所有已发布视频
	var videos []database.Video
	if err := db.WithContext(ctx).
		Where("status = ?", "published").
		Find(&videos).Error; err != nil {
		logger.Error("backfill ES: query published videos failed", zap.Error(err))
		return
	}

	if len(videos) == 0 {
		logger.Info("backfill ES: no published videos to index")
		return
	}

	// 批量查询作者用户名（避免 N+1 查询）
	authorIDs := make(map[string]struct{}, len(videos))
	for _, v := range videos {
		authorIDs[v.AuthorID] = struct{}{}
	}
	var authors []database.Account
	if len(authorIDs) > 0 {
		ids := make([]string, 0, len(authorIDs))
		for id := range authorIDs {
			ids = append(ids, id)
		}
		if err := db.WithContext(ctx).Where("id IN ?", ids).Find(&authors).Error; err != nil {
			logger.Error("backfill ES: query authors failed", zap.Error(err))
			return
		}
	}
	authorMap := make(map[string]string, len(authors))
	for _, a := range authors {
		authorMap[a.ID] = a.Username
	}

	indexed := 0
	for _, v := range videos {
		doc := es.NewVideoDocumentFromVideo(v, authorMap[v.AuthorID])
		if err := videoSearchRepo.IndexVideo(ctx, doc); err != nil {
			logger.Warn("backfill ES: index video failed",
				zap.Uint("video_id", v.ID), zap.Error(err))
			continue
		}
		indexed++
	}
	logger.Info("backfill ES: completed",
		zap.Int("total", len(videos)),
		zap.Int("indexed", indexed))
}
