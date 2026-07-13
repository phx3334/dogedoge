package request

// ArticleDraftReq 保存文章草稿请求
type ArticleDraftReq struct {
	Title    string   `json:"title" binding:"required,max=80"`
	BodyMD   string   `json:"body_md" binding:"required"`
	CoverURL string   `json:"cover_url" binding:"max=1024"`
	Tags     []string `json:"tags"`
	Images   []string `json:"images"`
}

// ArticlePublishReq 发布文章请求
type ArticlePublishReq struct {
	ArticleID uint64 `json:"article_id" binding:"required"`
}
