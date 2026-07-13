package handler

import (
	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/dto/response"
	"fake_tiktok/internal/logic"
	"fake_tiktok/internal/pkg"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// FavoriteFolderHandler 收藏夹管理 HTTP 处理层
type FavoriteFolderHandler struct {
	logic  *logic.LogicGroup
	logger *zap.Logger
}

// ListFolders 列出当前用户的所有收藏夹
//
// 路由：GET /favorite/folders（私有组，需登录）
// 响应：[]FavoriteFolderDetailResp
func (h *FavoriteFolderHandler) ListFolders(c *gin.Context) {
	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	data, err := h.logic.FavoriteFolderLogic.ListFolders(c.Request.Context(), userID)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithData(c, data)
}

// CreateFolder 创建收藏夹
//
// 路由：POST /favorite/folder（私有组，需登录）
// 请求体：CreateFavoriteFolderReq
// 响应：{folder_id: uint64}
func (h *FavoriteFolderHandler) CreateFolder(c *gin.Context) {
	var req request.CreateFavoriteFolderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, "参数错误：标题必填且最长 20 字符")
		return
	}

	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	folderID, err := h.logic.FavoriteFolderLogic.CreateFolder(c.Request.Context(), userID, req)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithData(c, gin.H{"folder_id": folderID})
}

// UpdateFolder 更新收藏夹标题/封面
//
// 路由：PUT /favorite/folder（私有组，需登录）
// 请求体：UpdateFavoriteFolderReq
func (h *FavoriteFolderHandler) UpdateFolder(c *gin.Context) {
	var req request.UpdateFavoriteFolderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, "参数错误")
		return
	}

	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	if err := h.logic.FavoriteFolderLogic.UpdateFolder(c.Request.Context(), userID, req); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithMsg(c, "更新成功")
}

// DeleteFolder 删除收藏夹
//
// 路由：DELETE /favorite/folder（私有组，需登录）
// 请求体：DeleteFavoriteFolderReq
func (h *FavoriteFolderHandler) DeleteFolder(c *gin.Context) {
	var req request.DeleteFavoriteFolderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, "参数错误")
		return
	}

	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	if err := h.logic.FavoriteFolderLogic.DeleteFolder(c.Request.Context(), userID, req); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithMsg(c, "删除成功")
}

// ListFolderVideos 列出收藏夹中的视频
//
// 路由：GET /favorite/folder/videos（私有组，需登录）
// 查询参数：folder_id, page, page_size
// 响应：PaginatedResp[HomeVideoInfo]
func (h *FavoriteFolderHandler) ListFolderVideos(c *gin.Context) {
	var req request.ListFolderVideosReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMsg(c, "参数错误")
		return
	}

	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	data, err := h.logic.FavoriteFolderLogic.ListFolderVideos(c.Request.Context(), userID, req)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithData(c, data)
}

// MoveFavorite 移动收藏视频到指定收藏夹
//
// 路由：POST /favorite/move（私有组，需登录）
// 请求体：MoveFavoriteReq
func (h *FavoriteFolderHandler) MoveFavorite(c *gin.Context) {
	var req request.MoveFavoriteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, "参数错误")
		return
	}

	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	if err := h.logic.FavoriteFolderLogic.MoveFavorite(c.Request.Context(), userID, req); err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithMsg(c, "移动成功")
}
