package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

var timeFormat = "2006-01-02T15:04:05.000Z"

var inMemoryRateLimiter common.InMemoryRateLimiter

var defNext = func(c *gin.Context) {
	c.Next()
}

func resolveRateLimitSpec(c *gin.Context, countSpec string, allowZero bool, mark string) (int, bool) {
	maxRequestNum, err := common.ResolveRateLimitSpec(mark, countSpec, allowZero)
	if err != nil {
		fmt.Println(err.Error())
		c.Status(http.StatusInternalServerError)
		c.Abort()
		return 0, false
	}
	return maxRequestNum, true
}

func redisRateLimiter(c *gin.Context, maxRequestNum int, duration int64, mark string) {
	if maxRequestNum == 0 {
		return
	}
	ctx := context.Background()
	rdb := common.RDB
	key := "rateLimit:" + mark + c.ClientIP()
	listLength, err := rdb.LLen(ctx, key).Result()
	if err != nil {
		fmt.Println(err.Error())
		c.Status(http.StatusInternalServerError)
		c.Abort()
		return
	}
	if listLength < int64(maxRequestNum) {
		rdb.LPush(ctx, key, time.Now().Format(timeFormat))
		rdb.Expire(ctx, key, common.RateLimitKeyExpirationDuration)
	} else {
		oldTimeStr, _ := rdb.LIndex(ctx, key, -1).Result()
		oldTime, err := time.Parse(timeFormat, oldTimeStr)
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			c.Abort()
			return
		}
		nowTimeStr := time.Now().Format(timeFormat)
		nowTime, err := time.Parse(timeFormat, nowTimeStr)
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			c.Abort()
			return
		}
		// time.Since will return negative number!
		// See: https://stackoverflow.com/questions/50970900/why-is-time-since-returning-negative-durations-on-windows
		if int64(nowTime.Sub(oldTime).Seconds()) < duration {
			rdb.Expire(ctx, key, common.RateLimitKeyExpirationDuration)
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		} else {
			rdb.LPush(ctx, key, time.Now().Format(timeFormat))
			rdb.LTrim(ctx, key, 0, int64(maxRequestNum-1))
			rdb.Expire(ctx, key, common.RateLimitKeyExpirationDuration)
		}
	}
}

func memoryRateLimiter(c *gin.Context, maxRequestNum int, duration int64, mark string) {
	if maxRequestNum == 0 {
		return
	}
	key := mark + c.ClientIP()
	if !inMemoryRateLimiter.Request(key, maxRequestNum, duration) {
		c.Status(http.StatusTooManyRequests)
		c.Abort()
		return
	}
}

func rateLimitFactory(maxRequestNumSpec string, duration int64, mark string, allowZero bool) func(c *gin.Context) {
	if common.RedisEnabled {
		return func(c *gin.Context) {
			maxRequestNum, ok := resolveRateLimitSpec(c, maxRequestNumSpec, allowZero, mark)
			if !ok {
				return
			}
			redisRateLimiter(c, maxRequestNum, duration, mark)
		}
	} else {
		// It's safe to call multi times.
		inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
		return func(c *gin.Context) {
			maxRequestNum, ok := resolveRateLimitSpec(c, maxRequestNumSpec, allowZero, mark)
			if !ok {
				return
			}
			memoryRateLimiter(c, maxRequestNum, duration, mark)
		}
	}
}

func GlobalWebRateLimit() func(c *gin.Context) {
	if common.GlobalWebRateLimitEnable {
		return rateLimitFactory(common.GlobalWebRateLimitNumSpec, common.GlobalWebRateLimitDuration, "GW", false)
	}
	return defNext
}

func GlobalAPIRateLimit() func(c *gin.Context) {
	inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
	return func(c *gin.Context) {
		if !common.GlobalApiRateLimitEnable {
			return
		}
		maxRequestNum, ok := resolveRateLimitSpec(c, common.GlobalApiRateLimitNumSpec, false, "GA")
		if !ok {
			return
		}
		duration := common.GlobalApiRateLimitDuration
		if maxRequestNum <= 0 || duration <= 0 {
			return
		}
		if common.RedisEnabled {
			redisRateLimiter(c, maxRequestNum, duration, "GA")
			return
		}
		memoryRateLimiter(c, maxRequestNum, duration, "GA")
	}
}

func CriticalRateLimit() func(c *gin.Context) {
	if common.CriticalRateLimitEnable {
		return rateLimitFactory(common.CriticalRateLimitNumSpec, common.CriticalRateLimitDuration, "CT", false)
	}
	return defNext
}

func DownloadRateLimit() func(c *gin.Context) {
	return rateLimitFactory(common.DownloadRateLimitNumSpec, common.DownloadRateLimitDuration, "DW", false)
}

func UploadRateLimit() func(c *gin.Context) {
	return rateLimitFactory(common.UploadRateLimitNumSpec, common.UploadRateLimitDuration, "UP", false)
}

// userRateLimitFactory creates a rate limiter keyed by authenticated user ID
// instead of client IP, making it resistant to proxy rotation attacks.
// Must be used AFTER authentication middleware (UserAuth).
func userRateLimitFactory(maxRequestNumSpec string, duration int64, mark string, allowZero bool) func(c *gin.Context) {
	if common.RedisEnabled {
		return func(c *gin.Context) {
			maxRequestNum, ok := resolveRateLimitSpec(c, maxRequestNumSpec, allowZero, mark)
			if !ok {
				return
			}
			userId := c.GetInt("id")
			if userId == 0 {
				c.Status(http.StatusUnauthorized)
				c.Abort()
				return
			}
			key := fmt.Sprintf("rateLimit:%s:user:%d", mark, userId)
			userRedisRateLimiter(c, maxRequestNum, duration, key)
		}
	}
	// It's safe to call multi times.
	inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
	return func(c *gin.Context) {
		maxRequestNum, ok := resolveRateLimitSpec(c, maxRequestNumSpec, allowZero, mark)
		if !ok {
			return
		}
		userId := c.GetInt("id")
		if userId == 0 {
			c.Status(http.StatusUnauthorized)
			c.Abort()
			return
		}
		key := fmt.Sprintf("%s:user:%d", mark, userId)
		if !inMemoryRateLimiter.Request(key, maxRequestNum, duration) {
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		}
	}
}

// userRedisRateLimiter is like redisRateLimiter but accepts a pre-built key
// (to support user-ID-based keys).
func userRedisRateLimiter(c *gin.Context, maxRequestNum int, duration int64, key string) {
	if maxRequestNum == 0 {
		return
	}
	ctx := context.Background()
	rdb := common.RDB
	listLength, err := rdb.LLen(ctx, key).Result()
	if err != nil {
		fmt.Println(err.Error())
		c.Status(http.StatusInternalServerError)
		c.Abort()
		return
	}
	if listLength < int64(maxRequestNum) {
		rdb.LPush(ctx, key, time.Now().Format(timeFormat))
		rdb.Expire(ctx, key, common.RateLimitKeyExpirationDuration)
	} else {
		oldTimeStr, _ := rdb.LIndex(ctx, key, -1).Result()
		oldTime, err := time.Parse(timeFormat, oldTimeStr)
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			c.Abort()
			return
		}
		nowTimeStr := time.Now().Format(timeFormat)
		nowTime, err := time.Parse(timeFormat, nowTimeStr)
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			c.Abort()
			return
		}
		if int64(nowTime.Sub(oldTime).Seconds()) < duration {
			rdb.Expire(ctx, key, common.RateLimitKeyExpirationDuration)
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		} else {
			rdb.LPush(ctx, key, time.Now().Format(timeFormat))
			rdb.LTrim(ctx, key, 0, int64(maxRequestNum-1))
			rdb.Expire(ctx, key, common.RateLimitKeyExpirationDuration)
		}
	}
}

// SearchRateLimit returns a per-user rate limiter for search endpoints.
// Configurable via SEARCH_RATE_LIMIT_ENABLE / SEARCH_RATE_LIMIT / SEARCH_RATE_LIMIT_DURATION.
func SearchRateLimit() func(c *gin.Context) {
	if !common.SearchRateLimitEnable {
		return defNext
	}
	return userRateLimitFactory(common.SearchRateLimitNumSpec, common.SearchRateLimitDuration, "SR", false)
}
