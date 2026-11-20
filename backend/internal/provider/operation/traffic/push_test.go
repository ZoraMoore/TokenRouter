package traffic

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/provider/operation"
	"github.com/TokenFlux/TokenRouter/internal/routing"
	"github.com/TokenFlux/TokenRouter/internal/scheduler"
)

type trafficFixture struct {
	record     *provider.Record
	candidates []Session
	requests   []CompletedRequest
	status     int
	concurrent int
	now        func() time.Time
	batches    [][]scheduler.OpenAISessionMove
}

func TestPushWaitsForSuccessfulRequestBeforeAddingSessions(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	fixture := newTrafficFixture()
	push := New(fixture, fixture, fixture, fixture, fixture, trafficTestModelRules, nil)
	push.now = func() time.Time { return now }
	fixture.now = push.now
	job := operation.Job{ID: "job-a", ProviderID: 91, GroupID: 18}
	checkpoint := func(stage string, data any) error {
		job.Stage = stage
		job.Data, _ = json.Marshal(data)
		return nil
	}

	result, err := push.Run(ctx, job, checkpoint)
	require.Error(t, err)
	require.Equal(t, 2, result.MigratedSessions)
	require.Len(t, fixture.batches, 1)

	now = now.Add(10 * time.Second)
	fixture.concurrent = 3
	fixture.requests = []CompletedRequest{{SessionID: "a", UserID: 1, APIKeyID: 2, StartedAt: now}}
	result, err = push.Run(ctx, job, checkpoint)
	require.Error(t, err)
	require.Equal(t, 1, result.SuccessfulRequests)
	require.Equal(t, 3, result.PeakConcurrency)
	require.Len(t, fixture.batches, 1)

	now = now.Add(trafficObservation)
	fixture.concurrent = 0
	result, err = push.Run(ctx, job, checkpoint)
	require.Error(t, err)
	require.Equal(t, 6, result.MigratedSessions)
	require.Len(t, fixture.batches, 2)
	require.Len(t, fixture.batches[1], 4)
}

func TestPushStopsAfterTargetRejectsTraffic(t *testing.T) {
	ctx := context.Background()
	fixture := newTrafficFixture()
	push := New(fixture, fixture, fixture, fixture, fixture, trafficTestModelRules, nil)
	job := operation.Job{ID: "job-b", ProviderID: 91, GroupID: 18}
	checkpoint := func(stage string, data any) error {
		job.Stage = stage
		job.Data, _ = json.Marshal(data)
		return nil
	}

	_, err := push.Run(ctx, job, checkpoint)
	require.Error(t, err)
	fixture.status = 429
	result, err := push.Run(ctx, job, checkpoint)
	require.EqualError(t, err, "目标提供商返回上游 429，已停止推流")
	require.Equal(t, 2, result.MigratedSessions)
	require.Len(t, fixture.batches, 1)
}

func TestPushRequiresCompletionFromCurrentSessionAndOwner(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	fixture := newTrafficFixture()
	push := New(fixture, fixture, fixture, fixture, fixture, trafficTestModelRules, nil)
	push.now = func() time.Time { return now }
	fixture.now = push.now
	job := operation.Job{ID: "job-c", ProviderID: 91, GroupID: 18}
	checkpoint := func(stage string, data any) error {
		job.Stage = stage
		job.Data, _ = json.Marshal(data)
		return nil
	}

	_, err := push.Run(ctx, job, checkpoint)
	require.Error(t, err)
	fixture.requests = []CompletedRequest{
		{SessionID: "a", UserID: 1, APIKeyID: 3, StartedAt: now.Add(time.Second)},
		{SessionID: "a", UserID: 1, APIKeyID: 2, StartedAt: now.Add(-time.Second)},
		{SessionID: "c", UserID: 1, APIKeyID: 2, StartedAt: now.Add(time.Second)},
	}
	now = now.Add(trafficObservation)
	result, err := push.Run(ctx, job, checkpoint)
	require.EqualError(t, err, "观察期内未发现本批会话在目标提供商完成请求")
	require.Equal(t, 0, result.SuccessfulRequests)
	require.Len(t, fixture.batches, 1)
}

func newTrafficFixture() *trafficFixture {
	f := &trafficFixture{record: &provider.Record{
		ID: 91, Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeOAuth,
		Status: provider.StatusActive, Schedulable: true, Concurrency: 10, GroupIDs: []int64{18},
	}, now: time.Now}
	for _, id := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		f.candidates = append(f.candidates, Session{
			ID: id, SourceID: 50, UserID: 1, APIKeyID: 2, Models: []string{"gpt-5"},
		})
	}
	return f
}

func (f *trafficFixture) GetByID(context.Context, int64) (*provider.Record, error) {
	return f.record, nil
}

func (f *trafficFixture) GetByIDLite(_ context.Context, id int64) (*routing.Group, error) {
	return &routing.Group{ID: id, Status: routing.StatusActive}, nil
}

func (f *trafficFixture) Candidates(context.Context, int64, int64, time.Time) ([]Session, error) {
	return f.candidates, nil
}

func (f *trafficFixture) Observe(context.Context, int64, int64, time.Time) ([]CompletedRequest, int, error) {
	return f.requests, f.status, nil
}

func (f *trafficFixture) GetProviderConcurrency(context.Context, int64) (int, error) {
	return f.concurrent, nil
}

func (f *trafficFixture) EligibleOpenAISessionMoves(_ context.Context, _ int64, moves []scheduler.OpenAISessionMove, limit int) ([]scheduler.OpenAISessionMove, error) {
	return moves[:min(limit, len(moves))], nil
}

func (f *trafficFixture) ApplyOpenAISessionMoves(_ context.Context, batch scheduler.OpenAISessionMoveBatch) (scheduler.OpenAISessionMoveResult, error) {
	f.batches = append(f.batches, batch.Moves)
	moved := make([]bool, len(batch.Moves))
	for index := range moved {
		moved[index] = true
	}
	return scheduler.OpenAISessionMoveResult{Available: true, AppliedAt: f.now(), Moved: moved}, nil
}

func trafficTestModelRules(record *provider.Record) *provider.ModelRulesSnapshot {
	return provider.PrepareModelRules(record, provider.ModelMappingDefaults{}, provider.ModelPlatformRules{})
}
