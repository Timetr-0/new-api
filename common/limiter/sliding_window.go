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
