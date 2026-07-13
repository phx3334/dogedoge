package mysql

import (
	"context"
	"errors"
	"time"

	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/repository/interfaces"

	"gorm.io/gorm"
)

var _ interfaces.UserDailyTaskRepository = (*UserDailyTaskRepo)(nil)

// UserDailyTaskRepo 每日任务数据存储
type UserDailyTaskRepo struct {
	db *gorm.DB
}

func NewUserDailyTaskRepo(db *gorm.DB) *UserDailyTaskRepo {
	return &UserDailyTaskRepo{db: db}
}

// FindOrCreateByUserDate 查询或创建某用户某天的任务记录
func (r *UserDailyTaskRepo) FindOrCreateByUserDate(ctx context.Context, userID uint64, taskDate string) (*database.UserDailyTask, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	task := database.UserDailyTask{
		UserID:   userID,
		TaskDate: taskDate,
	}
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND task_date = ?", userID, taskDate).
		FirstOrCreate(&task)
	if result.Error != nil {
		return nil, result.Error
	}
	return &task, nil
}

// MarkLoginDone 标记今日已访问（登录领奖）
// 仅当 LoginDone=false 时更新；返回 created=true 表示本次新增（首次今日访问）
func (r *UserDailyTaskRepo) MarkLoginDone(ctx context.Context, userID uint64, taskDate string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// 使用 FirstOrCreate 创建记录（如不存在），然后判断 LoginDone 标志
	task := database.UserDailyTask{
		UserID:   userID,
		TaskDate: taskDate,
	}
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND task_date = ?", userID, taskDate).
		FirstOrCreate(&task)
	if result.Error != nil {
		return false, result.Error
	}

	// 已存在且 LoginDone=true：本次未新增
	if task.LoginDone {
		return false, nil
	}

	// LoginDone=false → 更新为 true，标记为本次新增
	if err := r.db.WithContext(ctx).Model(&database.UserDailyTask{}).
		Where("id = ?", task.ID).
		Update("login_done", true).Error; err != nil {
		return false, err
	}
	return true, nil
}

// MarkWatchDone 标记今日已完成观看任务
func (r *UserDailyTaskRepo) MarkWatchDone(ctx context.Context, userID uint64, taskDate string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	task := database.UserDailyTask{
		UserID:   userID,
		TaskDate: taskDate,
	}
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND task_date = ?", userID, taskDate).
		FirstOrCreate(&task)
	if result.Error != nil {
		return false, result.Error
	}
	if task.WatchDone {
		return false, nil
	}
	if err := r.db.WithContext(ctx).Model(&database.UserDailyTask{}).
		Where("id = ?", task.ID).
		Update("watch_done", true).Error; err != nil {
		return false, err
	}
	return true, nil
}

// GetByUserDate 查询某用户某天的任务记录
// 不存在返回 nil + nil error（不视为错误）
func (r *UserDailyTaskRepo) GetByUserDate(ctx context.Context, userID uint64, taskDate string) (*database.UserDailyTask, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var task database.UserDailyTask
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND task_date = ?", userID, taskDate).
		First(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &task, nil
}
