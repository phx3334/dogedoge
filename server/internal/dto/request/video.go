package request

type HomeVideoList struct {
	CursorPage
	Zone string `json:"zone,omitempty" form:"zone"`
}

type VideoDetailReq struct {
	VideoID uint `form:"video_id" binding:"required"`
}

type SendDanmakuReq struct {
	VideoID   uint64  `json:"video_id" binding:"required"`
	Content   string  `json:"content" binding:"required,max=100"`
	VideoTime float64 `json:"video_time"`
	Color     string  `json:"color"`
	FontSize  string  `json:"font_size"`
	Mode      int     `json:"mode"` // 0=滚动, 1=顶部, 2=底部
}

type SearchVideoReq struct {
	Keyword string `form:"keyword" binding:"required,min=1,max=100"`
	Cursor  string `form:"cursor"`
	Limit   int    `form:"limit"`
}

// VideoDraftUploadReq 视频草稿上传请求（multipart/form-data 表单字段）
//
// 字段说明：
//   - Title：标题，必填，最长 255 字符（与 database.Video.Title 列保持一致）
//   - Description：视频简介，可选，最长 512 字符
//   - Zone：分区，可选，最长 64 字符（对应 Video.Zone gorm size:64）
//   - Tags：标签列表，由 logic 层 JSON 序列化后写入 Video.TagsJSON
type VideoDraftUploadReq struct {
	Title       string   `json:"title" form:"title" binding:"required,max=255"`
	Description string   `json:"description" form:"description" binding:"max=512"`
	Zone        string   `json:"zone" form:"zone" binding:"max=64"`
	Tags        []string `json:"tags" form:"tags"`
}
