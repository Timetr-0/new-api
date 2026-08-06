package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/common/limiter"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const (
	awsBedrockRateLimitScope     = "AWS_BEDROCK_RATE_LIMIT"
	awsBedrockRateLimitMinuteKey = "rateLimit:aws_bedrock:requests:minute"
	awsBedrockRateLimitSecondKey = "rateLimit:aws_bedrock:requests:second"
	awsBedrockRateLimitQueueKey  = "rateLimit:aws_bedrock:queue"
	awsBedrockRateLimitMinSleep  = 25 * time.Millisecond
	awsBedrockRateLimitMaxSleep  = time.Second
)

var awsBedrockMemoryLimiter = &awsBedrockSlidingWindowLimiter{}

type awsBedrockSlidingWindowLimiter struct {
	mu        sync.Mutex
	windows   map[string][]time.Time
	queueSize int
}

type awsBedrockMemoryWindowLimit struct {
	key    string
	limit  int
	window time.Duration
}

func (l *awsBedrockSlidingWindowLimiter) enterQueue(maxQueueSize int) (bool, func()) {
	if maxQueueSize <= 0 {
		return true, func() {}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.queueSize >= maxQueueSize {
		return false, func() {}
	}
	l.queueSize++
	return true, func() {
		l.mu.Lock()
		if l.queueSize > 0 {
			l.queueSize--
		}
		l.mu.Unlock()
	}
}

func (l *awsBedrockSlidingWindowLimiter) allow(limit int, window time.Duration, now time.Time) (bool, time.Duration) {
	return l.allowWindows([]awsBedrockMemoryWindowLimit{
		{
			key:    "default",
			limit:  limit,
			window: window,
		},
	}, now)
}

func (l *awsBedrockSlidingWindowLimiter) allowWindows(limits []awsBedrockMemoryWindowLimit, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.windows == nil {
		l.windows = make(map[string][]time.Time)
	}

	retryAfter := time.Duration(0)
	for _, limit := range limits {
		if limit.limit <= 0 {
			continue
		}
		times := l.windows[limit.key]
		cutoff := now.Add(-limit.window)
		drop := 0
		for drop < len(times) && !times[drop].After(cutoff) {
			drop++
		}
		if drop > 0 {
			times = times[drop:]
		}
		l.windows[limit.key] = times

		if len(times) >= limit.limit {
			currentRetryAfter := times[0].Add(limit.window).Sub(now)
			if currentRetryAfter < awsBedrockRateLimitMinSleep {
				currentRetryAfter = awsBedrockRateLimitMinSleep
			}
			if currentRetryAfter > retryAfter {
				retryAfter = currentRetryAfter
			}
		}
	}

	if retryAfter > 0 {
		return false, retryAfter
	}

	for _, limit := range limits {
		if limit.limit <= 0 {
			continue
		}
		l.windows[limit.key] = append(l.windows[limit.key], now)
	}
	return true, 0
}

func WaitAwsBedrockRateLimit(c *gin.Context) *types.NewAPIError {
	if !setting.AwsBedrockRateLimitEnabled {
		return nil
	}

	minuteLimit, err := common.ResolveRateLimitSpec(awsBedrockRateLimitScope, setting.AwsBedrockRateLimitCountSpec, true)
	if err != nil {
		return types.NewOpenAIError(
			fmt.Errorf("aws bedrock rate limit config invalid: %w", err),
			types.ErrorCodeRateLimitExceeded,
			http.StatusInternalServerError,
			types.ErrOptionWithSkipRetry(),
		)
	}
	secondLimit := setting.AwsBedrockRateLimitPerSecondCount
	if minuteLimit <= 0 && secondLimit <= 0 {
		return nil
	}

	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	timeoutSeconds := setting.AwsBedrockRateLimitQueueTimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	if common.RedisEnabled && common.RDB != nil {
		return waitAwsBedrockRedisRateLimit(ctx, minuteLimit, secondLimit)
	}
	return waitAwsBedrockMemoryRateLimit(ctx, minuteLimit, secondLimit)
}

func waitAwsBedrockRedisRateLimit(ctx context.Context, minuteLimit int, secondLimit int) *types.NewAPIError {
	entered, leave, err := enterAwsBedrockRedisQueue(ctx, setting.AwsBedrockRateLimitQueueMaxSize)
	if err != nil {
		return awsBedrockRateLimitError(fmt.Errorf("aws bedrock rate limit queue check failed: %w", err), http.StatusInternalServerError)
	}
	if !entered {
		return awsBedrockRateLimitError(errors.New("aws bedrock rate limit queue is full"), http.StatusTooManyRequests)
	}
	defer leave()

	limits := buildAwsBedrockRedisWindowLimits(minuteLimit, secondLimit)
	for {
		allowed, retryAfter, err := limiter.AllowSlidingWindows(ctx, common.RDB, limits)
		if err != nil {
			return awsBedrockRateLimitError(fmt.Errorf("aws bedrock rate limit check failed: %w", err), http.StatusInternalServerError)
		}
		if allowed {
			return nil
		}
		if err := waitAwsBedrockRateLimitRetry(ctx, retryAfter); err != nil {
			return awsBedrockRateLimitError(errors.New("aws bedrock rate limit queue timeout"), http.StatusTooManyRequests)
		}
	}
}

func waitAwsBedrockMemoryRateLimit(ctx context.Context, minuteLimit int, secondLimit int) *types.NewAPIError {
	entered, leave := awsBedrockMemoryLimiter.enterQueue(setting.AwsBedrockRateLimitQueueMaxSize)
	if !entered {
		return awsBedrockRateLimitError(errors.New("aws bedrock rate limit queue is full"), http.StatusTooManyRequests)
	}
	defer leave()

	limits := buildAwsBedrockMemoryWindowLimits(minuteLimit, secondLimit)
	for {
		allowed, retryAfter := awsBedrockMemoryLimiter.allowWindows(limits, time.Now())
		if allowed {
			return nil
		}
		if err := waitAwsBedrockRateLimitRetry(ctx, retryAfter); err != nil {
			return awsBedrockRateLimitError(errors.New("aws bedrock rate limit queue timeout"), http.StatusTooManyRequests)
		}
	}
}

func buildAwsBedrockRedisWindowLimits(minuteLimit int, secondLimit int) []limiter.SlidingWindowLimit {
	limits := make([]limiter.SlidingWindowLimit, 0, 2)
	if minuteLimit > 0 {
		limits = append(limits, limiter.SlidingWindowLimit{
			Key:    awsBedrockRateLimitMinuteKey,
			Limit:  minuteLimit,
			Window: time.Minute,
		})
	}
	if secondLimit > 0 {
		limits = append(limits, limiter.SlidingWindowLimit{
			Key:    awsBedrockRateLimitSecondKey,
			Limit:  secondLimit,
			Window: time.Second,
		})
	}
	return limits
}

func buildAwsBedrockMemoryWindowLimits(minuteLimit int, secondLimit int) []awsBedrockMemoryWindowLimit {
	limits := make([]awsBedrockMemoryWindowLimit, 0, 2)
	if minuteLimit > 0 {
		limits = append(limits, awsBedrockMemoryWindowLimit{
			key:    "minute",
			limit:  minuteLimit,
			window: time.Minute,
		})
	}
	if secondLimit > 0 {
		limits = append(limits, awsBedrockMemoryWindowLimit{
			key:    "second",
			limit:  secondLimit,
			window: time.Second,
		})
	}
	return limits
}

func enterAwsBedrockRedisQueue(ctx context.Context, maxQueueSize int) (bool, func(), error) {
	if maxQueueSize <= 0 {
		return true, func() {}, nil
	}
	current, err := common.RDB.Incr(ctx, awsBedrockRateLimitQueueKey).Result()
	if err != nil {
		return false, nil, err
	}
	_ = common.RDB.Expire(ctx, awsBedrockRateLimitQueueKey, 2*time.Minute).Err()
	if current > int64(maxQueueSize) {
		_, _ = common.RDB.Decr(context.Background(), awsBedrockRateLimitQueueKey).Result()
		return false, func() {}, nil
	}
	return true, func() {
		_, _ = common.RDB.Decr(context.Background(), awsBedrockRateLimitQueueKey).Result()
	}, nil
}

func waitAwsBedrockRateLimitRetry(ctx context.Context, retryAfter time.Duration) error {
	if retryAfter < awsBedrockRateLimitMinSleep {
		retryAfter = awsBedrockRateLimitMinSleep
	}
	if retryAfter > awsBedrockRateLimitMaxSleep {
		retryAfter = awsBedrockRateLimitMaxSleep
	}

	timer := time.NewTimer(retryAfter)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func awsBedrockRateLimitError(err error, statusCode int) *types.NewAPIError {
	return types.NewOpenAIError(
		err,
		types.ErrorCodeRateLimitExceeded,
		statusCode,
		types.ErrOptionWithSkipRetry(),
	)
}
