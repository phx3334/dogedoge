package interfaces

import (
	"context"

	"fake_tiktok/internal/domain/database"
)

// UserDailyTaskRepository 每日任务数据接口
//
// 一条记录表示某用户在某天的任务进度（按 Asia/Shanghai 时区切日）。
// 通过唯一索引 (user_id, task_date) 保证幂等。
type UserDailyTaskRepository interface {
	// FindOrCreateByUserDate 查询或创建某用户某天的任务记录
	// 不存在则创建新记录（默认 LoginDone=WatchDone=false）
	// 返回 *database.UserDailyTask，调用方据此判断今天是否已完成某任务
	FindOrCreateByUserDate(ctx context.Context, userID uint64, taskDate string) (*database.UserDailyTask, error)

	// MarkLoginDone 标记今日已访问（登录领奖）
	// 仅当 LoginDone=false 时更新；返回 created=true 表示本次新增（首次今日访问）
	MarkLoginDone(ctx context.Context, userID uint64, taskDate string) (created bool, err error)

	// MarkWatchDone 标记今日已完成观看任务
	// 仅当 WatchDone=false 时更新；返回 created=true 表示本次新增
	MarkWatchDone(ctx context.Context, userID uint64, taskDate string) (created bool, err error)

	// GetByUserDate 查询某用户某天的任务记录
	// 不存在返回 nil + nil error（不视为错误，调用方按未完成任务处理）
	GetByUserDate(ctx context.Context, userID uint64, taskDate string) (*database.UserDailyTask, error)
}

// 用户等级相关常量（基于需求：Lv1-6，经验阈值 50/200/500/1000/2500/5000）
// 这些是经验值上限：达到阈值后即升到下一级；满级 6 级不再升级。
const (
	LevelMax      = 6 // 满级
	LevelMin      = 1 // 初始等级
	ExpPerLogin   = 10
	ExpPerCoin    = 20
	ExpPerComment = 5
	ExpPerWatch   = 5
)

// LevelThresholds 各等级所需累计经验值上限（索引 0=Lv1, ..., 5=Lv6）
// 含义：达到 LevelThresholds[i] 即升至下一级（i+2）；Lv6 满级后超出 5000 的经验仍累计但不影响等级。
var LevelThresholds = [6]uint64{50, 200, 500, 1000, 2500, 5000}

// CalcLevel 根据 Experience 计算用户等级（1..LevelMax）
//   - exp < 50            → Lv1
//   - 50 <= exp < 200     → Lv2
//   - 200 <= exp < 500    → Lv3
//   - 500 <= exp < 1000   → Lv4
//   - 1000 <= exp < 2500  → Lv5
//   - 2500 <= exp         → Lv6（满级）
func CalcLevel(exp uint64) int {
	for i, threshold := range LevelThresholds {
		// Lv6 满级：达到最高阈值后不再升级
		if i == LevelMax-1 {
			return LevelMax
		}
		if exp < threshold {
			return i + 1
		}
	}
	return LevelMax
}

// LevelBaseExp 返回升到当前等级所需的累计经验起点（Lv1 为 0）。
// 例如 Lv3（阈值 500）的上一档为 Lv2（阈值 200），故 base=200。
func LevelBaseExp(level int) uint64 {
	if level <= 1 {
		return 0
	}
	// level-2 是上一等级的索引（LevelThresholds[0]=Lv1 阈值 50）
	idx := level - 2
	if idx < 0 || idx >= len(LevelThresholds) {
		return 0
	}
	return LevelThresholds[idx]
}
