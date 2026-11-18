package rediscache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	stickySessionPrefix      = "sticky_session:"
	stickySessionOwnerPrefix = "sticky_session_owner:"
)

// StickyCache 使用调度 Redis 存储粘性绑定，普通粘性和指定会话归属各自设置 TTL。
type StickyCache struct{ rdb *redis.Client }

func NewStickyCache(rdb *redis.Client) *StickyCache { return &StickyCache{rdb: rdb} }
func (c *StickyCache) GetSessionProviderID(ctx context.Context, groupID int64, sessionHash string) (int64, error) {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Get(ctx, key).Int64()
}

func (c *StickyCache) SetSessionProviderID(ctx context.Context, groupID int64, sessionHash string, providerID int64, ttl time.Duration) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Set(ctx, key, providerID, ttl).Err()
}

func (c *StickyCache) RefreshSessionTTL(ctx context.Context, groupID int64, sessionHash string, ttl time.Duration) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Expire(ctx, key, ttl).Err()
}

func (c *StickyCache) DeleteSessionProviderID(ctx context.Context, groupID int64, sessionHash string) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Del(ctx, key).Err()
}

func (c *StickyCache) SetSessionOwnerGroupID(ctx context.Context, userID int64, source, sessionHash string, groupID int64, ttl time.Duration) (bool, error) {
	key := buildSessionOwnerKey(userID, source, sessionHash)
	return c.rdb.SetNX(ctx, key, groupID, ttl).Result()
}

func (c *StickyCache) GetSessionOwnerGroupID(ctx context.Context, userID int64, source, sessionHash string) (int64, error) {
	key := buildSessionOwnerKey(userID, source, sessionHash)
	return c.rdb.Get(ctx, key).Int64()
}

func (c *StickyCache) RefreshSessionOwnerTTL(ctx context.Context, userID int64, source, sessionHash string, ttl time.Duration) error {
	key := buildSessionOwnerKey(userID, source, sessionHash)
	return c.rdb.Expire(ctx, key, ttl).Err()
}

func buildSessionKey(groupID int64, sessionHash string) string {
	return fmt.Sprintf("%s%d:%s", stickySessionPrefix, groupID, sessionHash)
}

func buildSessionOwnerKey(userID int64, source, sessionHash string) string {
	return fmt.Sprintf("%s%d:%s:%s", stickySessionOwnerPrefix, userID, source, sessionHash)
}
