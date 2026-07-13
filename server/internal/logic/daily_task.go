package logic

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"fake_tiktok/internal/breaker"
	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/dto/response"
	"fake_tiktok/internal/repository/interfaces"

	"go.uber.org/zap"
)

// DailyTaskLogic 每日任务业务逻辑
//
// 每日任务设计：
//   - 用户每天访问网站 +10 经验（一次/天）
//   - 投币 1 个 +20 经验（投币操作触发，不在这里实现）
//   - 评论 1 次 +5 经验（评论操作触发，不在这里实现）
//
// 这里仅负责"每日访问"任务：用户首次调用此接口时 +10 经验并标记今日已完成。
type DailyTaskLogic struct {
	deps *LogicDeps
}

func NewDailyTaskLogic(deps *LogicDeps) *DailyTaskLogic {
	return &DailyTaskLogic{deps: deps}
}

// TriggerDailyLogin 触发每日登录奖励
// 用户首次访问当天调用此接口；幂等：当天再次调用不重复发经验
//
// 流程：
//  1. 通过 MarkLoginDone 标记今日已完成（仅当 LoginDone=false 时返回 created=true）
//  2. created=true 时 +10 经验到用户账户
//  3. 写入 CoinLedger 流水（reason="daily_login"，delta=+5 tenths = 0.5 硬币）
//     注：经验 +10，硬币 +0.5（保留小奖励）—— 实际项目根据需求调整
//  4. 同时如果今日完成观看任务（WatchDone）则 +5 硬币流水（已由其他逻辑触发，此处不重复）
//
// 返回 created=true 表示今日首次访问并已发放奖励；false 表示今日已访问过
func (l *DailyTaskLogic) TriggerDailyLogin(ctx context.Context, userID string) (bool, error) {
	userIDUint, _ := strconv.ParseUint(userID, 10, 64)
	if userIDUint == 0 {
		return false, errors.New("invalid user id")
	}

	taskDate := todayInShanghai()

	if err := l.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); err != nil {
		return false, fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLWriteSem.Release(1)

	var created bool
	if err := l.deps.Breakers.MySQL.Execute(func() error {
		var err error
		created, err = l.deps.DailyTaskRepo.MarkLoginDone(ctx, userIDUint, taskDate)
		return err
	}); err != nil {
		if errors.Is(err, breaker.ErrCircuitOpen) {
			l.deps.Logger.Warn("MySQL circuit open during TriggerDailyLogin",
				zap.String("user_id", userID))
		}
		return false, fmt.Errorf("记录每日任务失败")
	}

	if !created {
		// 今日已完成登录任务，幂等返回
		return false, nil
	}

	// +10 经验
	if err := l.deps.Breakers.MySQL.Execute(func() error {
		return l.deps.AccountRepo.AddExperience(ctx, userID, int64(interfaces.ExpPerLogin))
	}); err != nil {
		l.deps.Logger.Warn("每日登录经验发放失败",
			zap.String("user_id", userID), zap.Error(err))
	}
	// 同步缓存经验（best-effort）
	_ = l.deps.Breakers.Redis.Execute(func() error {
		return l.deps.UserCacheRepo.IncrementExperience(ctx, userID, int64(interfaces.ExpPerLogin))
	})

	// 写入流水（reason=daily_login，delta=+5 tenths = +0.5 硬币）
	ledger := &database.CoinLedger{
		UserID:      userIDUint,
		DeltaTenths: 5,
		ReasonType:  "daily_login",
		VideoID:     0,
		CreatedAt:   time.Now(),
	}
	if err := l.deps.Breakers.MySQL.Execute(func() error {
		return l.deps.CoinLedgerRepo.Create(ctx, ledger)
	}); err != nil {
		l.deps.Logger.Warn("每日登录流水写入失败",
			zap.String("user_id", userID), zap.Error(err))
	}

	return true, nil
}

// TriggerCommentReward 评论奖励触发器
// 由评论模块在创建评论成功后调用：每次评论 +5 经验
// 简化处理：不限制每日次数（如需限制可在此扩展）
func (l *DailyTaskLogic) TriggerCommentReward(ctx context.Context, userID string) {
	if err := l.deps.Breakers.MySQL.Execute(func() error {
		return l.deps.AccountRepo.AddExperience(ctx, userID, int64(interfaces.ExpPerComment))
	}); err != nil {
		l.deps.Logger.Warn("评论经验发放失败",
			zap.String("user_id", userID), zap.Error(err))
	}
	// 同步缓存经验（best-effort）
	_ = l.deps.Breakers.Redis.Execute(func() error {
		return l.deps.UserCacheRepo.IncrementExperience(ctx, userID, int64(interfaces.ExpPerComment))
	})
}

// TriggerWatch 触发每日观看奖励
// 用户首次进入视频详情页调用此接口；幂等：当天再次调用不重复发经验
// 流程：
//  1. 通过 MarkWatchDone 标记今日已完成（仅当 WatchDone=false 时返回 created=true）
//  2. created=true 时 +5 经验到用户账户
//
// 返回 created=true 表示今日首次观看并已发放奖励；false 表示今日已完成
func (l *DailyTaskLogic) TriggerWatch(ctx context.Context, userID string) (bool, error) {
	userIDUint, _ := strconv.ParseUint(userID, 10, 64)
	if userIDUint == 0 {
		return false, errors.New("invalid user id")
	}

	taskDate := todayInShanghai()

	if err := l.deps.Breakers.MySQLWriteSem.Acquire(ctx, 1); err != nil {
		return false, fmt.Errorf("服务繁忙，请稍后重试")
	}
	defer l.deps.Breakers.MySQLWriteSem.Release(1)

	var created bool
	if err := l.deps.Breakers.MySQL.Execute(func() error {
		var err error
		created, err = l.deps.DailyTaskRepo.MarkWatchDone(ctx, userIDUint, taskDate)
		return err
	}); err != nil {
		return false, fmt.Errorf("记录每日任务失败")
	}

	if !created {
		// 今日已完成观看任务，幂等返回
		return false, nil
	}

	// +5 经验
	if err := l.deps.Breakers.MySQL.Execute(func() error {
		return l.deps.AccountRepo.AddExperience(ctx, userID, int64(interfaces.ExpPerWatch))
	}); err != nil {
		l.deps.Logger.Warn("每日观看经验发放失败",
			zap.String("user_id", userID), zap.Error(err))
	}
	// 同步缓存经验（best-effort）
	_ = l.deps.Breakers.Redis.Execute(func() error {
		return l.deps.UserCacheRepo.IncrementExperience(ctx, userID, int64(interfaces.ExpPerWatch))
	})

	return true, nil
}

// GetTodayTask 查询今日任务完成情况
func (l *DailyTaskLogic) GetTodayTask(ctx context.Context, userID string) (*response.UserLevelResp, *database.UserDailyTask, uint64, error) {
	userIDUint, _ := strconv.ParseUint(userID, 10, 64)
	taskDate := todayInShanghai()

	task, err := l.deps.DailyTaskRepo.GetByUserDate(ctx, userIDUint, taskDate)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("查询任务失败")
	}

	// 查询用户经验/等级
	account, err := l.deps.AccountRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("查询用户信息失败")
	}
	level := interfaces.CalcLevel(account.Experience)
	levelInfo := &response.UserLevelResp{
		Level:           level,
		Experience:      account.Experience,
		CurrentLevelExp: interfaces.LevelBaseExp(level),
		MaxLevelExp:     interfaces.LevelThresholds[interfaces.LevelMax-1],
		IsMaxLevel:      level >= interfaces.LevelMax,
	}
	if levelInfo.Level < interfaces.LevelMax {
		levelInfo.NextLevelExp = interfaces.LevelThresholds[levelInfo.Level-1]
	}

	// 今日已获任务经验：按已完成的每日任务累加（评论/投币为按次奖励，不在此统计）
	var todayExp uint64
	if task != nil {
		if task.LoginDone {
			todayExp += uint64(interfaces.ExpPerLogin)
		}
		if task.WatchDone {
			todayExp += uint64(interfaces.ExpPerWatch)
		}
	}

	return levelInfo, task, todayExp, nil
}

// todayInShanghai 返回 Asia/Shanghai 时区的今日日期字符串 (YYYY-MM-DD)
//
// 用于每日任务按上海时区切日：
//   - 上海时区 UTC+8
//   - 凌晨 0 点 ~ 8 点的请求仍归前一日（北京时间 0 点后即新一天）
//
// 简化：使用 UTC 时间 +8 小时近似（不考虑闰秒等）。生产环境建议使用时间库精确处理。
func todayInShanghai() string {
	// 上海时区：UTC+8
	loc := time.FixedZone("CST", 8*3600)
	return time.Now().In(loc).Format("2006-01-02")
}
