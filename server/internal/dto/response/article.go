package response

import "time"

// ArticleDetailResp 文章详情响应
type ArticleDetailResp struct {
	ID           uint64    `json:"id"`
	Title        string    `json:"title"`
	CoverURL     string    `json:"cover_url"`
	BodyMD       string    `json:"body_md"`
	Tags         []string  `json:"tags"`
	ImagesJSON   string    `json:"images_json"`
	ViewCount    uint64    `json:"view_count"`
	CommentCount uint64    `json:"comment_count"`
	CreatedAt    time.Time `json:"created_at"`
	Author       UserCard  `json:"author"`
}
