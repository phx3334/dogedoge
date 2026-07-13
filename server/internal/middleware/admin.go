package middleware

import(
	"github.com/gin-gonic/gin"
	"fake_tiktok/internal/pkg"
	"fake_tiktok/internal/domain/database"
	"fake_tiktok/internal/dto/response"
	"fake_tiktok/internal/config"
)

func AdminAuth(cfg *config.Config)gin.HandlerFunc{
	return func(c *gin.Context){
		role := pkg.GetRole(c, cfg)
		if role != database.Admin{
			response.Forbidden("admin auth required",c)
			c.Abort()
			return 
		}
		c.Next()
	}
}
