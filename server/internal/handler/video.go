package handler

import (

	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/dto/response"
	"fake_tiktok/internal/logic"
	"fake_tiktok/internal/pkg"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type VideoHandler struct {
	logic  *logic.LogicGroup
	logger *zap.Logger
}

func (h *VideoHandler) ListPublishedVideos(c *gin.Context) {
	var req request.HomeVideoList
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMsg(c, "参数错误")
		return
	}

	list, total, nextCursor := h.logic.VideoLogic.ListHotVideos(req)

	response.OkWithData(c, gin.H{
		"list":        list,
		"total":       total,
		"next_cursor": nextCursor,
		"has_more":    nextCursor != "",
	})
}

func (h *VideoHandler) GetVideoDetail(c *gin.Context) {
	var req request.VideoDetailReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMsg(c, "参数错误")
		return
	}

	userID := pkg.GetUserID(c, h.logic.Config)
	data, err := h.logic.VideoLogic.GetVideoDetail(c.Request.Context(), userID, req.VideoID)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}

	response.OkWithData(c, data)
}
