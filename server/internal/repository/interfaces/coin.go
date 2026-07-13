package interfaces

import (
	"context"

	"fake_tiktok/internal/domain/database"
)

// VideoCoinRepository 视频投币数据接口
//
// 一个用户对一个视频最多投 2 个硬币（Amount=1 或 2）。
// 通过唯一索引 idx_video_coin_user_video 保证幂等：再次投币时只更新 Amount。
type VideoCoinRepository interface {
	// FindByUserVideo 查询用户对某视频的投币记录
	// 不存在返回 gorm.ErrRecordNotFound
	FindByUserVideo(ctx context.Context, userID string, videoID uint) (*database.VideoCoin, error)

	// Upsert 创建或更新投币记录
	//   - 不存在：插入新记录，amount 即为本次投币数
	//   - 已存在：累加 amount 到现有 Amount（上限 2），返回 added=本次新增的硬币数
	// amount 必须 ∈ {1, 2}；累计超过 2 时只取 2，added 为实际新增数
	Upsert(ctx context.Context, userID string, videoID uint, amount int) (added int, err error)
}

// CoinLedgerRepository 硬币流水数据接口
type CoinLedgerRepository interface {
	// Create 写入一条流水
	Create(ctx context.Context, l *database.CoinLedger) error

	// ListByUser 分页查询用户流水（按时间倒序）
	// reasonType 为空时查所有类型
	ListByUser(ctx context.Context, userID uint64, reasonType string, page, pageSize int) ([]database.CoinLedger, int64, error)
}
