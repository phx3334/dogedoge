package routers

import (
	"fake_tiktok/internal/handler"

	"github.com/gin-gonic/gin"
)

// CoinRouter 硬币相关路由注册器
type CoinRouter struct{}

// InitCoinRouter 注册硬币相关路由
//
// 路由分组：
//   - 私有组：POST /coin/video（视频投币）、GET /coin/ledger（硬币流水）
func (r *CoinRouter) InitCoinRouter(privateGroup *gin.RouterGroup, handlerGroup *handler.HandlerGroup) {
	coin := privateGroup.Group("coin")
	{
		coin.POST("video", handlerGroup.CoinHandler.CoinVideo)
		coin.GET("ledger", handlerGroup.CoinHandler.ListCoinLedger)
	}
}
