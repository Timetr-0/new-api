package limiter

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

const slidingWindowScript = `
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window_us = tonumber(ARGV[2])
local member = ARGV[3]

local now = redis.call('TIME')
local now_us = tonumber(now[1]) * 1000000 + tonumber(now[2])
local cutoff = now_us - window_us

redis.call('ZREMRANGEBYSCORE', key, '-inf', cutoff)
local count = redis.call('ZCARD', key)

if count < limit then
    redis.call('ZADD', key, now_us, member)
    redis.call('EXPIRE', key, math.max(1, math.ceil(window_us / 1000000) * 2))
    return {1, 0, count + 1}
end

local oldest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
local retry_ms = 100
if oldest[2] then
    retry_ms = math.max(1, math.ceil((tonumber(oldest[2]) + window_us - now_us) / 1000))
end

redis.call('EXPIRE', key, math.max(1, math.ceil(window_us / 1000000) * 2))
return {0, retry_ms, count}
`

const slidingWindowsScript = `
local member = ARGV[1]

local now = redis.call('TIME')
local now_us = tonumber(now[1]) * 1000000 + tonumber(now[2])
local retry_ms = 0

for i = 1, #KEYS do
    local limit = tonumber(ARGV[i * 2])
    local window_us = tonumber(ARGV[i * 2 + 1])
    local cutoff = now_us - window_us

    redis.call('ZREMRANGEBYSCORE', KEYS[i], '-inf', cutoff)
    local count = redis.call('ZCARD', KEYS[i])

    if count >= limit then
        local oldest = redis.call('ZRANGE', KEYS[i], 0, 0, 'WITHSCORES')
        local current_retry_ms = 100
        if oldest[2] then
            current_retry_ms = math.max(1, math.ceil((tonumber(oldest[2]) + window_us - now_us) / 1000))
        end
        if current_retry_ms > retry_ms then
            retry_ms = current_retry_ms
        end
    end
end

for i = 1, #KEYS do
    local window_us = tonumber(ARGV[i * 2 + 1])
    redis.call('EXPIRE', KEYS[i], math.max(1, math.ceil(window_us / 1000000) * 2))
end

if retry_ms > 0 then
    return {0, retry_ms}
end

for i = 1, #KEYS do
    local window_us = tonumber(ARGV[i * 2 + 1])
    redis.call('ZADD', KEYS[i], now_us, member)
    redis.call('EXPIRE', KEYS[i], math.max(1, math.ceil(window_us / 1000000) * 2))
end

return {1, 0}
`

type SlidingWindowLimit struct {
	Key    string
	Limit  int
	Window time.Duration
}

func AllowSlidingWindow(ctx context.Context, rdb *redis.Client, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	if limit <= 0 {
		return true, 0, nil
	}
	if window <= 0 {
		return false, 0, fmt.Errorf("sliding window duration must be positive")
	}
	if rdb == nil {
		return false, 0, fmt.Errorf("redis client is nil")
	}

	result, err := rdb.Eval(
		ctx,
		slidingWindowScript,
		[]string{key},
		limit,
		window.Microseconds(),
		common.GetUUID(),
	).Result()
	if err != nil {
		return false, 0, err
	}

	items, ok := result.([]interface{})
	if !ok || len(items) < 2 {
		return false, 0, fmt.Errorf("unexpected sliding window result: %v", result)
	}
	allowed, err := redisInt(items[0])
	if err != nil {
		return false, 0, err
	}
	retryAfterMillis, err := redisInt(items[1])
	if err != nil {
		return false, 0, err
	}
	return allowed == 1, time.Duration(retryAfterMillis) * time.Millisecond, nil
}

func AllowSlidingWindows(ctx context.Context, rdb *redis.Client, limits []SlidingWindowLimit) (bool, time.Duration, error) {
	keys := make([]string, 0, len(limits))
	args := make([]interface{}, 0, len(limits)*2+1)
	args = append(args, common.GetUUID())

	for _, limit := range limits {
		if limit.Limit <= 0 {
			continue
		}
		if limit.Window <= 0 {
			return false, 0, fmt.Errorf("sliding window duration must be positive")
		}
		if limit.Key == "" {
			return false, 0, fmt.Errorf("sliding window key must not be empty")
		}
		keys = append(keys, limit.Key)
		args = append(args, limit.Limit, limit.Window.Microseconds())
	}
	if len(keys) == 0 {
		return true, 0, nil
	}
	if rdb == nil {
		return false, 0, fmt.Errorf("redis client is nil")
	}

	result, err := rdb.Eval(ctx, slidingWindowsScript, keys, args...).Result()
	if err != nil {
		return false, 0, err
	}

	items, ok := result.([]interface{})
	if !ok || len(items) < 2 {
		return false, 0, fmt.Errorf("unexpected sliding windows result: %v", result)
	}
	allowed, err := redisInt(items[0])
	if err != nil {
		return false, 0, err
	}
	retryAfterMillis, err := redisInt(items[1])
	if err != nil {
		return false, 0, err
	}
	return allowed == 1, time.Duration(retryAfterMillis) * time.Millisecond, nil
}

func redisInt(value interface{}) (int64, error) {
	switch v := value.(type) {
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case string:
		return strconv.ParseInt(v, 10, 64)
	case []byte:
		return strconv.ParseInt(string(v), 10, 64)
	default:
		return 0, fmt.Errorf("unexpected redis integer value: %T", value)
	}
}
