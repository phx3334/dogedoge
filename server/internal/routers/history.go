package routers

import (
	"fake_tiktok/internal/handler"

	"github.com/gin-gonic/gin"
)

// HistoryRouter 用户历史路由注册器
type HistoryRouter struct{}

// InitHistoryRouter 注册历史相关路由
//
// 路由分组（全部私有组，需登录）：
//   - 视频观看历史：
//     POST   /history/video/view     记录观看进度
//     GET    /history/video/list     观看历史列表
//     DELETE /history/video          删除单条
//     POST   /history/video/clear    清空
//   - 文章阅读历史：
//     GET    /history/article/list   阅读历史列表
//     DELETE /history/article        删除单条
//   - 搜索历史：
//     POST   /history/search         保存搜索
//     GET    /history/search/list     搜索历史列表
//     DELETE /history/search         删除单条
//     POST   /history/search/clear   清空
func (r *HistoryRouter) InitHistoryRouter(privateGroup *gin.RouterGroup, handlerGroup *handler.HandlerGroup) {
	history := privateGroup.Group("history")
	{
		// 视频观看历史
		video := history.Group("video")
		{
			video.POST("view", handlerGroup.HistoryHandler.RecordVideoView)
			video.GET("list", handlerGroup.HistoryHandler.ListVideoHistory)
			video.DELETE("", handlerGroup.HistoryHandler.DeleteVideoHistory)
			video.POST("clear", handlerGroup.HistoryHandler.ClearVideoHistory)
		}

		// 文章阅读历史
		article := history.Group("article")
		{
			article.GET("list", handlerGroup.HistoryHandler.ListArticleHistory)
			article.DELETE("", handlerGroup.HistoryHandler.DeleteArticleHistory)
		}

		// 搜索历史
		search := history.Group("search")
		{
			search.POST("", handlerGroup.HistoryHandler.SaveSearchHistory)
			search.GET("list", handlerGroup.HistoryHandler.ListSearchHistory)
			search.DELETE("", handlerGroup.HistoryHandler.DeleteSearchHistory)
			search.POST("clear", handlerGroup.HistoryHandler.ClearSearchHistory)
		}
	}
}
