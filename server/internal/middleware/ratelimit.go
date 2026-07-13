package middleware

import (
	"fake_tiktok/internal/dto/response"
	redis_repo "fake_tiktok/internal/repository/redis"
	"time"


	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type KeyFunc func(c *gin.Context) (string, bool)

func Limit(cache *redis_repo.RedisClient, keyprefix string, maxRequests int64, window time.Duration, keyFunc KeyFunc, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		if cache == nil || keyFunc == nil || maxRequests <= 0 || window <= 0 {
			c.Next()
			return
		}
		subject, ok := keyFunc(c)
		if !ok {
			logger.Warn("rate limit keyFunc failed",
				zap.String("keyprefix", keyprefix),
				zap.String("path", c.Request.URL.Path),
			)
			c.Next()
			return
		}
		key := cache.BuildKey("ratelimit", keyprefix+":"+subject)
		allowed, err := cache.SlidingWindowLimit(c.Request.Context(), key, maxRequests, window)
		if err != nil {
			logger.Error("rate limit redis error",
				zap.Error(err),
				zap.String("key", key),
				zap.String("path", c.Request.URL.Path),
			)
			c.Next()
			return
		}
		if !allowed {
			logger.Info("rate limit exceeded",
				zap.String("key", key),
				zap.Int64("maxRequests", maxRequests),
				zap.Duration("window", window),
				zap.String("path", c.Request.URL.Path),
				zap.String("ip", c.ClientIP()),
			)
			response.FailWithMsg(c, "rate limit exceeded")
			return
		}
		c.Next()
	}
}
