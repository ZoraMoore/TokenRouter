package rediscache

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/TokenFlux/TokenRouter/internal/provider"
)

const openAI403CounterPrefix = "openai_403_count:provider:"

var openAI403CounterIncrScript = redis.NewScript(`
	local key = KEYS[1]
	local ttl = tonumber(ARGV[1])

	local count = redis.call('INCR', key)
	if count == 1 then
		redis.call('EXPIRE', key, ttl)
	end

	return count
`)

type openAI403CounterCache struct {
	rdb *redis.Client
}

func NewOpenAI403CounterCache(rdb *redis.Client) provider.OpenAI403CounterCache {
	return &openAI403CounterCache{rdb: rdb}
}

func (c *openAI403CounterCache) IncrementOpenAI403Count(ctx context.Context, providerID int64, windowMinutes int) (int64, error) {
	key := fmt.Sprintf("%s%d", openAI403CounterPrefix, providerID)

	ttlSeconds := max(windowMinutes*60, 60)

	result, err := openAI403CounterIncrScript.Run(ctx, c.rdb, []string{key}, ttlSeconds).Int64()
	if err != nil {
		return 0, fmt.Errorf("increment openai 403 count: %w", err)
	}
	return result, nil
}

func (c *openAI403CounterCache) ResetOpenAI403Count(ctx context.Context, providerID int64) error {
	key := fmt.Sprintf("%s%d", openAI403CounterPrefix, providerID)
	return c.rdb.Del(ctx, key).Err()
}
