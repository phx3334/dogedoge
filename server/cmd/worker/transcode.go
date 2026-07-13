package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fake_tiktok/internal/config"
	"fake_tiktok/internal/domain/database"
	es "fake_tiktok/internal/domain/es"
	"fake_tiktok/internal/dto/cache"
	"fake_tiktok/internal/pkg/storage"
	"fake_tiktok/internal/repository/interfaces"
	"fake_tiktok/internal/repository/rabbitmq"
	redis_repo "fake_tiktok/internal/repository/redis"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// transcodeConsumeLoop 是转码任务消费者的重订阅循环。
//
// 与 consumeLoop / userConsumeLoop / likeConsumeLoop 模式完全一致：
//   - 外层循环：conn.WaitReady → QueueDeclare → Qos → Consume → 内层消息处理
//   - 内层循环：for msg := range msgs，每条消息解析为 cache.TranscodeMsg 后调用 transcodeOne
//   - 消息处理成功 / 失败均 Ack（避免死循环重试）；仅 JSON 解析失败时 Nack(false, false) 丢弃
//   - ctx 取消时把未处理的消息 Nack(requeue=true) 让 broker 重新投递
//
// 为什么转码失败也 Ack：
//   - ffmpeg 失败通常是输入文件损坏 / 编码不支持等不可恢复原因
//   - 重试只会反复失败消耗 CPU
//   - 失败信息已写入 videos.fail_reason，由用户决定是否重新上传
//
// 为什么 JSON 解析失败时 Nack(false, false)（不 requeue）：
//   - 消息体损坏重试也是损坏，没意义
//   - false, false 表示丢弃（broker 默认无死信队列时直接丢弃）
func transcodeConsumeLoop(ctx context.Context, conn *rabbitmq.Connection, db *gorm.DB, storageInstance storage.Storage, ffmpegCfg config.FFmpegConfig, tempDir string, videoSearchRepo interfaces.VideoSearchRepository, logger *zap.Logger) {
	for {
		// 1. 阻塞等连接可用：MQ 启动 / 重连期间这里会短暂阻塞
		ch, err := conn.WaitReady(ctx)
		if err != nil {
			if ctx.Err() != nil {
				// 主流程已收信号：正常退出
				return
			}
			logger.Warn("transcode consume: wait ready failed", zap.Error(err))
			time.Sleep(time.Second)
			continue
		}

		// 2. 声明队列（durable + non-auto-delete）
		//    即便 worker 反复重启，broker 端队列不会丢
		if _, err := ch.QueueDeclare(redis_repo.TranscodeQueueName, true, false, false, false, nil); err != nil {
			logger.Warn("transcode consume: queue declare failed", zap.Error(err))
			time.Sleep(time.Second)
			continue
		}

		// 3. Qos=10：未 ack 消息超过 10 条时 broker 停止推送
		//    转码是 CPU 密集型，限制并发避免压垮 worker
		if err := ch.Qos(10, 0, false); err != nil {
			logger.Warn("transcode consume: qos set failed", zap.Error(err))
			time.Sleep(time.Second)
			continue
		}

		// 4. 注册消费者：autoAck=false 让我们能精确控制 ack 时机
		msgs, err := ch.Consume(redis_repo.TranscodeQueueName, "", false, false, false, false, nil)
		if err != nil {
			logger.Warn("transcode consume: consume register failed", zap.Error(err))
			time.Sleep(time.Second)
			continue
		}

		logger.Info("transcode consume registered, waiting for messages")

		// 5. 内层消息处理循环
		//    msgs channel 在以下情况会关闭：
		//      - Connection 断开（broker 端断开 socket）
		//      - ch.Cancel() 被调用
		//    此时 for range 退出，回到外层重新 WaitReady
		for msg := range msgs {
			// 停机信号优先：先 nack + requeue 让消息回到 broker，再退出
			select {
			case <-ctx.Done():
				_ = msg.Nack(false, true)
				return
			default:
			}

			var m cache.TranscodeMsg
			if err := json.Unmarshal(msg.Body, &m); err != nil {
				// 消息体损坏：nack(requeue=false) 让它进死信或被丢弃
				logger.Warn("transcode consume: invalid message body, dropping",
					zap.Error(err),
					zap.Int("body_len", len(msg.Body)))
				_ = msg.Nack(false, false)
				continue
			}

			// transcodeOne 内部已处理失败场景（写入 status=failed），
			// 任意返回值都视为"已处理"，Ack 消息不再重投
			if err := transcodeOne(ctx, m, db, storageInstance, ffmpegCfg, tempDir, videoSearchRepo, logger); err != nil {
				logger.Warn("transcode one returned error (still acking to avoid retry loop)",
					zap.Uint("video_id", m.VideoID),
					zap.Error(err))
			}
			_ = msg.Ack(false)
		}

		// 走到这里说明 msgs channel 关闭（broker 断开 / channel 被 cancel）
		// 回到外层 WaitReady 重新订阅
		logger.Warn("transcode consume channel closed, will re-subscribe after reconnect")
	}
}

// transcodeOne 处理单条转码消息：调用 ffmpeg 转码 → 截帧生成封面（如需）→ 上传 → 更新 DB。
//
// 流程：
//  1. 标记 status=transcoding，让前端轮询能看到"正在转码"
//  2. 准备输出目录 {tempDir}/transcoded/，避免与上传目录混用
//  3. 调用 ffmpeg -i {draft} -c:v libx264 -preset medium -crf 23 -c:a aac -y {output}.mp4
//     使用 CommandContext 绑定超时，避免 ffmpeg 卡死拖垮 worker
//  4. 失败：截取 stderr 末尾 2000 字符作为 fail_reason，写入 status=failed，返回 nil（Ack）
//  5. 成功：用 ffprobe 解析时长
//  6. 若 DraftCoverPath 为空：调用 ffmpeg -ss {duration/2} -i {draft} -frames:v 1 截帧生成 JPG
//  7. 通过 storage.Put 上传 video 和 cover 到 {video}/{videoID}.mp4 / {cover}/{videoID}.jpg
//  8. UPDATE videos SET video_url, cover_url, duration_sec, status='published' WHERE id=?
//
// 返回值约定：
//   - nil：转码流程结束（无论成功或失败，DB 状态已正确）
//   - error：仅用于不可恢复的内部错误（如 DB 不可达）；调用方仍会 Ack
func transcodeOne(ctx context.Context, msg cache.TranscodeMsg, db *gorm.DB, storageInstance storage.Storage, ffmpegCfg config.FFmpegConfig, tempDir string, videoSearchRepo interfaces.VideoSearchRepository, logger *zap.Logger) error {
	// 1. 标记 status=transcoding，让前端轮询能看到"正在转码"
	//    使用裸 db.Model 而不是 VideoDraftRepo.UpdateTranscodeResult，因为 worker 进程
	//    没有 Repos 容器（Repos 是 API 进程的依赖聚合）
	if err := db.WithContext(ctx).Model(&database.Video{}).
		Where("id = ?", msg.VideoID).
		Update("status", "transcoding").Error; err != nil {
		logger.Warn("transcode: failed to mark transcoding",
			zap.Uint("video_id", msg.VideoID),
			zap.Error(err))
		// 继续转码：标记失败不应阻止后续步骤
	}

	// 2. 准备输出目录
	outDir := filepath.Join(tempDir, "transcoded")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		// 目录创建失败是致命错误：写不进去就无法转码
		failReason := truncateErr(fmt.Sprintf("mkdir transcoded dir failed: %v", err))
		_ = db.WithContext(ctx).Model(&database.Video{}).
			Where("id = ?", msg.VideoID).
			Updates(map[string]interface{}{"status": "failed", "fail_reason": failReason})
		return nil
	}

	outputPath := filepath.Join(outDir, fmt.Sprintf("%d.mp4", msg.VideoID))

	// 3. 调用 ffmpeg 转码
	//    超时通过 ctx 控制：30 分钟兜底，足够覆盖 1GB 视频的转码时间
	//    （libx264 medium preset 在主流 CPU 上约 30~60fps，1GB / 5Mbps ≈ 30 分钟视频）
	//    注意：ffmpeg 的 -y 表示覆盖输出，避免交互式提示阻塞
	//    TODO(Phase 5): 从 config.TranscodeConfig.Timeout 注入更精确的超时
	transcodeCtx, transcodeCancel := context.WithTimeout(ctx, 30*time.Minute)
	defer transcodeCancel()

	ffmpegBin := ffmpegCfg.BinaryPath
	if ffmpegBin == "" {
		ffmpegBin = "ffmpeg"
	}
	outputCodec := ffmpegCfg.OutputCodec
	if outputCodec == "" {
		outputCodec = "libx264"
	}
	preset := ffmpegCfg.Preset
	if preset == "" {
		preset = "medium"
	}
	crf := ffmpegCfg.CRF
	if crf == 0 {
		crf = 23
	}

	// ffmpeg -i {input} -c:v libx264 -preset medium -crf 23 -c:a aac -y {output}
	cmd := exec.CommandContext(transcodeCtx, ffmpegBin,
		"-i", msg.DraftRawPath,
		"-c:v", outputCodec,
		"-preset", preset,
		"-crf", strconv.Itoa(crf),
		"-c:a", "aac",
		"-y", outputPath,
	)
	// 捕获 stderr 用于失败时输出 fail_reason
	// ffmpeg 进度信息全部走 stderr，stdout 留空
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf
	// stdin 显式设为 nil 避免从控制台读取
	cmd.Stdin = nil

	if err := cmd.Run(); err != nil {
		// 转码失败：截取 stderr 末尾 2000 字符作为 fail_reason
		failReason := truncateErr(fmt.Sprintf("ffmpeg exit: %v, stderr: %s",
			err, stderrBuf.String()))
		logger.Warn("transcode: ffmpeg failed",
			zap.Uint("video_id", msg.VideoID),
			zap.String("fail_reason", failReason))
		_ = db.WithContext(ctx).Model(&database.Video{}).
			Where("id = ?", msg.VideoID).
			Updates(map[string]interface{}{"status": "failed", "fail_reason": failReason})
		// 清理可能产生的部分输出文件
		_ = os.Remove(outputPath)
		return nil
	}

	// 4. 解析视频时长：使用 ffprobe（默认在 PATH 中）
	//    ffprobe -v error -show_entries format=duration -of default=noprint_wrappers=1:nokey=1 {output}
	durationCtx, durationCancel := context.WithTimeout(ctx, 30*time.Second)
	defer durationCancel()
	probeCmd := exec.CommandContext(durationCtx, "ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		outputPath,
	)
	probeOut, err := probeCmd.Output()
	if err != nil {
		// ffprobe 失败：不算致命，duration 留 0
		logger.Warn("transcode: ffprobe failed, duration will be 0",
			zap.Uint("video_id", msg.VideoID),
			zap.Error(err))
	}
	duration := 0.0
	if err == nil {
		// 输出格式：浮点数字符串 + 换行，如 "12.345000\n"
		durStr := strings.TrimSpace(string(probeOut))
		if durStr != "" {
			if d, perr := strconv.ParseFloat(durStr, 64); perr == nil {
				duration = d
			}
		}
	}

	// 5. 准备封面文件
	//    若 DraftCoverPath 已指定（用户上传时自带），直接复用
	//    否则调用 ffmpeg -ss {duration/2} -i {draft} -frames:v 1 截帧生成 JPG
	var coverLocalPath string
	if msg.DraftCoverPath != "" {
		coverLocalPath = msg.DraftCoverPath
	} else {
		coverLocalPath = filepath.Join(outDir, fmt.Sprintf("%d_cover.jpg", msg.VideoID))
		// 取视频中间时刻截帧，避免开头黑屏
		ssTime := duration / 2
		coverCtx, coverCancel := context.WithTimeout(ctx, 30*time.Second)
		coverCmd := exec.CommandContext(coverCtx, ffmpegBin,
			"-ss", strconv.FormatFloat(ssTime, 'f', 3, 64),
			"-i", msg.DraftRawPath,
			"-frames:v", "1",
			"-y", coverLocalPath,
		)
		var coverStderr strings.Builder
		coverCmd.Stderr = &coverStderr
		if err := coverCmd.Run(); err != nil {
			// 截帧失败：不致命，cover_url 留空，视频仍可发布
			logger.Warn("transcode: cover extract failed, cover_url will be empty",
				zap.Uint("video_id", msg.VideoID),
				zap.String("stderr", coverStderr.String()),
				zap.Error(err))
			coverLocalPath = ""
		}
		coverCancel()
	}

	// 6. 上传视频文件到 Storage
	videoFile, err := os.Open(outputPath)
	if err != nil {
		// 致命：转码好的文件打不开，标记失败
		failReason := truncateErr(fmt.Sprintf("open transcoded video failed: %v", err))
		_ = db.WithContext(ctx).Model(&database.Video{}).
			Where("id = ?", msg.VideoID).
			Updates(map[string]interface{}{"status": "failed", "fail_reason": failReason})
		return nil
	}
	videoKey := fmt.Sprintf("video/%d.mp4", msg.VideoID)
	var videoSize int64
	if fi, statErr := videoFile.Stat(); statErr == nil {
		videoSize = fi.Size()
	}
	videoURL, err := storageInstance.Put(ctx, videoKey, videoFile, videoSize)
	_ = videoFile.Close()
	if err != nil {
		failReason := truncateErr(fmt.Sprintf("upload video to storage failed: %v", err))
		_ = db.WithContext(ctx).Model(&database.Video{}).
			Where("id = ?", msg.VideoID).
			Updates(map[string]interface{}{"status": "failed", "fail_reason": failReason})
		return nil
	}

	// 7. 上传封面文件到 Storage（若有）
	var coverURL string
	if coverLocalPath != "" {
		coverFile, err := os.Open(coverLocalPath)
		if err == nil {
			coverKey := fmt.Sprintf("cover/%d.jpg", msg.VideoID)
			var coverSize int64
			if fi, statErr := coverFile.Stat(); statErr == nil {
				coverSize = fi.Size()
			}
			coverURL, err = storageInstance.Put(ctx, coverKey, coverFile, coverSize)
			_ = coverFile.Close()
		}
		if err != nil {
			// 封面上传失败：不致命，cover_url 留空，视频仍可发布
			logger.Warn("transcode: cover upload failed, cover_url will be empty",
				zap.Uint("video_id", msg.VideoID),
				zap.Error(err))
			coverURL = ""
		}
	}

	// 8. 更新 DB：写入 video_url / cover_url / duration_sec / status=published
	//    使用 Updates(map[...]) 确保零值字段也被写入
	if err := db.WithContext(ctx).Model(&database.Video{}).
		Where("id = ?", msg.VideoID).
		Updates(map[string]interface{}{
			"play_url":     videoURL,
			"cover_url":    coverURL,
			"duration_sec": duration,
			"status":       "published",
		}).Error; err != nil {
		logger.Error("transcode: failed to update video status to published",
			zap.Uint("video_id", msg.VideoID),
			zap.Error(err))
		return err
	}

	// 8.5 更新作者投稿数 video_count +1
	if err := db.WithContext(ctx).Model(&database.Account{}).
		Where("id = ?", msg.UserID).
		UpdateColumn("video_count", gorm.Expr("video_count + 1")).Error; err != nil {
		// 非致命错误：视频已发布成功，仅记日志
		logger.Warn("transcode: failed to increment video_count",
			zap.String("user_id", msg.UserID),
			zap.Uint("video_id", msg.VideoID),
			zap.Error(err))
	}

	// 8.5 索引视频到 ES（搜索功能依赖）
	//     转码完成、status=published 后立即写入 ES，使新视频可被搜索到
	//     ES 索引失败不影响发布流程（视频已可正常播放），仅记日志
	if videoSearchRepo != nil {
		indexVideoToES(ctx, db, videoSearchRepo, msg.VideoID, logger)
	}

	// 9. 转码成功后清理临时文件（draft 原始视频 + 转码输出）
	//    清理失败仅记日志，不影响业务流程
	//    注意：DraftCoverPath 若是用户上传的，也应一起清理
	_ = os.Remove(outputPath)
	if coverLocalPath != msg.DraftCoverPath {
		// 截帧生成的封面在 outDir，清理它
		_ = os.Remove(coverLocalPath)
	}
	// draft 原始文件保留由后续清理任务处理（避免清理后用户立即重传同名文件冲突）

	logger.Info("transcode: success",
		zap.Uint("video_id", msg.VideoID),
		zap.Float64("duration", duration),
		zap.String("video_url", videoURL),
		zap.String("cover_url", coverURL))
	return nil
}

// truncateErr 把 fail_reason 截断到 2000 字符以内，匹配 database.Video.FailReason 的 gorm size:2000。
// 截取末尾部分（最近发生的错误更相关）。
func truncateErr(s string) string {
	const max = 2000
	if len(s) <= max {
		return s
	}
	// 保留末尾 max 字符：最近的 ffmpeg 错误信息通常最有用
	return "..." + s[len(s)-max+3:]
}

// indexVideoToES 查询视频及其作者，构造 ES 文档并写入索引。
// 用于转码完成后将新发布的视频加入搜索索引。
// 任何步骤失败都仅记日志，不影响视频发布流程。
func indexVideoToES(ctx context.Context, db *gorm.DB, repo interfaces.VideoSearchRepository, videoID uint, logger *zap.Logger) {
	var video database.Video
	if err := db.WithContext(ctx).First(&video, videoID).Error; err != nil {
		logger.Warn("index to ES: query video failed",
			zap.Uint("video_id", videoID), zap.Error(err))
		return
	}
	var author database.Account
	if err := db.WithContext(ctx).First(&author, "id = ?", video.AuthorID).Error; err != nil {
		logger.Warn("index to ES: query author failed",
			zap.Uint("video_id", videoID), zap.Error(err))
		return
	}
	doc := es.NewVideoDocumentFromVideo(video, author.Username)
	if err := repo.IndexVideo(ctx, doc); err != nil {
		logger.Warn("index to ES: IndexVideo failed",
			zap.Uint("video_id", videoID), zap.Error(err))
		return
	}
	logger.Info("index to ES: success",
		zap.Uint("video_id", videoID),
		zap.String("title", video.Title))
}
