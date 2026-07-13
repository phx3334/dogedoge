package handler

import (
	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/dto/response"
	"fake_tiktok/internal/logic"
	"strconv"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"fake_tiktok/internal/pkg"
)

type InteractionHandler struct {
	logic  *logic.LogicGroup
	logger *zap.Logger
}

func NewInteractionHandler(logic *logic.LogicGroup, logger *zap.Logger) *InteractionHandler {
	return &InteractionHandler{
		logic:  logic,
		logger: logger,
	}
}

func (i *InteractionHandler) LikeVideo(c *gin.Context) {
	var req request.InteractionVideo
	err := c.ShouldBindJSON(&req)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	// 修复：使用 JWT 中提取的 userID，忽略请求体中的 user_id
	// 防止用户伪造请求体给其他人点赞
	req.UserID = pkg.GetUserID(c, i.logic.Config)
	if req.UserID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}
	err = i.logic.InteractionLogic.LikeVideo(c.Request.Context(), req)
	if err != nil {
		i.logger.Error("Failed to like video:", zap.Error(err))
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithMsg(c, "Successfully like video")
}

// UnlikeVideo 取消点赞视频
func (i *InteractionHandler) UnlikeVideo(c *gin.Context) {
	var req request.InteractionVideo
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	req.UserID = pkg.GetUserID(c, i.logic.Config)
	if req.UserID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}
	if err := i.logic.InteractionLogic.UnlikeVideo(c.Request.Context(), req); err != nil {
		i.logger.Error("Failed to unlike video:", zap.Error(err))
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithMsg(c, "Successfully unlike video")
}

// FavoriteVideo 收藏视频到指定收藏夹
func (i *InteractionHandler) FavoriteVideo(c *gin.Context) {
	var req request.FavoriteVideoReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	userID := pkg.GetUserID(c, i.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}
	if err := i.logic.InteractionLogic.FavoriteVideo(c.Request.Context(), userID, req); err != nil {
		i.logger.Error("Failed to favorite video:", zap.Error(err))
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithMsg(c, "收藏成功")
}

// UnfavoriteVideo 取消收藏视频
func (i *InteractionHandler) UnfavoriteVideo(c *gin.Context) {
	var req request.UnfavoriteVideoReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	userID := pkg.GetUserID(c, i.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}
	if err := i.logic.InteractionLogic.UnfavoriteVideo(c.Request.Context(), userID, req); err != nil {
		i.logger.Error("Failed to unfavorite video:", zap.Error(err))
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithMsg(c, "取消收藏成功")
}

// FollowUser 关注用户
func (i *InteractionHandler) FollowUser(c *gin.Context) {
	var req request.FollowUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	followerID := pkg.GetUserID(c, i.logic.Config)
	if followerID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}
	if err := i.logic.InteractionLogic.FollowUser(c.Request.Context(), followerID, req.TargetUserID); err != nil {
		i.logger.Error("Failed to follow user:", zap.Error(err))
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithMsg(c, "关注成功")
}

// UnfollowUser 取关用户
func (i *InteractionHandler) UnfollowUser(c *gin.Context) {
	var req request.FollowUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	followerID := pkg.GetUserID(c, i.logic.Config)
	if followerID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}
	if err := i.logic.InteractionLogic.UnfollowUser(c.Request.Context(), followerID, req.TargetUserID); err != nil {
		i.logger.Error("Failed to unfollow user:", zap.Error(err))
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithMsg(c, "取关成功")
}

func (i *InteractionHandler) GetDanmakuList(c *gin.Context) {
	videoIDStr := c.Query("video_id")
	videoID, err := strconv.ParseUint(videoIDStr, 10, 64)
	if err != nil || videoID == 0 {
		response.FailWithMsg(c, "参数错误")
		return
	}

	data, err := i.logic.DanmakuLogic.GetDanmakuList(c.Request.Context(), videoID)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}

	response.OkWithData(c, data)
}

func (i *InteractionHandler) SendDanmaku(c *gin.Context) {
	var req request.SendDanmakuReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}

	userID := pkg.GetUserID(c, i.logic.Config)
	if err := i.logic.DanmakuLogic.SendDanmaku(c.Request.Context(), userID, uint(req.VideoID), req); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}

	response.OkWithMsg(c, "发送成功")
}
