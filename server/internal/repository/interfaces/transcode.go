package interfaces

import (
	"context"

	"fake_tiktok/internal/dto/cache"
)

// TranscodePublisher 视频转码任务消息发布器
//
// 由 API 层（VideoDraftLogic.UploadDraft）在 draft 落库成功后调用 Publish，
// 将转码任务投递到 mini_bili_transcode 队列；worker 端 transcodeConsumeLoop
// 消费后调用 ffmpeg 完成转码与封面截帧，并将可播放 URL 回写 video_url/cover_url。
type TranscodePublisher interface {
	Publish(ctx context.Context, msg cache.TranscodeMsg) error
}
