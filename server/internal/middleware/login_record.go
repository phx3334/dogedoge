package middleware

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/repository/interfaces"
)

// AddressResolver 定义 IP 地址解析接口，用于解耦 login_record 中间件对 logic 层全局变量的依赖。
// 通过闭包注入实现，与其它中间件（JWT / Limit）的依赖注入风格保持一致。
type AddressResolver interface {
	GetAddressByIP(ip string) string
}

// LoginRecord 记录用户登录日志的中间件。
// 在请求处理完成后异步记录登录信息（IP 地址、设备信息等）。
// addressResolver：IP 地址解析器，通常为 GaodeLogic 实例，通过闭包注入避免依赖全局变量。
func LoginRecord(loginRepo interfaces.LoginRepository, addressResolver AddressResolver, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		go func() {
			var userID string
			ip := c.ClientIP()
			userAgent := c.Request.UserAgent()

			if value, exists := c.Get("user_id"); exists {
				if id, ok := value.(string); ok {
					userID = id
				}
			}

			if userID == "" {
				return
			}

			// 通过注入的 AddressResolver 解析 IP 地址，而非依赖全局变量 LogicGroupApp
			var address string
			if addressResolver != nil {
				address = addressResolver.GetAddressByIP(ip)
			} else {
				address = "未知"
			}

			os, deviceInfo, browserInfo := parseUserAgent(userAgent)

			login := database.Login{
				UserID:      userID,
				IP:          ip,
				Address:     address,
				OS:          os,
				DeviceInfo:  deviceInfo,
				BrowserInfo: browserInfo,
				Status:      c.Writer.Status(),
			}

			if err := loginRepo.Create(context.Background(), &login); err != nil {
				logger.Error("记录登录日志失败", zap.Error(err))
			}
		}()
	}
}

func parseUserAgent(userAgent string) (os, deviceInfo, browserInfo string) {
	os = parseOS(userAgent)
	deviceInfo = parseDevice(userAgent)
	browserInfo = parseBrowser(userAgent)
	return
}

func parseOS(userAgent string) string {
	if strings.Contains(userAgent, "Windows") {
		return "Windows"
	}
	if strings.Contains(userAgent, "Mac OS") {
		return "MacOS"
	}
	if strings.Contains(userAgent, "Linux") {
		return "Linux"
	}
	if strings.Contains(userAgent, "Android") {
		return "Android"
	}
	if strings.Contains(userAgent, "iPhone") || strings.Contains(userAgent, "iPad") {
		return "iOS"
	}
	return "Unknown"
}

func parseBrowser(userAgent string) string {
	if strings.Contains(userAgent, "Chrome") && !strings.Contains(userAgent, "Edg") {
		return "Chrome"
	}
	if strings.Contains(userAgent, "Firefox") {
		return "Firefox"
	}
	if strings.Contains(userAgent, "Safari") && !strings.Contains(userAgent, "Chrome") {
		return "Safari"
	}
	if strings.Contains(userAgent, "Edg") {
		return "Edge"
	}
	if strings.Contains(userAgent, "Opera") || strings.Contains(userAgent, "OPR") {
		return "Opera"
	}
	return "Unknown"
}

func parseDevice(userAgent string) string {
	if strings.Contains(userAgent, "Mobile") || strings.Contains(userAgent, "Android") {
		return "Mobile"
	}
	if strings.Contains(userAgent, "Tablet") || strings.Contains(userAgent, "iPad") {
		return "Tablet"
	}
	if strings.Contains(userAgent, "Windows") || strings.Contains(userAgent, "Mac OS") || strings.Contains(userAgent, "Linux") {
		return "Desktop"
	}
	return "Unknown"
}
