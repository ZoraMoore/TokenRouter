package identity

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type activityCacheStub struct{ values map[string]string }

func (c *activityCacheStub) Get(_ context.Context, key string) (string, error) {
	if value, ok := c.values[key]; ok {
		return value, nil
	}
	return "", errors.New("missing")
}

func (c *activityCacheStub) Put(_ context.Context, key, value string, _ time.Duration) error {
	c.values[key] = value
	return nil
}

func TestAPIActivityCountsUniqueUsersAcrossNodes(t *testing.T) {
	now := time.Now().UTC()
	cache := &activityCacheStub{values: map[string]string{}}
	makeTracker := func(id string) *APIActivityTracker {
		return NewAPIActivityTracker(APIActivityOptions{NodeID: id, NodeIDs: []string{"a", "b", "c"}, Cache: cache, Now: func() time.Time { return now }})
	}
	a, b, c := makeTracker("a"), makeTracker("b"), makeTracker("c")
	finish := a.Track(10)
	b.Track(10)()
	c.Track(20)()
	require.NoError(t, a.Publish(context.Background()))
	require.NoError(t, b.Publish(context.Background()))
	require.NoError(t, c.Publish(context.Background()))
	stats, err := a.Count(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, stats.ActiveUsers)
	require.True(t, stats.Complete)
	finish()
	finish()
	require.Equal(t, 0, a.snapshot(now).Users[10].Active)
}

func TestAPIActivityIncludesLongRequestsAndExpiresCompletedUsers(t *testing.T) {
	now := time.Now().UTC()
	tracker := NewAPIActivityTracker(APIActivityOptions{NodeID: "a", NodeIDs: []string{"a"}, Now: func() time.Time { return now }})
	finish := tracker.Track(11)
	now = now.Add(6 * time.Minute)
	stats, err := tracker.Count(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, stats.ActiveUsers)
	finish()
	now = now.Add(6 * time.Minute)
	stats, err = tracker.Count(context.Background())
	require.NoError(t, err)
	require.Zero(t, stats.ActiveUsers)
}

func TestAPIActivityDiscardsStaleInFlightStateButKeepsRecentHistory(t *testing.T) {
	now := time.Now().UTC()
	cache := &activityCacheStub{values: map[string]string{}}
	a := NewAPIActivityTracker(APIActivityOptions{NodeID: "a", NodeIDs: []string{"a", "b"}, Cache: cache, Now: func() time.Time { return now }})
	b := NewAPIActivityTracker(APIActivityOptions{NodeID: "b", NodeIDs: []string{"a", "b"}, Cache: cache, Now: func() time.Time { return now }})
	b.Track(22)
	require.NoError(t, b.Publish(context.Background()))
	now = now.Add(2 * time.Minute)
	stats, err := a.Count(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, stats.ActiveUsers)
	require.False(t, stats.Complete)
	now = now.Add(4 * time.Minute)
	stats, err = a.Count(context.Background())
	require.NoError(t, err)
	require.Zero(t, stats.ActiveUsers)
}

func TestAPIActivitySeedsHistoryWithoutRestoringOldInFlightCounters(t *testing.T) {
	now := time.Now().UTC()
	cache := &activityCacheStub{values: map[string]string{}}
	old := NewAPIActivityTracker(APIActivityOptions{NodeID: "a", NodeIDs: []string{"a"}, Cache: cache, Now: func() time.Time { return now }})
	old.Track(30)
	require.NoError(t, old.Publish(context.Background()))
	current := NewAPIActivityTracker(APIActivityOptions{NodeID: "a", NodeIDs: []string{"a"}, Cache: cache, Now: func() time.Time { return now }, Seed: func(context.Context, time.Time) (map[int64]time.Time, error) {
		return map[int64]time.Time{31: now.Add(-time.Minute)}, nil
	}})
	require.NoError(t, current.Publish(context.Background()))
	require.Zero(t, current.snapshot(now).Users[30].Active)
	stats, err := current.Count(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, stats.ActiveUsers)
}

func TestAPIActivityConcurrentTrackingPublishingAndCounting(t *testing.T) {
	tracker := NewAPIActivityTracker(APIActivityOptions{NodeID: "a"})
	var workers sync.WaitGroup
	for userID := int64(1); userID <= 6; userID++ {
		workers.Go(func() {
			for range 100 {
				finish := tracker.Track(userID)
				_, _ = tracker.Count(context.Background())
				finish()
			}
		})
	}
	workers.Go(func() {
		for range 100 {
			_ = tracker.Publish(context.Background())
		}
	})
	workers.Wait()
	stats, err := tracker.Count(context.Background())
	require.NoError(t, err)
	require.Equal(t, 6, stats.ActiveUsers)
	for _, user := range tracker.snapshot(time.Now()).Users {
		require.Zero(t, user.Active)
	}
}
