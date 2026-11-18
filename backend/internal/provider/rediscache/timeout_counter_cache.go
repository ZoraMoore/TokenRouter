package rediscache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/TokenFlux/TokenRouter/internal/provider"
)

const timeoutCounterPrefix = "timeout_count:provider:"

// timeoutCounterIncrScript 使用 Lua 脚本原子性地增加计数并返回当前值
// 如果 key 不存在，则创建并设置过期时间。
var timeoutCounterIncrScript = redis.NewScript(`
	local key = KEYS[1]
	local ttl = tonumber(ARGV[1])

	local count = redis.call('INCR', key)
	if count == 1 then
		redis.call('EXPIRE', key, ttl)
	end

	return count
`)

type timeoutCounterCache struct {
	rdb *redis.Client
}

// NewTimeoutCounterCache 创建超时计数器缓存实例。
func NewTimeoutCounterCache(rdb *redis.Client) provider.TimeoutCounterCache {
	return &timeoutCounterCache{rdb: rdb}
}

// IncrementTimeoutCount 增加提供商的超时计数，返回当前计数值
// windowMinutes 是计数窗口时间（分钟），超过此时间计数器会自动重置。
func (c *timeoutCounterCache) IncrementTimeoutCount(ctx context.Context, providerID int64, windowMinutes int) (int64, error) {
	key := fmt.Sprintf("%s%d", timeoutCounterPrefix, providerID)

	ttlSeconds := max(windowMinutes*60,
		// 最小1分钟
		60)

	result, err := timeoutCounterIncrScript.Run(ctx, c.rdb, []string{key}, ttlSeconds).Int64()
	if err != nil {
		return 0, fmt.Errorf("increment timeout count: %w", err)
	}

	return result, nil
}

// GetTimeoutCount 获取提供商当前的超时计数。
func (c *timeoutCounterCache) GetTimeoutCount(ctx context.Context, providerID int64) (int64, error) {
	key := fmt.Sprintf("%s%d", timeoutCounterPrefix, providerID)

	val, err := c.rdb.Get(ctx, key).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get timeout count: %w", err)
	}

	return val, nil
}

// ResetTimeoutCount 重置提供商的超时计数。
func (c *timeoutCounterCache) ResetTimeoutCount(ctx context.Context, providerID int64) error {
	key := fmt.Sprintf("%s%d", timeoutCounterPrefix, providerID)
	return c.rdb.Del(ctx, key).Err()
}

// GetTimeoutCountTTL 获取计数器剩余过期时间。
func (c *timeoutCounterCache) GetTimeoutCountTTL(ctx context.Context, providerID int64) (time.Duration, error) {
	key := fmt.Sprintf("%s%d", timeoutCounterPrefix, providerID)
	return c.rdb.TTL(ctx, key).Result()
}
