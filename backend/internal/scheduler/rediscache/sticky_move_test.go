package rediscache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/scheduler"
)

func TestOpenAISessionMoveCompareAndSetAndRecoverReceipt(t *testing.T) {
	server := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	ctx := context.Background()
	cache := NewStickyCache(rdb)
	move := scheduler.OpenAISessionMove{Hash: "0123456789abcdef", SourceID: 121, APIKeyID: 32}
	changed := scheduler.OpenAISessionMove{Hash: "fedcba9876543210", SourceID: 121, APIKeyID: 32}
	window := scheduler.OpenAISessionMove{Hash: "0123400000000000", SourceID: 121, APIKeyID: 32}
	short := scheduler.OpenAISessionMove{Hash: "ffffdddd0000ba00", SourceID: 121, APIKeyID: 32}

	require.NoError(t, rdb.Set(ctx, schedulerProviderKey("88"),
		`{"ID":88,"Platform":"openai","Type":"oauth","Status":"active","Schedulable":true,"Concurrency":5,"GroupIDs":[32]}`, 0).Err())
	require.NoError(t, rdb.Set(ctx, openAISessionBindingKey(32, move.Hash), 121, 4*time.Minute).Err())
	require.NoError(t, rdb.Set(ctx, openAISessionBindingKey(32, changed.Hash), 100, 4*time.Minute).Err())
	require.NoError(t, rdb.Set(ctx, openAISessionBindingKey(32, window.Hash), 121, 4*time.Minute).Err())
	require.NoError(t, rdb.Set(ctx, wsSessionWindowKey(32, window), "owner", 4*time.Minute).Err())
	require.NoError(t, rdb.Set(ctx, openAISessionBindingKey(32, short.Hash), 121, 2*time.Minute).Err())
	candidates := []scheduler.OpenAISessionMove{move, changed, window, short}
	eligible, err := cache.EligibleOpenAISessionMoves(ctx, 32, candidates, 4)
	require.NoError(t, err)
	require.Equal(t, []scheduler.OpenAISessionMove{move}, eligible)

	batch := scheduler.OpenAISessionMoveBatch{JobID: "first", Number: 1, GroupID: 32, ProviderID: 88, Limit: 5, Moves: candidates}
	result, err := cache.ApplyOpenAISessionMoves(ctx, batch)
	require.NoError(t, err)
	require.True(t, result.Available)
	require.Equal(t, []bool{true, false, false, false}, result.Moved)
	require.Equal(t, "88", rdb.Get(ctx, openAISessionBindingKey(32, move.Hash)).Val())
	require.Equal(t, 4*time.Minute, rdb.PTTL(ctx, openAISessionBindingKey(32, move.Hash)).Val())

	require.NoError(t, rdb.Del(ctx, schedulerProviderKey("88")).Err())
	batch.Limit = 0
	replayed, err := cache.ApplyOpenAISessionMoves(ctx, batch)
	require.NoError(t, err)
	require.Equal(t, result, replayed)

	require.NoError(t, rdb.Set(ctx, schedulerProviderKey("99"),
		`{"ID":99,"Platform":"openai","Type":"oauth","Status":"active","Schedulable":true,"Concurrency":5,"GroupIDs":[32]}`, 0).Err())
	batch.JobID = "another"
	batch.ProviderID = 99
	batch.Limit = 5
	batch.Moves = []scheduler.OpenAISessionMove{{Hash: move.Hash, SourceID: 88, APIKeyID: 32}}
	result, err = cache.ApplyOpenAISessionMoves(ctx, batch)
	require.NoError(t, err)
	require.Equal(t, []bool{false}, result.Moved)
	require.Equal(t, "88", rdb.Get(ctx, openAISessionBindingKey(32, move.Hash)).Val())
}

func TestOpenAISessionMoveRespectsTargetCapacityAndMembership(t *testing.T) {
	server := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	ctx := context.Background()
	cache := NewStickyCache(rdb)
	move := scheduler.OpenAISessionMove{Hash: "0123456789abcdef", SourceID: 121, APIKeyID: 32}

	require.NoError(t, rdb.Set(ctx, schedulerProviderKey("88"),
		`{"ID":88,"Platform":"openai","Type":"oauth","Status":"active","Schedulable":true,"Concurrency":1,"GroupIDs":[32]}`, 0).Err())
	require.NoError(t, rdb.Set(ctx, openAISessionBindingKey(32, move.Hash), 121, 4*time.Minute).Err())
	require.NoError(t, rdb.ZAdd(ctx, liveProviderSlotKey(88), redis.Z{Member: "existing-live-slot", Score: float64(time.Now().Unix())}).Err())
	batch := scheduler.OpenAISessionMoveBatch{JobID: "first", Number: 1, GroupID: 32, ProviderID: 88, Limit: 1, Moves: []scheduler.OpenAISessionMove{move}}
	result, err := cache.ApplyOpenAISessionMoves(ctx, batch)
	require.NoError(t, err)
	require.Equal(t, 1, result.Concurrency)
	require.Equal(t, []bool{false}, result.Moved)
	require.Equal(t, "121", rdb.Get(ctx, openAISessionBindingKey(32, move.Hash)).Val())

	batch.JobID = "second"
	batch.GroupID = 33
	result, err = cache.ApplyOpenAISessionMoves(ctx, batch)
	require.NoError(t, err)
	require.False(t, result.Available)
}
