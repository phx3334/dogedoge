package routers

import (
	"fake_tiktok/internal/handler"

	"github.com/gin-gonic/gin"
)

// DailyTaskRouter 每日任务路由注册器
type DailyTaskRouter struct{}

// InitDailyTaskRouter 注册每日任务相关路由
//
// 路由分组（全部私有组，需登录）：
//   - POST /daily/login  触发每日登录奖励
//   - GET  /daily/today  查询今日任务完成情况
func (r *DailyTaskRouter) InitDailyTaskRouter(privateGroup *gin.RouterGroup, handlerGroup *handler.HandlerGroup) {
	daily := privateGroup.Group("daily")
	{
		daily.POST("login", handlerGroup.DailyTaskHandler.TriggerDailyLogin)
		daily.POST("watch", handlerGroup.DailyTaskHandler.TriggerWatch)
		daily.GET("today", handlerGroup.DailyTaskHandler.GetTodayTask)
	}
}
