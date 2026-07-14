package handler

import (
	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/dto/response"
	"fake_tiktok/internal/logic"
	"fake_tiktok/internal/pkg"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// -----------------------------------------------------------------------------

type UserHandler struct {
	logic  *logic.LogicGroup
	logger *zap.Logger
}

// -----------------------------------------------------------------------------

func (h *UserHandler) Register(c *gin.Context) {
	var req request.Register
	err := c.ShouldBindJSON(&req)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}

	// 修复：之前用 cookie session 存验证码，跨域下 cookie 不会被回传，
	// 导致 "email not match"。改用 Redis 存储，按 email 维度独立校验。
	ok, err := h.logic.BaseLogic.VerifyEmailCode(c.Request.Context(), req.Email, req.VerifyCode)
	if err != nil {
		h.logger.Error("verify email code failed", zap.Error(err))
		response.FailWithMsg(c, "verify email code failed")
		return
	}
	if !ok {
		response.FailWithMsg(c, "verifyCode not match or expired")
		return
	}

	u := database.Account{Username: req.Username, Email: req.Email, Password: req.Password}
	user, err := h.logic.UserLogic.Register(c.Request.Context(), u)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	h.handleTokenResult(c, user)
}

// -----------------------------------------------------------------------------

func (h *UserHandler) handleTokenResult(c *gin.Context, user database.Account) {
	result, err := h.logic.UserLogic.GenerateToken(user)
	if err != nil {
		h.logger.Error("Failed to generate token:", zap.Error(err))
		response.FailWithMsg(c, err.Error())
		return
	}
	pkg.SetRefreshToken(c, result.RefreshToken, result.RefreshExpiry)
	c.Set("user_id", result.Account.ID)
	response.OkWithDetail(c, response.Login{
		Account:           result.Account,
		AccessToken:       result.AccessToken,
		AccessTokenExpire: int(result.AccessTokenExpire),
	}, "login success")
}

// -----------------------------------------------------------------------------

func (h *UserHandler) Login(c *gin.Context) {
	var req request.Login
	err := c.ShouldBindJSON(&req)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	if !h.logic.UserLogic.VerifyCaptcha(req.CaptchaID, req.Captcha) {
		response.FailWithMsg(c, "captcha not match or expired")
		return
	}
	u := database.Account{Email: req.Email, Password: req.Password}
	user, err := h.logic.UserLogic.Login(c.Request.Context(), u)
	if err != nil {
		h.logger.Error("Failed to login:", zap.Error(err))
		response.FailWithMsg(c, "login failed")
		return
	}
	h.handleTokenResult(c, user)
}

// -----------------------------------------------------------------------------

func (h *UserHandler) ForgotPassword(c *gin.Context) {
	var req request.ForgotPassword
	err := c.ShouldBindJSON(&req)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	// 修复：同样改用 Redis 校验邮箱验证码，绕开 cookie session 跨域问题。
	ok, err := h.logic.BaseLogic.VerifyEmailCode(c.Request.Context(), req.Email, req.VerifyCode)
	if err != nil {
		h.logger.Error("verify email code failed", zap.Error(err))
		response.FailWithMsg(c, "verify email code failed")
		return
	}
	if !ok {
		response.FailWithMsg(c, "verifyCode not match or expired")
		return
	}
	err = h.logic.UserLogic.ForgotPassword(c.Request.Context(), req)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
}

// -----------------------------------------------------------------------------

func (h *UserHandler) UserHomePage(c *gin.Context) {
	var req request.UserCard
	err := c.ShouldBindQuery(&req)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	userHome, err := h.logic.UserLogic.UserHome(c.Request.Context(), pkg.GetUserID(c, h.logic.Config), req.UserID, req.Page, req.PageSize)
	if err != nil {
		h.logger.Error("Failed to get card:", zap.Error(err))
		response.FailWithMsg(c, "Failed to get card")
		return
	}
	response.OkWithData(c, userHome)
}

// UserBrief 按 user_id 返回用户简档（id / username / avatar_url）。
// 用于私信入口：从关注列表 / 用户主页点"私信"时无需手动输入对方 ID。
func (h *UserHandler) UserBrief(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		response.FailWithMsg(c, "参数错误：user_id 必填")
		return
	}
	brief, err := h.logic.UserLogic.GetBrief(c.Request.Context(), userID)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	response.OkWithData(c, brief)
}

// -----------------------------------------------------------------------------

func (h *UserHandler) Logout(c *gin.Context) {
	userID := pkg.GetUserID(c, h.logic.Config)
	refreshToken := pkg.GetRefreshToken(c)
	pkg.ClearRefreshToken(c)
	// Logout 现在返回 error：Redis 不可用时 JWT 无法拉黑，需告知客户端
	if err := h.logic.UserLogic.Logout(userID, refreshToken); err != nil {
		h.logger.Warn("Logout may not have taken effect",
			zap.String("user_id", userID), zap.Error(err))
		response.FailWithMsg(c, "logout may not have taken effect due to server error")
		return
	}
	response.OkWithMsg(c, "Successful logout")
}

// -----------------------------------------------------------------------------

func (h *UserHandler) PersonalInfo(c *gin.Context) {
	userID := pkg.GetUserID(c, h.logic.Config)
	user, err := h.logic.UserLogic.UserInfo(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get user info:", zap.Error(err))
		response.FailWithMsg(c, "Failed to get user info")
		return
	}
	response.OkWithData(c, user)
}

// -----------------------------------------------------------------------------

func (h *UserHandler) UserChangeInfo(c *gin.Context) {
	var req request.UserChangeInfo
	err := c.ShouldBindJSON(&req)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	req.UserID = pkg.GetUserID(c, h.logic.Config)
	err = h.logic.UserLogic.UserChangeInfo(c.Request.Context(), req)
	if err != nil {
		h.logger.Error("Failed to change user info:", zap.Error(err))
		response.FailWithMsg(c, "Failed to change user info")
		return
	}
	response.OkWithMsg(c, "Successful change user info")
}

// -----------------------------------------------------------------------------

func (h *UserHandler) UserList(c *gin.Context) {
	var pageInfo request.UserList
	err := c.ShouldBindQuery(&pageInfo)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	list, total, err := h.logic.UserLogic.UserList(c.Request.Context(), pageInfo)
	if err != nil {
		h.logger.Error("Failed to get user list:", zap.Error(err))
		response.FailWithMsg(c, "Failed to get user list")
		return
	}
	response.OkWithData(c, response.PageResult{
		List:  list,
		Total: total,
	})
}

// -----------------------------------------------------------------------------

func (h *UserHandler) UserFreeze(c *gin.Context) {
	var req request.UserOperation
	err := c.ShouldBindJSON(&req)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	err = h.logic.UserLogic.UserFreeze(c.Request.Context(), req)
	if err != nil {
		h.logger.Error("Failed to freeze user:", zap.Error(err))
		response.FailWithMsg(c, "Failed to freeze user")
		return
	}
	response.OkWithMsg(c, "Successfully freeze user")
}

// -----------------------------------------------------------------------------

func (h *UserHandler) UserUnfreeze(c *gin.Context) {
	var req request.UserOperation
	err := c.ShouldBindJSON(&req)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}
	err = h.logic.UserLogic.UserUnfreeze(c.Request.Context(), req)
	if err != nil {
		h.logger.Error("Failed to unfreeze user:", zap.Error(err))
		response.FailWithMsg(c, "Failed to unfreeze user")
		return
	}
	response.OkWithMsg(c, "Successfully unfreeze user")
}

// -----------------------------------------------------------------------------

func (h *UserHandler) UserLoginList(c *gin.Context) {
	var pageInfo request.UserLoginList
	err := c.ShouldBindQuery(&pageInfo)
	if err != nil {
		response.FailWithMsg(c, err.Error())
		return
	}

	list, total, err := h.logic.UserLogic.UserLoginList(c.Request.Context(), pageInfo)
	if err != nil {
		h.logger.Error("Failed to get user login list:", zap.Error(err))
		response.FailWithMsg(c, "Failed to get user login list")
		return
	}
	response.OkWithData(c, response.PageResult{
		List:  list,
		Total: total,
	})
}

// -----------------------------------------------------------------------------

func (h *UserHandler) UploadAvatar(c *gin.Context) {
	file, err := c.FormFile("avatar")
	if err != nil {
		response.FailWithMsg(c, "please upload an avatar file")
		return
	}

	userID := pkg.GetUserID(c, h.logic.Config)
	avatarURL, err := h.logic.UserLogic.UploadAvatar(c.Request.Context(), userID, file)
	if err != nil {
		h.logger.Error("Failed to upload avatar:", zap.Error(err))
		response.FailWithMsg(c, err.Error())
		return
	}

	response.OkWithData(c, map[string]string{"avatar_url": avatarURL})
}

// UploadImage 通用图片上传接口（用于收藏夹封面等场景）
//
// 路由：POST /upload/image（私有组，需登录）
// 表单字段：file（必填，图片文件）
// 支持格式：jpg, jpeg, png, gif, webp
// 返回：{ url: string }
func (h *UserHandler) UploadImage(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		response.FailWithMsg(c, "请上传图片文件")
		return
	}

	userID := pkg.GetUserID(c, h.logic.Config)
	if userID == "" {
		response.FailWithMsg(c, "未登录")
		return
	}

	// 通过统一存储抽象上传（driver=qiniu 时落七牛云）
	url, err := h.logic.UserLogic.UploadImage(c.Request.Context(), userID, file)
	if err != nil {
		h.logger.Error("Failed to upload image", zap.Error(err))
		response.FailWithMsg(c, err.Error())
		return
	}

	response.OkWithData(c, map[string]string{"url": url})
}
