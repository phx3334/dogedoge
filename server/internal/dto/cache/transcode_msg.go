package cache

// TranscodeMsg 视频转码消息体，由 API 发布到 mini_bili_transcode 队列，
// worker 消费后执行 ffmpeg 转码并将结果回写到 videos 表。
//
// 字段说明：
//   - VideoID：刚插入的 draft Video 记录 ID，worker 完成后据此 UPDATE
//   - DraftRawPath：API 端原子写入到 TempUploadDir 的本地视频文件路径
//   - DraftCoverPath：用户上传的封面本地路径；为空时由 worker 调用 ffmpeg 截帧生成 JPG
//   - UserID：作者 ID，用于日志追踪与权限校验
type TranscodeMsg struct {
	VideoID        uint   `json:"video_id"`
	DraftRawPath   string `json:"draft_raw_path"`
	DraftCoverPath string `json:"draft_cover_path"` // 可能为空，表示需要 ffmpeg 截帧生成封面
	UserID         string `json:"user_id"`
}
