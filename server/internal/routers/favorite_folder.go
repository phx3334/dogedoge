package routers

import (
	"fake_tiktok/internal/handler"

	"github.com/gin-gonic/gin"
)

// FavoriteFolderRouter 收藏夹管理路由注册器
type FavoriteFolderRouter struct{}

// InitFavoriteFolderRouter 注册收藏夹管理相关路由
//
// 路由分组：
//   - 私有组（全部需登录）：
//   - GET    /favorite/folders         列出收藏夹
//   - POST   /favorite/folder          创建收藏夹
//   - PUT    /favorite/folder           更新收藏夹
//   - DELETE /favorite/folder           删除收藏夹
//   - GET    /favorite/folder/videos    收藏夹视频列表
//   - POST   /favorite/move             移动收藏
func (r *FavoriteFolderRouter) InitFavoriteFolderRouter(privateGroup *gin.RouterGroup, handlerGroup *handler.HandlerGroup) {
	fav := privateGroup.Group("favorite")
	{
		fav.GET("folders", handlerGroup.FavoriteFolderHandler.ListFolders)
		fav.POST("folder", handlerGroup.FavoriteFolderHandler.CreateFolder)
		fav.PUT("folder", handlerGroup.FavoriteFolderHandler.UpdateFolder)
		fav.DELETE("folder", handlerGroup.FavoriteFolderHandler.DeleteFolder)
		fav.GET("folder/videos", handlerGroup.FavoriteFolderHandler.ListFolderVideos)
		fav.POST("move", handlerGroup.FavoriteFolderHandler.MoveFavorite)
	}
}
