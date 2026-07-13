package middleware

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"

	"fake_tiktok/internal/config"
	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/dto/response"
	"fake_tiktok/internal/pkg"
	"fake_tiktok/internal/repository/interfaces"
)

func JWTAuth(cfg *config.Config, jwtRepo interfaces.JWTRepository, accountRepo interfaces.AccountRepository) gin.HandlerFunc {
	// 在中间件初始化时创建 JWT 实例，避免每次请求都重复创建
	// JWT 实例是无状态的（只包含 secret 字节切片），可以安全复用
	// 旧版在每次请求中调用 pkg.NewJWT(&cfg.JWT)，产生不必要的内存分配
	j := pkg.NewJWT(&cfg.JWT)

	return func(c *gin.Context) {
		accessToken := pkg.GetAccessToken(c)
		refreshToken := pkg.GetRefreshToken(c)

		if jwtRepo.IsBlackListed(c.Request.Context(), refreshToken) {
			pkg.ClearRefreshToken(c)
			response.NoAuth("Please login again", c)
			c.Abort()
			return
		}
		claims, err := j.ParseAccessToken(accessToken)
		if err != nil {
			if accessToken == "" || errors.Is(err, pkg.TokenExpired) {
				refreshClaims, err := j.ParseRefreshToken(refreshToken)
				if err != nil {
					pkg.ClearRefreshToken(c)
					response.NoAuth("Please login again", c)
					c.Abort()
					return
				}
				var user database.Account
				userPtr, err := accountRepo.FindByID(c.Request.Context(), refreshClaims.UserID)
				if err != nil {
					pkg.ClearRefreshToken(c)
					response.NoAuth("Please login again", c)
					c.Abort()
					return
				}
				user = *userPtr
				newAccessClaims, err := j.CreateAccessClaims(request.BaseClaims{
					UserID: refreshClaims.UserID,
					Role:   user.Role,
				}, &cfg.JWT)
				if err != nil {
					pkg.ClearRefreshToken(c)
					response.NoAuth("Please login again", c)
					c.Abort()
					return
				}
				newaccessToken, err := j.CreateAccessToken(newAccessClaims)
				if err != nil {
					pkg.ClearRefreshToken(c)
					response.NoAuth("Please login again", c)
					c.Abort()
					return
				}
				c.Header("new-access-token", newaccessToken)
			c.Header("new-access-expiry", strconv.FormatInt(newAccessClaims.ExpiresAt.Unix(), 10))
			c.Set("claims", &newAccessClaims)
				c.Next()
				return
			}
			pkg.ClearRefreshToken(c)
			response.NoAuth("Please login again", c)
			c.Abort()
			return
		}
		c.Set("claims", claims)
		c.Next()
	}
}
