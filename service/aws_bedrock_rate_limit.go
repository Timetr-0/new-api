package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/common/limiter"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const (
	awsBedrockRateLimitScope           = "AWS_BEDROCK_RATE_LIMIT"
	awsBedrockRateLimitDefaultGroup    = "default"
	awsBedrockRateLimitMinuteKeyPrefix = "rateLimit:aws_bedrock:requests:minute:"
	awsBedrockRateLimitSecondKeyPrefix = "rateLimit:aws_bedrock:requests:second:"
	awsBedrockRateLimitQueueKeyPrefix  = "rateLimit:aws_bedrock:queue:"
	awsBedrockRateLimitMinSleep        = 25 * time.Millisecond
	awsBedrockRateLimitMaxSleep        = time.Second
)

var awsBedrockMemoryLimiter = &awsBedrockSlidingWindowLimiter{}

type awsBedrockSlidingWindowLimiter struct {
	mu         sync.Mutex
	windows    map[string][]time.Time
	queueSizes map[string]int
}

type awsBedrockMemoryWindowLimit struct {
	key    string
	limit  int
	window time.Duration
}

func (l *awsBedrockSlidingWindowLimiter) enterQueue(queueKey string, maxQueueSize int) (bool, func()) {
	if maxQueueSize <= 0 {
		return true, func() {}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.queueSizes == nil {
		l.queueSizes = make(map[string]int)
	}
	if l.queueSizes[queueKey] >= maxQueueSize {
		return false, func() {}
	}
	l.queueSizes[queueKey]++
	return true, func() {
		l.mu.Lock()
		if l.queueSizes[queueKey] > 0 {
			l.queueSizes[queueKey]--
		}
		if l.queueSizes[queueKey] == 0 {
			delete(l.queueSizes, queueKey)
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

	minuteBaseLimit, err := common.ResolveRateLimitSpec(awsBedrockRateLimitScope, setting.AwsBedrockRateLimitCountSpec, true)
	if err != nil {
		return types.NewOpenAIError(
			fmt.Errorf("aws bedrock rate limit config invalid: %w", err),
			types.ErrorCodeRateLimitExceeded,
			http.StatusInternalServerError,
			types.ErrOptionWithSkipRetry(),
		)
	}
	secondBaseLimit, err := common.ResolveRateLimitSpec("AWS_BEDROCK_RATE_LIMIT_PER_SECOND", setting.AwsBedrockRateLimitPerSecondCountSpec, true)
	if err != nil {
		return types.NewOpenAIError(
			fmt.Errorf("aws bedrock per-second rate limit config invalid: %w", err),
			types.ErrorCodeRateLimitExceeded,
			http.StatusInternalServerError,
			types.ErrOptionWithSkipRetry(),
		)
	}
	if minuteBaseLimit <= 0 && secondBaseLimit <= 0 {
		return nil
	}

	group := awsBedrockRateLimitGroup(c)
	channelCount, err := awsBedrockRateLimitChannelCount(group)
	if err != nil {
		return awsBedrockRateLimitError(fmt.Errorf("aws bedrock channel count failed: %w", err), http.StatusInternalServerError)
	}
	minuteLimit := scaleAwsBedrockRateLimit(minuteBaseLimit, channelCount)
	secondLimit := scaleAwsBedrockRateLimit(secondBaseLimit, channelCount)

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
		return waitAwsBedrockRedisRateLimit(ctx, group, minuteLimit, secondLimit)
	}
	return waitAwsBedrockMemoryRateLimit(ctx, group, minuteLimit, secondLimit)
}

func awsBedrockRateLimitGroup(c *gin.Context) string {
	if c == nil {
		return awsBedrockRateLimitDefaultGroup
	}

	group := common.GetContextKeyString(c, constant.ContextKeyAutoGroup)
	if group == "" {
		group = common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	}
	if group == "" {
		group = common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
	}
	if group == "" || setting.IsAutoGroup(group) {
		return awsBedrockRateLimitDefaultGroup
	}
	return group
}

func awsBedrockRateLimitChannelCount(group string) (int, error) {
	group = strings.TrimSpace(group)
	if group == "" {
		return 1, nil
	}

	count, err := model.CountEnabledChannelsByGroupAndType(group, constant.ChannelTypeAws)
	if err != nil {
		return 0, err
	}
	if count < 1 {
		return 1, nil
	}
	return count, nil
}

func scaleAwsBedrockRateLimit(baseLimit int, channelCount int) int {
	if baseLimit <= 0 {
		return 0
	}
	if channelCount < 1 {
		channelCount = 1
	}

	maxInt := int(^uint(0) >> 1)
	if baseLimit > maxInt/channelCount {
		return maxInt
	}
	return baseLimit * channelCount
}

func waitAwsBedrockRedisRateLimit(ctx context.Context, group string, minuteLimit int, secondLimit int) *types.NewAPIError {
	entered, leave, err := enterAwsBedrockRedisQueue(ctx, group, setting.AwsBedrockRateLimitQueueMaxSize)
	if err != nil {
		return awsBedrockRateLimitError(fmt.Errorf("aws bedrock rate limit queue check failed: %w", err), http.StatusInternalServerError)
	}
	if !entered {
		return awsBedrockRateLimitError(errors.New("aws bedrock rate limit queue is full"), http.StatusTooManyRequests)
	}
	defer leave()

	limits := buildAwsBedrockRedisWindowLimits(group, minuteLimit, secondLimit)
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

func waitAwsBedrockMemoryRateLimit(ctx context.Context, group string, minuteLimit int, secondLimit int) *types.NewAPIError {
	entered, leave := awsBedrockMemoryLimiter.enterQueue(awsBedrockRateLimitQueueKey(group), setting.AwsBedrockRateLimitQueueMaxSize)
	if !entered {
		return awsBedrockRateLimitError(errors.New("aws bedrock rate limit queue is full"), http.StatusTooManyRequests)
	}
	defer leave()

	limits := buildAwsBedrockMemoryWindowLimits(group, minuteLimit, secondLimit)
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

func buildAwsBedrockRedisWindowLimits(group string, minuteLimit int, secondLimit int) []limiter.SlidingWindowLimit {
	limits := make([]limiter.SlidingWindowLimit, 0, 2)
	if minuteLimit > 0 {
		limits = append(limits, limiter.SlidingWindowLimit{
			Key:    awsBedrockRateLimitMinuteKey(group),
			Limit:  minuteLimit,
			Window: time.Minute,
		})
	}
	if secondLimit > 0 {
		limits = append(limits, limiter.SlidingWindowLimit{
			Key:    awsBedrockRateLimitSecondKey(group),
			Limit:  secondLimit,
			Window: time.Second,
		})
	}
	return limits
}

func buildAwsBedrockMemoryWindowLimits(group string, minuteLimit int, secondLimit int) []awsBedrockMemoryWindowLimit {
	limits := make([]awsBedrockMemoryWindowLimit, 0, 2)
	if minuteLimit > 0 {
		limits = append(limits, awsBedrockMemoryWindowLimit{
			key:    awsBedrockRateLimitMinuteKey(group),
			limit:  minuteLimit,
			window: time.Minute,
		})
	}
	if secondLimit > 0 {
		limits = append(limits, awsBedrockMemoryWindowLimit{
			key:    awsBedrockRateLimitSecondKey(group),
			limit:  secondLimit,
			window: time.Second,
		})
	}
	return limits
}

func enterAwsBedrockRedisQueue(ctx context.Context, group string, maxQueueSize int) (bool, func(), error) {
	if maxQueueSize <= 0 {
		return true, func() {}, nil
	}
	queueKey := awsBedrockRateLimitQueueKey(group)
	current, err := common.RDB.Incr(ctx, queueKey).Result()
	if err != nil {
		return false, nil, err
	}
	_ = common.RDB.Expire(ctx, queueKey, 2*time.Minute).Err()
	if current > int64(maxQueueSize) {
		_, _ = common.RDB.Decr(context.Background(), queueKey).Result()
		return false, func() {}, nil
	}
	return true, func() {
		_, _ = common.RDB.Decr(context.Background(), queueKey).Result()
	}, nil
}

func awsBedrockRateLimitMinuteKey(group string) string {
	return awsBedrockRateLimitMinuteKeyPrefix + awsBedrockRateLimitGroupKey(group)
}

func awsBedrockRateLimitSecondKey(group string) string {
	return awsBedrockRateLimitSecondKeyPrefix + awsBedrockRateLimitGroupKey(group)
}

func awsBedrockRateLimitQueueKey(group string) string {
	return awsBedrockRateLimitQueueKeyPrefix + awsBedrockRateLimitGroupKey(group)
}

func awsBedrockRateLimitGroupKey(group string) string {
	group = strings.TrimSpace(group)
	if group == "" || setting.IsAutoGroup(group) {
		group = awsBedrockRateLimitDefaultGroup
	}
	return url.QueryEscape(group)
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
