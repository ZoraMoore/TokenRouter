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
	record       *provider.Record
	candidates   []Session
	requests     []CompletedRequest
	status       int
	concurrent   int
	now          func() time.Time
	batches      [][]scheduler.OpenAISessionMove
	batchGroups  []int64
	emptyGroup   int64
	requestGroup int64
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
	require.Equal(t, 10, result.TargetConcurrency)
	require.NotNil(t, result.UnconfirmedSessions)
	require.Equal(t, 2, *result.UnconfirmedSessions)
	require.Len(t, fixture.batches, 1)

	now = now.Add(10 * time.Second)
	fixture.concurrent = 3
	completedRequests := []CompletedRequest{
		{SessionID: "a", UserID: 1, APIKeyID: 2, StartedAt: now},
		{SessionID: "a", UserID: 1, APIKeyID: 2, StartedAt: now.Add(time.Second)},
	}
	fixture.requests = completedRequests
	result, err = push.Run(ctx, job, checkpoint)
	require.Error(t, err)
	require.Equal(t, 2, result.SuccessfulRequests)
	require.Equal(t, 3, result.PeakConcurrency)
	require.Equal(t, 1, *result.UnconfirmedSessions)
	require.Len(t, fixture.batches, 1)

	// 续跑从检查点读回已确认会话，即使这次没有读到成功请求。
	fixture.requests = nil
	result, err = push.Run(ctx, job, checkpoint)
	require.Error(t, err)
	require.Equal(t, 1, *result.UnconfirmedSessions)

	now = now.Add(trafficObservation)
	fixture.concurrent = 0
	fixture.requests = completedRequests
	result, err = push.Run(ctx, job, checkpoint)
	require.Error(t, err)
	require.Equal(t, 6, result.MigratedSessions)
	require.Equal(t, 5, *result.UnconfirmedSessions)
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
	require.Equal(t, 10, result.TargetConcurrency)
	require.Equal(t, 2, *result.UnconfirmedSessions)
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

// TestPushRotatesSelectedGroupsAfterCheckpoint 复用同一目标并发，并按分组隔离相同会话 ID。
func TestPushRotatesSelectedGroupsAfterCheckpoint(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	fixture := newTrafficFixture()
	fixture.record.GroupIDs = []int64{17, 18, 19}
	fixture.record.Concurrency = 3
	fixture.emptyGroup = 17
	push := New(fixture, fixture, fixture, fixture, fixture, trafficTestModelRules, nil)
	push.now = func() time.Time { return now }
	fixture.now = push.now
	job := operation.Job{ID: "job-multi", ProviderID: 91, GroupIDs: []int64{17, 18, 19}}
	checkpoint := func(stage string, data any) error {
		job.Stage = stage
		var err error
		job.Data, err = json.Marshal(data)
		return err
	}

	result, err := push.Run(ctx, job, checkpoint)
	require.Error(t, err)
	require.Equal(t, 2, result.MigratedSessions)
	require.Equal(t, []int64{18}, fixture.batchGroups)

	now = now.Add(trafficObservation + time.Second)
	fixture.concurrent = 2
	fixture.requestGroup = 18
	fixture.requests = []CompletedRequest{{SessionID: "a", UserID: 1, APIKeyID: 2, StartedAt: now}}
	result, err = push.Run(ctx, job, checkpoint)
	require.Error(t, err)
	require.Equal(t, []int64{18, 19}, fixture.batchGroups)
	require.Len(t, fixture.batches[1], 1)
	require.Equal(t, fixture.batches[0][0].Hash, fixture.batches[1][0].Hash)
	require.Equal(t, 3, result.MigratedSessions)
	require.Equal(t, 1, result.SuccessfulRequests)
	require.Equal(t, 2, result.PeakConcurrency)
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

func (f *trafficFixture) Candidates(_ context.Context, _ int64, groupID int64, _ time.Time) ([]Session, error) {
	if f.emptyGroup == groupID {
		return nil, nil
	}
	return f.candidates, nil
}

func (f *trafficFixture) Observe(_ context.Context, _ int64, groupID int64, _ time.Time) ([]CompletedRequest, int, error) {
	if f.requestGroup != 0 && f.requestGroup != groupID {
		return nil, 0, nil
	}
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
	f.batchGroups = append(f.batchGroups, batch.GroupID)
	moved := make([]bool, len(batch.Moves))
	for index := range moved {
		moved[index] = true
	}
	return scheduler.OpenAISessionMoveResult{Available: true, AppliedAt: f.now(), Moved: moved}, nil
}

func trafficTestModelRules(record *provider.Record) *provider.ModelRulesSnapshot {
	return provider.PrepareModelRules(record, provider.ModelMappingDefaults{}, provider.ModelPlatformRules{})
}
