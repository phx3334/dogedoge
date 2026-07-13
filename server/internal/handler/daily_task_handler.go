package handler

import (
	"fake_tiktok/internal/dto/response"
	"fake_tiktok/internal/logic"
	"fake_tiktok/internal/pkg"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// DailyTaskHandler 每日任务 HTTP 处理层
type DailyTaskHandler struct {
	logic  *logic.LogicGroup
	logger *zap.Logger
}

// TriggerDailyLogin 触发每日登录奖励
//
// 路由：POST /daily/login（私有组，需登录）
// 响应：{rewarded: bool} rewarded=true 表示今日首次访问已发放奖励，false 表示今日已访问
func (h *DailyTaskHandler) TriggerDailyLogin(c *gin.Context) {
	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	rewarded, err := h.logic.DailyTaskLogic.TriggerDailyLogin(c.Request.Context(), userID)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithData(c, gin.H{"rewarded": rewarded})
}

// TriggerWatch 触发每日观看奖励
//
// 路由：POST /daily/watch（私有组，需登录）
// 响应：{rewarded: bool} rewarded=true 表示今日首次观看已发放奖励，false 表示今日已完成
func (h *DailyTaskHandler) TriggerWatch(c *gin.Context) {
	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	rewarded, err := h.logic.DailyTaskLogic.TriggerWatch(c.Request.Context(), userID)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithData(c, gin.H{"rewarded": rewarded})
}

// GetTodayTask 查询今日任务完成情况
//
// 路由：GET /daily/today（私有组，需登录）
// 响应：{level: UserLevelResp, task: UserDailyTask}
func (h *DailyTaskHandler) GetTodayTask(c *gin.Context) {
	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	levelInfo, task, todayExp, err := h.logic.DailyTaskLogic.GetTodayTask(c.Request.Context(), userID)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithData(c, gin.H{
		"level":     levelInfo,
		"task":      task,
		"today_exp": todayExp,
	})
}
