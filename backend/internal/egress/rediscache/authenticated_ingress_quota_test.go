package rediscache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestAuthenticatedIngressQuotaStoreSharesAtomicLimits(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store := &authenticatedIngressQuotaStore{rdb: client}
	expiresAt := time.Now().UTC().Add(time.Hour)

	allowed, err := store.ReserveValidation(context.Background(), "2026-09-03", "owner-a", expiresAt, 2, 1)
	require.NoError(t, err)
	require.True(t, allowed)
	allowed, err = store.ReserveValidation(context.Background(), "2026-09-03", "owner-a", expiresAt, 2, 1)
	require.NoError(t, err)
	require.False(t, allowed)
	allowed, err = store.ReserveValidation(context.Background(), "2026-09-03", "owner-b", expiresAt, 2, 1)
	require.NoError(t, err)
	require.True(t, allowed)
	allowed, err = store.ReserveValidation(context.Background(), "2026-09-03", "owner-c", expiresAt, 2, 1)
	require.NoError(t, err)
	require.False(t, allowed)

	allowed, err = store.ReserveProfile(context.Background(), "2026-09-03", expiresAt, 1)
	require.NoError(t, err)
	require.True(t, allowed)
	allowed, err = store.ReserveProfile(context.Background(), "2026-09-03", expiresAt, 1)
	require.NoError(t, err)
	require.False(t, allowed)
}
