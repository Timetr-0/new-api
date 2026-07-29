package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
)

const (
	EmailVerificationRateLimitMark = "EV"
	EmailVerificationDuration      = 30 // 30秒时间窗口
)

var (
	EmailVerificationMaxRequestsSpec = common.GetRateLimitSpecEnvOrDefault("EMAIL_VERIFICATION_RATE_LIMIT", "2", false)
	EmailVerificationMaxRequests     = common.RateLimitSpecBaseValue(EmailVerificationMaxRequestsSpec, 2) // 30秒内最多2次
)

func resolveEmailVerificationMaxRequests(c *gin.Context) (int, bool) {
	maxRequests, err := common.ResolveRateLimitSpec(EmailVerificationRateLimitMark, EmailVerificationMaxRequestsSpec, false)
	if err != nil {
		fmt.Println(err.Error())
		c.Status(http.StatusInternalServerError)
		c.Abort()
		return 0, false
	}
	return maxRequests, true
}

func redisEmailVerificationRateLimiter(c *gin.Context) {
	maxRequests, ok := resolveEmailVerificationMaxRequests(c)
	if !ok {
		return
	}
	ctx := context.Background()
	rdb := common.RDB
	key := "emailVerification:" + EmailVerificationRateLimitMark + ":" + c.ClientIP()

	count, err := rdb.Incr(ctx, key).Result()
	if err != nil {
		// fallback
		memoryEmailVerificationRateLimiter(c)
		return
	}

	// 第一次设置键时设置过期时间
	if count == 1 {
		_ = rdb.Expire(ctx, key, time.Duration(EmailVerificationDuration)*time.Second).Err()
	}

	// 检查是否超出限制
	if count <= int64(maxRequests) {
		c.Next()
		return
	}

	// 获取剩余等待时间
	ttl, err := rdb.TTL(ctx, key).Result()
	waitSeconds := int64(EmailVerificationDuration)
	if err == nil && ttl > 0 {
		waitSeconds = int64(ttl.Seconds())
	}

	c.JSON(http.StatusTooManyRequests, gin.H{
		"success": false,
		"message": fmt.Sprintf("发送过于频繁，请等待 %d 秒后再试", waitSeconds),
	})
	c.Abort()
}

func memoryEmailVerificationRateLimiter(c *gin.Context) {
	maxRequests, ok := resolveEmailVerificationMaxRequests(c)
	if !ok {
		return
	}
	key := EmailVerificationRateLimitMark + ":" + c.ClientIP()

	if !inMemoryRateLimiter.Request(key, maxRequests, EmailVerificationDuration) {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"success": false,
			"message": "发送过于频繁，请稍后再试",
		})
		c.Abort()
		return
	}

	c.Next()
}

func EmailVerificationRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if common.RedisEnabled {
			redisEmailVerificationRateLimiter(c)
		} else {
			inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
			memoryEmailVerificationRateLimiter(c)
		}
	}
}
