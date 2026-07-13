package logic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/dto/cache"
	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/dto/response"
	"fake_tiktok/internal/pkg/storage"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// allowedVideoExt 视频上传扩展名白名单（小写形式）。
// 与 spec 中"mp4/mov/avi/mkv/flv"保持一致；
// ffmpeg 转码阶段会统一输出 H.264 MP4，所以原始格式仅做扩展名白名单过滤即可。
var allowedVideoExt = map[string]bool{
	".mp4": true,
	".mov": true,
	".avi": true,
	".mkv": true,
	".flv": true,
}

// allowedCoverExt 封面上传扩展名白名单（小写形式）。
// 与 UserLogic.UploadAvatar 的图片白名单保持一致，便于前端复用上传组件。
var allowedCoverExt = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".webp": true,
}

// VideoDraftLogic 视频草稿上传业务编排层
//
// 职责：
//   - UploadDraft：校验文件大小/扩展名 → 原子写入临时目录 → 插入 status=draft 记录
//     → 发布 TranscodeMsg 触发异步转码
//   - GetStatus：按 video_id 查询当前状态供前端轮询，校验调用者为视频作者
//
// 与 VideoLogic 的区别：本结构只承接草稿链路；已发布视频的详情、热度榜等仍由 VideoLogic 维护。
type VideoDraftLogic struct {
	deps    *LogicDeps
	storage storage.Storage
}

// NewVideoDraftLogic 构造一个 VideoDraftLogic 实例。
//
// storage 由调用方（initialize/router.go）通过 storage.NewStorage 注入，
// 这样 logic 不依赖具体 LocalStorage/QiniuStorage 实现，便于测试与切流。
func NewVideoDraftLogic(deps *LogicDeps, storage storage.Storage) *VideoDraftLogic {
	return &VideoDraftLogic{deps: deps, storage: storage}
}

// UploadDraft 接收视频草稿上传，原子写入临时目录并发布转码任务。
//
// 流程：
//  1. 校验视频文件大小 ≤ cfg.Upload.MaxFileSize
//  2. 校验视频扩展名 ∈ {mp4,mov,avi,mkv,flv}（小写比较）
//  3. 生成目标路径 {TempUploadDir}/{userID}/{userID}_{unixMilli}{ext}，先写 .tmp 再 rename
//     —— 写 .tmp + rename 是 POSIX 原子操作，避免半截文件被 worker 读到
//  4. 若提供了 cover：同样校验扩展名 + 原子写入 {TempUploadDir}/{userID}/{userID}_{unixMilli}_cover{coverExt}
//  5. tags []string → JSON 序列化为 TagsJSON 字段
//  6. 插入 Video{Status:"draft", AuthorID, DraftRawPath, DraftCoverPath, Title, Description, Zone, TagsJSON}
//  7. 发布 TranscodeMsg 到 mini_bili_transcode 队列；发布失败仅记日志不回滚 DB
//     —— worker 在未收到消息时不会卡住；如确需保证 exactly-once，可由运维通过 status=draft 重发
//  8. 返回 videoID 供前端轮询 status
//
// 错误约定（业务码字符串走 response.FailWithMsg）：
//   - "文件超过大小限制"
//   - "不支持的视频格式"
//   - "不支持的封面格式"
//   - "保存文件失败"
//   - "数据库写入失败"
func (l *VideoDraftLogic) UploadDraft(ctx context.Context, userID string, file *multipart.FileHeader, cover *multipart.FileHeader, meta request.VideoDraftUploadReq) (uint, error) {
	// 1. 校验视频文件大小
	if file.Size > l.deps.Config.Upload.MaxFileSize {
		return 0, errors.New("文件超过大小限制")
	}

	// 2. 校验视频扩展名
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !allowedVideoExt[ext] {
		return 0, errors.New("不支持的视频格式")
	}

	// 3. 准备目标目录：{TempUploadDir}/{userID}/
	userDir := filepath.Join(l.deps.Config.Upload.TempUploadDir, userID)
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		return 0, fmt.Errorf("保存文件失败: %w", err)
	}

	// 4. 原子写入视频文件
	// 文件名采用 {userID}_{unixMilli}{ext}，避免与 user.go 的 snowNode 耦合；
	// unixMilli + userID 的组合对单用户来说足够区分，不会出现同名覆盖
	videoFilename := fmt.Sprintf("%s_%d%s", userID, time.Now().UnixMilli(), ext)
	videoSavePath := filepath.Join(userDir, videoFilename)
	if err := atomicWriteFile(file, videoSavePath); err != nil {
		return 0, fmt.Errorf("保存文件失败: %w", err)
	}

	// 5. 若提供了封面，同样原子写入
	var coverSavePath string
	if cover != nil {
		coverExt := strings.ToLower(filepath.Ext(cover.Filename))
		if !allowedCoverExt[coverExt] {
			// 清理刚写入的视频文件，避免悬挂文件
			_ = os.Remove(videoSavePath)
			return 0, errors.New("不支持的封面格式")
		}
		coverFilename := fmt.Sprintf("%s_%d_cover%s", userID, time.Now().UnixMilli(), coverExt)
		coverSavePath = filepath.Join(userDir, coverFilename)
		if err := atomicWriteFile(cover, coverSavePath); err != nil {
			// 封面写入失败：清理视频文件
			_ = os.Remove(videoSavePath)
			return 0, fmt.Errorf("保存文件失败: %w", err)
		}
	}

	// 6. tags → JSON
	var tagsJSON string
	if len(meta.Tags) > 0 {
		body, err := json.Marshal(meta.Tags)
		if err != nil {
			// 序列化失败：清理已写入的文件
			_ = os.Remove(videoSavePath)
			if coverSavePath != "" {
				_ = os.Remove(coverSavePath)
			}
			return 0, fmt.Errorf("保存文件失败: %w", err)
		}
		tagsJSON = string(body)
	}

	// 7. 插入 Video 草稿记录
	video := &database.Video{
		Status:         "draft",
		AuthorID:       userID,
		DraftRawPath:   videoSavePath,
		DraftCoverPath: coverSavePath,
		Title:          meta.Title,
		Description:    meta.Description,
		Zone:           meta.Zone,
		TagsJSON:       tagsJSON,
		// PlayURL/CoverURL 留空，由 worker 转码完成后回写
	}
	if err := l.deps.VideoDraftRepo.CreateDraft(ctx, video); err != nil {
		// DB 写入失败：清理磁盘文件，避免悬挂
		_ = os.Remove(videoSavePath)
		if coverSavePath != "" {
			_ = os.Remove(coverSavePath)
		}
		return 0, fmt.Errorf("数据库写入失败: %w", err)
	}

	// 8. 发布转码消息到 RabbitMQ
	// 失败处理：仅记日志，不回滚 DB —— worker 在收不到消息时不会卡住，
	// 运维可通过 status=draft 但长时间未 transcoding 的视频识别并重发
	msg := cache.TranscodeMsg{
		VideoID:        video.ID,
		DraftRawPath:   videoSavePath,
		DraftCoverPath: coverSavePath,
		UserID:         userID,
	}
	if err := l.deps.TranscodePublisher.Publish(ctx, msg); err != nil {
		// PublishBuffer 内部会入缓冲并自动重放，这里拿到的 err 只是
		// "未能直发"的信号，不构成致命错误；记日志即可
		if l.deps.Logger != nil {
			l.deps.Logger.Warn("transcode publish buffered or failed",
				zap.Uint("video_id", video.ID),
				zap.Error(err))
		}
	}

	return video.ID, nil
}

// GetStatus 查询视频草稿/转码状态，供前端轮询。
//
// 校验：
//   - videoID 必须存在，不存在视为业务错误
//   - 调用者 userID 必须等于 video.AuthorID，否则返回"无权限查看"
//
// 返回 response.VideoDraftStatusResp，映射关系：
//   - Status / FailReason 直接来自 DB
//   - VideoURL ← video.PlayURL（worker 转码完成时回写）
//   - CoverURL ← video.CoverURL（worker 转码完成时回写）
func (l *VideoDraftLogic) GetStatus(ctx context.Context, userID string, videoID uint) (response.VideoDraftStatusResp, error) {
	video, err := l.deps.VideoDraftRepo.FindDraftByID(ctx, videoID)
	if err != nil {
		// 区分"未找到"和"DB 错误"
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return response.VideoDraftStatusResp{}, errors.New("视频不存在")
		}
		return response.VideoDraftStatusResp{}, fmt.Errorf("查询失败: %w", err)
	}

	if video.AuthorID != userID {
		return response.VideoDraftStatusResp{}, errors.New("无权限查看")
	}

	return response.VideoDraftStatusResp{
		Status:     video.Status,
		FailReason: video.FailReason,
		VideoURL:   video.PlayURL,
		CoverURL:   video.CoverURL,
	}, nil
}

// atomicWriteFile 把 multipart.FileHeader 内容原子写入 savePath。
//
// 流程（与 UserLogic.UploadAvatar 一致）：
//  1. 打开上传文件 src
//  2. 创建 tmpPath = savePath + ".tmp"
//  3. io.Copy / ReadFrom 写入 tmpPath
//  4. Close 后 os.Rename(tmpPath, savePath) 原子提交
//  5. 失败时清理 tmpPath 残留
//
// 原子性保证：
//   - rename 是 POSIX 原子操作；外部观察者要么看到完整文件，要么看不到
//   - 避免 worker 在转码途中读到半截文件导致 ffmpeg 失败
func atomicWriteFile(file *multipart.FileHeader, savePath string) error {
	src, err := file.Open()
	if err != nil {
		return fmt.Errorf("open upload: %w", err)
	}
	defer src.Close()

	tmpPath := savePath + ".tmp"
	dst, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create tmp file: %w", err)
	}
	// renamed 用于 defer 中判断是否需要清理 tmp 残留：
	//   rename 成功后 tmpPath 已不存在，不能再次删除；
	//   rename 失败或前面任意步骤出错，tmpPath 仍是孤立临时文件，必须清理。
	renamed := false
	defer func() {
		// 先关闭文件句柄（即使写入失败也要确保 Close 不会泄漏 fd）。
		_ = dst.Close()
		// 仅在 rename 未成功时清理临时文件。
		if !renamed {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := dst.ReadFrom(src); err != nil {
		return fmt.Errorf("write tmp file: %w", err)
	}

	// 显式关闭 dst，确保数据已 flush 到磁盘，再执行 rename。
	if cerr := dst.Close(); cerr != nil {
		return fmt.Errorf("close tmp file: %w", cerr)
	}

	// 原子提交：rename 后外部观察者只会看到完整文件或不存在。
	if err := os.Rename(tmpPath, savePath); err != nil {
		return fmt.Errorf("commit file: %w", err)
	}
	renamed = true
	return nil
}
