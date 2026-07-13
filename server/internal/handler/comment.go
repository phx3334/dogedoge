package handler

import (
	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/dto/response"
	"fake_tiktok/internal/logic"
	"fake_tiktok/internal/pkg"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// CommentHandler 评论相关 HTTP 处理器
//
// 统一处理视频/文章/动态三类目标的评论操作，
// 通过 target_type 参数路由到对应逻辑。
type CommentHandler struct {
	logic  *logic.LogicGroup
	logger *zap.Logger
}

func NewCommentHandler(logic *logic.LogicGroup, logger *zap.Logger) *CommentHandler {
	return &CommentHandler{
		logic:  logic,
		logger: logger,
	}
}

// CreateComment 创建评论或回复
func (h *CommentHandler) CreateComment(c *gin.Context) {
	var req request.CreateCommentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}

	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	commentID, err := h.logic.CommentLogic.CreateComment(c.Request.Context(), userID, req)
	if err != nil {
		h.logger.Error("创建评论失败", zap.Error(err))
		response.FailWithMsg(c, err.Error())
		return
	}

	response.OkWithData(c, gin.H{"comment_id": commentID})
}

// ListComments 查询评论列表
func (h *CommentHandler) ListComments(c *gin.Context) {
	var req request.ListCommentsReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}

	list, total, err := h.logic.CommentLogic.ListComments(c.Request.Context(), req)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}

	response.OkWithData(c, gin.H{
		"list":  list,
		"total": total,
	})
}

// ListReplies 查询评论回复列表
func (h *CommentHandler) ListReplies(c *gin.Context) {
	var req request.ListRepliesReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}

	list, total, err := h.logic.CommentLogic.ListReplies(c.Request.Context(), req)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}

	response.OkWithData(c, gin.H{
		"list":  list,
		"total": total,
	})
}

// LikeComment 点赞评论
func (h *CommentHandler) LikeComment(c *gin.Context) {
	var req request.CommentLikeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}

	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	if err := h.logic.CommentLogic.LikeComment(c.Request.Context(), userID, req); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}

	response.OkWithMsg(c, "点赞成功")
}

// UnlikeComment 取消点赞评论
func (h *CommentHandler) UnlikeComment(c *gin.Context) {
	var req request.CommentLikeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}

	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	if err := h.logic.CommentLogic.UnlikeComment(c.Request.Context(), userID, req); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}

	response.OkWithMsg(c, "取消点赞成功")
}

// DeleteComment 删除评论
func (h *CommentHandler) DeleteComment(c *gin.Context) {
	var req request.DeleteCommentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}

	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	if err := h.logic.CommentLogic.DeleteComment(c.Request.Context(), userID, req); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}

	response.OkWithMsg(c, "删除成功")
}
