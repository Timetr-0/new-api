package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAwsBedrockSlidingWindowLimiterRejectsBurstOverLimit(t *testing.T) {
	limiter := &awsBedrockSlidingWindowLimiter{}
	now := time.Unix(100, 0)

	allowed, retryAfter := limiter.allow(2, time.Minute, now)
	require.True(t, allowed)
	assert.Zero(t, retryAfter)

	allowed, retryAfter = limiter.allow(2, time.Minute, now.Add(time.Second))
	require.True(t, allowed)
	assert.Zero(t, retryAfter)

	allowed, retryAfter = limiter.allow(2, time.Minute, now.Add(2*time.Second))
	require.False(t, allowed)
	assert.Equal(t, 58*time.Second, retryAfter)
}

func TestAwsBedrockSlidingWindowLimiterAllowsAfterWindowSlides(t *testing.T) {
	limiter := &awsBedrockSlidingWindowLimiter{}
	now := time.Unix(100, 0)

	allowed, _ := limiter.allow(1, time.Minute, now)
	require.True(t, allowed)

	allowed, retryAfter := limiter.allow(1, time.Minute, now.Add(time.Minute))
	require.True(t, allowed)
	assert.Zero(t, retryAfter)
}

func TestAwsBedrockSlidingWindowLimiterRequiresAllWindowsBeforeRecording(t *testing.T) {
	limiter := &awsBedrockSlidingWindowLimiter{}
	now := time.Unix(100, 0)
	limits := []awsBedrockMemoryWindowLimit{
		{key: "minute", limit: 2, window: time.Minute},
		{key: "second", limit: 1, window: time.Second},
	}

	allowed, retryAfter := limiter.allowWindows(limits, now)
	require.True(t, allowed)
	assert.Zero(t, retryAfter)

	allowed, retryAfter = limiter.allowWindows(limits, now.Add(100*time.Millisecond))
	require.False(t, allowed)
	assert.Equal(t, 900*time.Millisecond, retryAfter)

	allowed, retryAfter = limiter.allowWindows(limits, now.Add(time.Second))
	require.True(t, allowed)
	assert.Zero(t, retryAfter)

	allowed, retryAfter = limiter.allowWindows(limits, now.Add(2*time.Second))
	require.False(t, allowed)
	assert.Equal(t, 58*time.Second, retryAfter)
}

func TestScaleAwsBedrockRateLimitUsesChannelCount(t *testing.T) {
	assert.Equal(t, 0, scaleAwsBedrockRateLimit(0, 8))
	assert.Equal(t, 10, scaleAwsBedrockRateLimit(10, 0))
	assert.Equal(t, 30, scaleAwsBedrockRateLimit(10, 3))
}

func TestAwsBedrockMemoryQueueLimit(t *testing.T) {
	limiter := &awsBedrockSlidingWindowLimiter{}

	entered, leave := limiter.enterQueue(1)
	require.True(t, entered)

	entered, leaveSecond := limiter.enterQueue(1)
	require.False(t, entered)
	leaveSecond()

	leave()
	entered, leaveThird := limiter.enterQueue(1)
	require.True(t, entered)
	leaveThird()
}
