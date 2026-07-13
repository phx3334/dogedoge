package request

type PageInfo struct {
	Page     int `json:"page" form:"page"`
	PageSize int `json:"page_size" form:"page_size"`
}

type SendEmailCode struct {
	Email     string `json:"email" binding:"required,email"`
	Captcha   string `json:"captcha" binding:"required,len=6"`
	CaptchaID string `json:"captcha_id" binding:"required"`
}

type CursorPage struct {
	Limit  int    `json:"limit" form:"limit"`
	Cursor string `json:"cursor" form:"cursor"`
}
