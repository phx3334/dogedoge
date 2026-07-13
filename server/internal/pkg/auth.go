package pkg

import (
	"fake_tiktok/internal/config"
	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/dto/request"
	"net"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func GetAccessToken(c *gin.Context) string {
	token := c.Request.Header.Get("x-access-token")
	return token
}

func GetRefreshToken(c *gin.Context) string {
	token, _ := c.Cookie("x-refresh-token")
	return token
}

func Getclaims(c *gin.Context, cfg *config.Config) (*request.JwtCustomClaims, error) {
	token := GetAccessToken(c)
	jwtConfig := cfg.JWT
	j := NewJWT(&jwtConfig)
	claims, err := j.ParseAccessToken(token)
	if err != nil {
		// 使用 zap.L() 全局日志实例（由 initialize/zap.go 中 ReplaceGlobals 初始化）
		zap.L().Error("parse claims error", zap.Error(err))
		return nil, err
	}
	return claims, err
}

// GetRole 从 gin.Context 中获取用户角色。
// 优先从上下文缓存中读取（由 JWT 中间件通过 c.Set("claims", ...) 设置），
// 若缓存不存在则重新解析 Access Token 获取。
// 注意：上下文 key 必须与 JWT 中间件中 c.Set 的 key 一致（均为 "claims"）。
func GetRole(c *gin.Context, cfg *config.Config) database.Role {
	if claims, exist := c.Get("claims"); !exist {
		if claim, err := Getclaims(c, cfg); err != nil {
			return 0
		} else {
			return claim.Role
		}
	} else {
		return claims.(*request.JwtCustomClaims).Role
	}
}

func SetRefreshToken(c *gin.Context, refreshToken string, expiry int) {
	host, _, err := net.SplitHostPort(c.Request.Host)
	if err != nil {
		host = c.Request.Host
	}
	SetCookie(c, "x-refresh-token", refreshToken, expiry, host)
}

func ClearRefreshToken(c *gin.Context) {
	host, _, err := net.SplitHostPort(c.Request.Host)
	if err != nil {
		host = c.Request.Host
	}
	SetCookie(c, "x-refresh-token", "", -1, host)
}

func SetCookie(c *gin.Context, name, value string, expire int, host string) {
	if net.ParseIP(host) != nil {
		c.SetCookie(name, value, expire, "/", "", false, true)
	} else {
		c.SetCookie(name, value, expire, "/", host, false, true)
	}
}

// GetUserID 从 gin.Context 中获取用户 ID。
// 优先从上下文缓存中读取（由 JWT 中间件通过 c.Set("claims", ...) 设置），
// 若缓存不存在则重新解析 Access Token 获取。
// 注意：上下文 key 必须与 JWT 中间件中 c.Set 的 key 一致（均为 "claims"）。
func GetUserID(c *gin.Context, cfg *config.Config) string {
	if claims, exist := c.Get("claims"); !exist {
		if cl, err := Getclaims(c, cfg); err != nil {
			return ""
		} else {
			return cl.UserID
		}
	} else {
		return claims.(*request.JwtCustomClaims).UserID
	}
}
