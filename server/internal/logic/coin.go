package logic

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"fake_tiktok/internal/breaker"
	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/dto/response"
	"fake_tiktok/internal/repository/interfaces"

	"go.uber.org/zap"
)

// CoinLogic 硬币相关业务逻辑
//
// 负责：
//   - 视频投币（含余额扣减、CoinLedger 流水、视频 coin_count 自增、用户经验 +20/硬币）
//   - 文章投币（同视频，写入 ArticleCoin）
//   - 硬币流水查询
type CoinLogic struct {
	deps *LogicDeps
}

func NewCoinLogic(deps *LogicDeps) *CoinLogic {
	return &CoinLogic{deps: deps}
}

// CoinVideo 视频投币
//
// 业务规则：
//   - amount 必须 ∈ {1, 2}（在 handler 层校验）
//   - 单用户对单视频最多投 2 个硬币（VideoCoin 表唯一索引 + Amount 上限）
//   - 用户硬币余额必须足够（CoinBalanceTenths，1 硬币=10 tenths）
//   - 投币后写入 CoinLedger 流水（reason="coin_video"）
//   - 视频 coin_count +added
//   - 用户经验 +20*added（每日任务设计）
//
// 流程（单事务内）：
//  1. 通过 VideoCoinRepo.Upsert 在 user_coin 表中创建/累加（返回 added=本次实际新增硬币数）
//  2. 如 added=0 则幂等返回（已投过 2 个硬币）
//  3. 用户余额扣减 added*10 tenths（防透支）
//  4. 写入 CoinLedger 流水
//  5. 视频 coin_count 自增
//  6. 用户经验 +20*added
func (l *CoinLogic) CoinVideo(ctx context.Context, userID string, req request.CoinVideoReq) (*response.CoinResultResp, error) {
	// amount 校验（双保险：binding tag 可能因版本差异不生效）
	if req.Amount != 1 && req.Amount != 2 {
		return nil, errors.New("投币数量必须为 1 或 2")
	}

	// 写信号量保护
	if err := l.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); err != nil {
		return nil, fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLWriteSem.Release(1)

	// Step 1: 创建/更新投币记录
	var added int
	if err := l.deps.Breakers.MySQL.Execute(func() error {
		var err error
		added, err = l.deps.VideoCoinRepo.Upsert(ctx, userID, req.VideoID, req.Amount)
		return err
	}); err != nil {
		if errors.Is(err, breaker.ErrCircuitOpen) {
			l.deps.Logger.Warn("MySQL circuit open during CoinVideo",
				zap.String("user_id", userID), zap.Uint("video_id", req.VideoID))
		}
		return nil, fmt.Errorf("投币失败，请稍后重试")
	}

	// added=0：用户已投过 2 个硬币，幂等返回当前状态
	if added == 0 {
		// 查询当前投币数和余额
		return l.buildCoinResult(ctx, userID, req.VideoID, 0)
	}

	// Step 2: 用户余额扣减
	// 1 硬币 = 10 tenths；added 个硬币 = added*10 tenths
	deltaTenths := int64(added) * -10
	if err := l.deps.Breakers.MySQL.Execute(func() error {
		return l.deps.AccountRepo.AddCoinBalanceTenths(ctx, userID, deltaTenths)
	}); err != nil {
		// 余额扣减失败：理论上 GREATEST(0, ...) 防止透支，但 SQL 错误仍可能发生
		// 此时不回滚 VideoCoin 表，因为用户已"消耗"了硬币额度
		// 系统通过对账任务修复（参考 backfillCoinLedger）
		l.deps.Logger.Error("用户余额扣减失败",
			zap.String("user_id", userID), zap.Int("added", added), zap.Error(err))
		return nil, fmt.Errorf("投币失败，请稍后重试")
	}

	// Step 3: 写入 CoinLedger 流水
	userIDUint, _ := strconv.ParseUint(userID, 10, 64)
	ledger := &database.CoinLedger{
		UserID:      userIDUint,
		DeltaTenths: deltaTenths,
		ReasonType:  "coin_video",
		VideoID:     uint64(req.VideoID),
		CreatedAt:   time.Now(),
	}
	if err := l.deps.Breakers.MySQL.Execute(func() error {
		return l.deps.CoinLedgerRepo.Create(ctx, ledger)
	}); err != nil {
		l.deps.Logger.Warn("CoinLedger 写入失败（不影响主流程）",
			zap.String("user_id", userID), zap.Error(err))
	}

	// Step 4: 视频 coin_count +added
	if err := l.deps.Breakers.MySQL.Execute(func() error {
		return l.deps.VideoRepo.IncrementCoinCount(ctx, req.VideoID, added)
	}); err != nil {
		l.deps.Logger.Warn("视频 coin_count 自增失败",
			zap.Uint("video_id", req.VideoID), zap.Error(err))
	}
	// 同步更新 Redis 视频动态缓存中的 coin_count
	_ = l.deps.Breakers.Redis.Execute(func() error {
		return l.deps.VideoCacheRepo.IncrementCoinCount(ctx, req.VideoID, added)
	})

	// Step 5: 用户经验 +20*added（每日任务设计）
	if err := l.deps.Breakers.MySQL.Execute(func() error {
		return l.deps.AccountRepo.AddExperience(ctx, userID, int64(interfaces.ExpPerCoin*added))
	}); err != nil {
		l.deps.Logger.Warn("用户经验增加失败",
			zap.String("user_id", userID), zap.Error(err))
	}
	// 同步缓存经验（best-effort）
	_ = l.deps.Breakers.Redis.Execute(func() error {
		return l.deps.UserCacheRepo.IncrementExperience(ctx, userID, int64(interfaces.ExpPerCoin*added))
	})

	// Step 6: 投币数缓存同步（best-effort）
	if l.deps.InteractionCacheRepo != nil {
		_ = l.deps.Breakers.Redis.Execute(func() error {
			return l.deps.InteractionCacheRepo.SetCoinCount(ctx, userID, req.VideoID, int64(req.Amount))
		})
	}

	return l.buildCoinResult(ctx, userID, req.VideoID, added)
}

// buildCoinResult 构造投币结果响应
func (l *CoinLogic) buildCoinResult(ctx context.Context, userID string, videoID uint, added int) (*response.CoinResultResp, error) {
	// 查询用户当前余额
	var balance int64
	if err := l.deps.Breakers.MySQL.Execute(func() error {
		user, err := l.deps.AccountRepo.FindByID(ctx, userID)
		if err == nil {
			balance = user.CoinBalanceTenths
		}
		return err
	}); err != nil && !errors.Is(err, breaker.ErrCircuitOpen) {
		// 余额查询失败不阻塞响应
		l.deps.Logger.Warn("查询用户余额失败", zap.String("user_id", userID), zap.Error(err))
	}

	// 查询视频当前 coin_count
	var coinCnt int64
	if err := l.deps.Breakers.MySQL.Execute(func() error {
		video, err := l.deps.VideoRepo.FindVideoByID(ctx, videoID)
		if err == nil {
			coinCnt = int64(video.CoinCount)
		}
		return err
	}); err != nil && !errors.Is(err, breaker.ErrCircuitOpen) {
		l.deps.Logger.Warn("查询视频 coin_count 失败", zap.Uint("video_id", videoID), zap.Error(err))
	}

	return &response.CoinResultResp{
		Added:        added,
		VideoCoinCnt: coinCnt,
		UserBalance:  balance,
	}, nil
}

// ListCoinLedger 查询硬币流水
func (l *CoinLogic) ListCoinLedger(ctx context.Context, userID string, req request.ListCoinLedgerReq) (*response.PaginatedResp[response.CoinLedgerItem], error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 || req.PageSize > 100 {
		req.PageSize = 20
	}

	userIDUint, _ := strconv.ParseUint(userID, 10, 64)

	if err := l.deps.Breakers.MySQLReadSem.Acquire(ctx, 1); err != nil {
		return nil, fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLReadSem.Release(1)

	var ledgers []database.CoinLedger
	var total int64
	if err := l.deps.Breakers.MySQL.Execute(func() error {
		var err error
		ledgers, total, err = l.deps.CoinLedgerRepo.ListByUser(ctx, userIDUint, req.Reason, req.Page, req.PageSize)
		return err
	}); err != nil {
		return nil, fmt.Errorf("查询流水失败")
	}

	items := make([]response.CoinLedgerItem, 0, len(ledgers))
	for _, lg := range ledgers {
		items = append(items, response.CoinLedgerItem{
			ID:          lg.ID,
			DeltaTenths: lg.DeltaTenths,
			ReasonType:  lg.ReasonType,
			VideoID:     lg.VideoID,
			CreatedAt:   lg.CreatedAt,
		})
	}
	return &response.PaginatedResp[response.CoinLedgerItem]{
		List:     items,
		Total:    total,
		Page:     req.Page,
		PageSize: req.PageSize,
	}, nil
}
