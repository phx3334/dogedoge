package handler

import (
	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/dto/response"
	"fake_tiktok/internal/logic"
	"fake_tiktok/internal/pkg"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// CoinHandler 硬币相关 HTTP 处理层（视频投币 / 硬币流水查询）
type CoinHandler struct {
	logic  *logic.LogicGroup
	logger *zap.Logger
}

// CoinVideo 视频投币
//
// 路由：POST /coin/video（私有组，需登录）
// 请求体：CoinVideoReq
// 响应：CoinResultResp
func (h *CoinHandler) CoinVideo(c *gin.Context) {
	var req request.CoinVideoReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, "参数错误：video_id 必填，amount 必须 1 或 2")
		return
	}

	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	data, err := h.logic.CoinLogic.CoinVideo(c.Request.Context(), userID, req)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithData(c, data)
}

// ListCoinLedger 查询硬币流水
//
// 路由：GET /coin/ledger（私有组，需登录）
// 查询参数：page, page_size, reason_type
// 响应：PaginatedResp[CoinLedgerItem]
func (h *CoinHandler) ListCoinLedger(c *gin.Context) {
	var req request.ListCoinLedgerReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMsg(c, "参数错误")
		return
	}

	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	data, err := h.logic.CoinLogic.ListCoinLedger(c.Request.Context(), userID, req)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithData(c, data)
}
