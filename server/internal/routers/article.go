package routers

import (
	"fake_tiktok/internal/handler"

	"github.com/gin-gonic/gin"
)

// ArticleRouter 专栏文章路由注册器。
type ArticleRouter struct{}

// InitArticleRouter 注册文章相关路由。
//
// 路由分组：
//   - 公开组（publicGroup）：GET /article/detail —— 详情无需登录
//   - 私有组（privateGroup）：POST /article/draft、POST /article/publish —— 草稿/发布需要登录
func (ar *ArticleRouter) InitArticleRouter(publicGroup, privateGroup *gin.RouterGroup, handlerGroup *handler.HandlerGroup) {
	// 公开路由：详情无需登录
	articlePublic := publicGroup.Group("article")
	{
		articlePublic.GET("detail", handlerGroup.ArticleHandler.GetArticleDetail)
	}

	// 私有路由：草稿/发布需要登录
	articlePrivate := privateGroup.Group("article")
	{
		articlePrivate.POST("draft", handlerGroup.ArticleHandler.SaveDraft)
		articlePrivate.POST("publish", handlerGroup.ArticleHandler.PublishArticle)
	}
}
