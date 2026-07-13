package handler

import (
	"context"
	"strconv"
	"time"

	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/dto/response"
	"fake_tiktok/internal/logic"
	"fake_tiktok/internal/pkg"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// VideoDraftHandler 视频草稿上传与转码状态查询 handler
//
// 与 VideoHandler 的区别：
//   - VideoHandler 承接已发布视频的列表、详情等读路径
//   - VideoDraftHandler 承接 draft 上传写路径与 status 轮询
//
// 路由注册由 VideoRouter.InitVideoRouter 中的 privateGroup 完成（Phase 5 接入）：
//   - POST /video/draft/upload  → UploadDraft
//   - GET  /video/draft/status  → GetStatus
type VideoDraftHandler struct {
	logic  *logic.LogicGroup
	logger *zap.Logger
}

// UploadDraft 处理 multipart/form-data 视频草稿上传请求。
//
// 表单字段：
//   - file（必填，文件）：视频文件，扩展名 ∈ {mp4,mov,avi,mkv,flv}
//   - cover（可选，文件）：封面图片，扩展名 ∈ {jpg,jpeg,png,gif,webp}
//   - title（必填，字符串）：视频标题
//   - description（可选，字符串）：视频简介
//   - zone（可选，字符串）：分区
//   - tags（可选，字符串数组）：标签列表
//
// 返回：{"video_id": uint}，前端据此轮询 GetStatus 接口
func (h *VideoDraftHandler) UploadDraft(c *gin.Context) {
	// 1. 获取调用者 userID；JWTAuth 中间件已校验登录态
	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	// 2. 解析必填的视频文件
	file, err := c.FormFile("file")
	if err != nil {
		response.FailWithMsg(c, "缺少视频文件")
		return
	}

	// 3. 解析可选的封面文件；不存在时 FormFile 返回错误，忽略即可
	cover, _ := c.FormFile("cover")

	// 4. 绑定表单元数据到 request.VideoDraftUploadReq
	//    使用 ShouldBind 而非 ShouldBindQuery，因为 multipart 表单字段
	//    走 form tag；ShouldBind 会根据 Content-Type 自动选择 form/JSON
	var req request.VideoDraftUploadReq
	if err := c.ShouldBind(&req); err != nil {
		response.FailWithMsg(c, "参数错误")
		return
	}

	// 5. 调用 logic 完成原子写入 + DB 插入 + 消息发布
	// 设置 30 分钟超时：与前端 axios 超时对齐，超时后取消上传避免无限等待（支持 1GB 上传）
	uploadCtx, uploadCancel := context.WithTimeout(c.Request.Context(), 30*time.Minute)
	defer uploadCancel()
	videoID, err := h.logic.VideoDraftLogic.UploadDraft(uploadCtx, userID, file, cover, req)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}

	response.OkWithData(c, gin.H{"video_id": videoID})
}

// GetStatus 处理 GET /video/draft/status?video_id= 请求。
//
// 查询参数：
//   - video_id（必填，uint）：UploadDraft 返回的 video_id
//
// 返回：response.VideoDraftStatusResp（status / fail_reason / video_url / cover_url）
func (h *VideoDraftHandler) GetStatus(c *gin.Context) {
	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	videoIDStr := c.Query("video_id")
	if videoIDStr == "" {
		response.FailWithMsg(c, "缺少 video_id 参数")
		return
	}
	videoID, err := strconv.ParseUint(videoIDStr, 10, 64)
	if err != nil {
		response.FailWithMsg(c, "video_id 参数错误")
		return
	}

	data, err := h.logic.VideoDraftLogic.GetStatus(c.Request.Context(), userID, uint(videoID))
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}

	response.OkWithData(c, data)
}
