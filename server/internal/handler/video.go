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

// DeleteVideo 处理作者删除自己投稿的视频请求。
// 鉴权由 privateGroup 的 JWT 中间件保证；视频归属校验在 VideoLogic.DeleteVideo 内完成。
func (h *VideoHandler) DeleteVideo(c *gin.Context) {
	var req request.VideoDetailReq
	// 优先从 query 字符串读取（前端用 params 传参，与 detail 接口一致）；
	// 兜底再尝试 JSON/表单 body，避免参数放错位置直接报"参数错误"。
	if err := c.ShouldBindQuery(&req); err != nil {
		if err2 := c.ShouldBind(&req); err2 != nil {
			response.FailWithMsg(c, "参数错误")
			return
		}
	}

	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "请先登录")
		return
	}

	if err := h.logic.VideoLogic.DeleteVideo(c.Request.Context(), userID, req.VideoID); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}

	response.OkWithMsg(c, "删除成功")
}
