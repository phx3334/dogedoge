package pkg

import (
	"strings"

	"fake_tiktok/internal/config"

	"github.com/gin-gonic/gin"
)

func KeyByIP(c *gin.Context) (string, bool) {
	ip := strings.TrimSpace(c.ClientIP())
	if ip == "" {
		return "", false
	}
	return ip, true
}

func KeyByUserID(c *gin.Context, config *config.Config) (string, bool) {
	uid := GetUserID(c, config)
	if uid == "" {
		return "", false
	}
	return uid, true
}
