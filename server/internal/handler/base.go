package handler

import (
	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/dto/response"
	"fake_tiktok/internal/logic"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// -----------------------------------------------------------------------------

type BaseHandler struct {
	logic  *logic.LogicGroup
	logger *zap.Logger
}

// -----------------------------------------------------------------------------

func (h *BaseHandler) Captcha(c *gin.Context) {
	id, b64s, err := h.logic.BaseLogic.GenerateCaptcha()
	if err != nil {
		h.logger.Error("generate captcha failed", zap.Error(err))
		response.FailWithMsg(c, "generate captcha failed")
		return
	}
	response.OkWithData(c, response.Captcha{
		CaptchaID: id,
		PicPath:   b64s,
	})
}

// -----------------------------------------------------------------------------

func (h *BaseHandler) SendEmainlCode(c *gin.Context) {
	var req request.SendEmailCode
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, "bind json failed")
		return
	}
	if h.logic.BaseLogic.VerifyCaptcha(req.CaptchaID, req.Captcha) {
		err := h.logic.BaseLogic.SendEmailCode(c, req.Email)
		if err != nil {
			h.logger.Error("Failed to send email:", zap.Error(err))
			response.FailWithMsg(c, "Failed to send email")
			return
		}
		response.OkWithMsg(c, "Successfully sent email")
		return
	}
	response.FailWithMsg(c, "Incorrect verification code")
}
