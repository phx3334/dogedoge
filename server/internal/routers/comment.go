package routers

import (
	"fake_tiktok/internal/handler"

	"github.com/gin-gonic/gin"
)

// CommentRouter 评论路由
type CommentRouter struct{}

// InitCommentRouter 注册评论相关路由
//
// 公开路由（无需登录）：list / replies
// 私有路由（需登录）：create / like / unlike / delete
func (cr *CommentRouter) InitCommentRouter(publicGroup, privateGroup *gin.RouterGroup, handlerGroup *handler.HandlerGroup) {
	// 公开路由：评论列表和回复列表无需登录
	commentPublic := publicGroup.Group("comment")
	{
		commentPublic.GET("list", handlerGroup.CommentHandler.ListComments)
		commentPublic.GET("replies", handlerGroup.CommentHandler.ListReplies)
	}

	// 私有路由：创建/点赞/取消点赞/删除需要登录
	commentPrivate := privateGroup.Group("comment")
	{
		commentPrivate.POST("create", handlerGroup.CommentHandler.CreateComment)
		commentPrivate.POST("like", handlerGroup.CommentHandler.LikeComment)
		commentPrivate.POST("unlike", handlerGroup.CommentHandler.UnlikeComment)
		commentPrivate.POST("delete", handlerGroup.CommentHandler.DeleteComment)
	}
}
