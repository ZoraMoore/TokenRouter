package rediscache

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/TokenFlux/TokenRouter/internal/egress/provider"
)

var (
	authenticatedIngressValidationQuotaScript = redis.NewScript(`
local global_count = tonumber(redis.call('GET', KEYS[1]) or '0')
local owner_count = tonumber(redis.call('GET', KEYS[2]) or '0')
if global_count >= tonumber(ARGV[1]) or owner_count >= tonumber(ARGV[2]) then
  return 0
end
redis.call('INCR', KEYS[1])
redis.call('INCR', KEYS[2])
redis.call('PEXPIREAT', KEYS[1], ARGV[3])
redis.call('PEXPIREAT', KEYS[2], ARGV[3])
return 1
`)

	authenticatedIngressProfileQuotaScript = redis.NewScript(`
local profile_count = tonumber(redis.call('GET', KEYS[1]) or '0')
if profile_count >= tonumber(ARGV[1]) then
  return 0
end
redis.call('INCR', KEYS[1])
redis.call('PEXPIREAT', KEYS[1], ARGV[2])
return 1
`)
)

type authenticatedIngressQuotaStore struct {
	rdb *redis.Client
}

// NewAuthenticatedIngressQuotaStore 创建只保存日计数和不可逆 owner 摘要的 Redis 配额仓储。
func NewAuthenticatedIngressQuotaStore(rdb *redis.Client) provider.AuthenticatedIngressQuotaStore {
	return &authenticatedIngressQuotaStore{rdb: rdb}
}

func (s *authenticatedIngressQuotaStore) ReserveValidation(
	ctx context.Context,
	date string,
	ownerID string,
	expiresAt time.Time,
	globalLimit int,
	ownerLimit int,
) (bool, error) {
	if s == nil || s.rdb == nil {
		return false, errors.New("authenticated ingress quota Redis is unavailable")
	}
	date = strings.TrimSpace(date)
	ownerID = strings.TrimSpace(ownerID)
	if date == "" || ownerID == "" || globalLimit <= 0 || ownerLimit <= 0 || expiresAt.IsZero() {
		return false, errors.New("authenticated ingress validation quota input is invalid")
	}
	globalKey := "tlsfp:authenticated-ingress:quota:" + date + ":validation:global"
	ownerKey := "tlsfp:authenticated-ingress:quota:" + date + ":validation:owner:" + ownerID
	result, err := authenticatedIngressValidationQuotaScript.Run(
		ctx,
		s.rdb,
		[]string{globalKey, ownerKey},
		globalLimit,
		ownerLimit,
		expiresAt.UnixMilli(),
	).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

func (s *authenticatedIngressQuotaStore) ReserveProfile(
	ctx context.Context,
	date string,
	expiresAt time.Time,
	limit int,
) (bool, error) {
	if s == nil || s.rdb == nil {
		return false, errors.New("authenticated ingress quota Redis is unavailable")
	}
	date = strings.TrimSpace(date)
	if date == "" || limit <= 0 || expiresAt.IsZero() {
		return false, errors.New("authenticated ingress profile quota input is invalid")
	}
	key := "tlsfp:authenticated-ingress:quota:" + date + ":profile:global"
	result, err := authenticatedIngressProfileQuotaScript.Run(
		ctx,
		s.rdb,
		[]string{key},
		limit,
		expiresAt.UnixMilli(),
	).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}
