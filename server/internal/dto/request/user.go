package request

type Register struct {
	Username   string `json:"username" binding:"required,max=20"`
	Password   string `json:"password" binding:"required,min=6,max=15"`
	Email      string `json:"email" binding:"required,email"`
	VerifyCode string `json:"verifyCode" binding:"required,len=6"`
}

type Login struct {
	Email     string `json:"email" binding:"required,email"`
	Password  string `json:"password" binding:"required,min=6,max=15"`
	Captcha   string `json:"captcha" binding:"required,len=6"`
	CaptchaID string `json:"captcha_id" binding:"required"`
}

type ForgotPassword struct {
	Email       string `json:"email" binding:"required,email"`
	VerifyCode  string `json:"verifyCode" binding:"required,len=6"`
	NewPassword string `json:"newPassword" binding:"required,min=6,max=15"`
}

type UserCard struct {
	UserID string `json:"user_id" form:"user_id" binding:"required"`
	PageInfo
}

type UserChangeInfo struct {
	UserID                 string `json:"-"`
	Username               string `json:"username" binding:"required,max=20"`
	Signature              string `json:"signature" binding:"max=320"`
	Avatar                 string `json:"avatar"`
	Birthday               string `json:"birthday"`
	Gender                 string `json:"gender"`
	PrivacyPublicFavorites *bool  `json:"privacy_public_favorites"`
	PrivacyPublicFollowing *bool  `json:"privacy_public_following"`
	PrivacyPublicFans      *bool  `json:"privacy_public_fans"`
	ViewHistoryPaused      *bool  `json:"view_history_paused"`
}                          

type UserList struct {
	UUID *string `json:"uuid" form:"uuid"`
	PageInfo
}

type UserOperation struct {
	ID uint `json:"id" binding:"required"`
}

type UserLoginList struct {
	UUID *string `json:"uuid" form:"uuid"`
	PageInfo
}
