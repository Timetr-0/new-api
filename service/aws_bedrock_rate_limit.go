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
	awsBedrockRateLimitScope    = "AWS_BEDROCK_RATE_LIMIT"
	awsBedrockRateLimitKey      = "rateLimit:aws_bedrock:requests"
	awsBedrockRateLimitQueueKey = "rateLimit:aws_bedrock:queue"
	awsBedrockRateLimitWindow   = time.Minute
	awsBedrockRateLimitMinSleep = 25 * time.Millisecond
	awsBedrockRateLimitMaxSleep = time.Second
)

var awsBedrockMemoryLimiter = &awsBedrockSlidingWindowLimiter{}

type awsBedrockSlidingWindowLimiter struct {
	mu        sync.Mutex
	times     []time.Time
	queueSize int
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
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := now.Add(-window)
	drop := 0
	for drop < len(l.times) && !l.times[drop].After(cutoff) {
		drop++
	}
	if drop > 0 {
		l.times = l.times[drop:]
	}

	if len(l.times) < limit {
		l.times = append(l.times, now)
		return true, 0
	}

	retryAfter := l.times[0].Add(window).Sub(now)
	if retryAfter < awsBedrockRateLimitMinSleep {
		retryAfter = awsBedrockRateLimitMinSleep
	}
	return false, retryAfter
}

func WaitAwsBedrockRateLimit(c *gin.Context) *types.NewAPIError {
	if !setting.AwsBedrockRateLimitEnabled {
		return nil
	}

	limit, err := common.ResolveRateLimitSpec(awsBedrockRateLimitScope, setting.AwsBedrockRateLimitCountSpec, true)
	if err != nil {
		return types.NewOpenAIError(
			fmt.Errorf("aws bedrock rate limit config invalid: %w", err),
			types.ErrorCodeRateLimitExceeded,
			http.StatusInternalServerError,
			types.ErrOptionWithSkipRetry(),
		)
	}
	if limit <= 0 {
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
		return waitAwsBedrockRedisRateLimit(ctx, limit)
	}
	return waitAwsBedrockMemoryRateLimit(ctx, limit)
}

func waitAwsBedrockRedisRateLimit(ctx context.Context, limit int) *types.NewAPIError {
	entered, leave, err := enterAwsBedrockRedisQueue(ctx, setting.AwsBedrockRateLimitQueueMaxSize)
	if err != nil {
		return awsBedrockRateLimitError(fmt.Errorf("aws bedrock rate limit queue check failed: %w", err), http.StatusInternalServerError)
	}
	if !entered {
		return awsBedrockRateLimitError(errors.New("aws bedrock rate limit queue is full"), http.StatusTooManyRequests)
	}
	defer leave()

	for {
		allowed, retryAfter, err := limiter.AllowSlidingWindow(ctx, common.RDB, awsBedrockRateLimitKey, limit, awsBedrockRateLimitWindow)
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

func waitAwsBedrockMemoryRateLimit(ctx context.Context, limit int) *types.NewAPIError {
	entered, leave := awsBedrockMemoryLimiter.enterQueue(setting.AwsBedrockRateLimitQueueMaxSize)
	if !entered {
		return awsBedrockRateLimitError(errors.New("aws bedrock rate limit queue is full"), http.StatusTooManyRequests)
	}
	defer leave()

	for {
		allowed, retryAfter := awsBedrockMemoryLimiter.allow(limit, awsBedrockRateLimitWindow, time.Now())
		if allowed {
			return nil
		}
		if err := waitAwsBedrockRateLimitRetry(ctx, retryAfter); err != nil {
			return awsBedrockRateLimitError(errors.New("aws bedrock rate limit queue timeout"), http.StatusTooManyRequests)
		}
	}
}

func enterAwsBedrockRedisQueue(ctx context.Context, maxQueueSize int) (bool, func(), error) {
	if maxQueueSize <= 0 {
		return true, func() {}, nil
	}
	current, err := common.RDB.Incr(ctx, awsBedrockRateLimitQueueKey).Result()
	if err != nil {
		return false, nil, err
	}
	_ = common.RDB.Expire(ctx, awsBedrockRateLimitQueueKey, awsBedrockRateLimitWindow*2).Err()
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
