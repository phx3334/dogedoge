package request

// CreateCommentReq 创建评论请求
type CreateCommentReq struct {
	// TargetType 目标类型：video / article / dynamic
	TargetType string `json:"target_type" binding:"required"`
	// TargetID 目标 ID（视频 ID / 文章 ID / 动态 ID）
	TargetID uint64 `json:"target_id" binding:"required"`
	// ParentID 父评论 ID，0 表示顶级评论
	ParentID uint64 `json:"parent_id"`
	// Content 评论内容，1-2000 字符
	Content string `json:"content" binding:"required,min=1,max=2000"`
}

// ListCommentsReq 评论列表查询请求
type ListCommentsReq struct {
	TargetType string `form:"target_type" binding:"required"`
	TargetID   uint64 `form:"target_id" binding:"required"`
	Page       int    `form:"page"`
	PageSize   int    `form:"page_size"`
}

// ListRepliesReq 回复列表查询请求
type ListRepliesReq struct {
	// TargetType 目标类型，用于路由到对应评论表
	TargetType string `form:"target_type" binding:"required"`
	CommentID  uint64 `form:"comment_id" binding:"required"`
	Page       int    `form:"page"`
	PageSize   int    `form:"page_size"`
}

// CommentLikeReq 评论点赞请求
type CommentLikeReq struct {
	TargetType string `json:"target_type" binding:"required"`
	CommentID  uint64 `json:"comment_id" binding:"required"`
}

// DeleteCommentReq 删除评论请求
type DeleteCommentReq struct {
	TargetType string `json:"target_type" binding:"required"`
	CommentID  uint64 `json:"comment_id" binding:"required"`
}
