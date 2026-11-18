package routing

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/provider"
)

type pelicanFinishFixture struct {
	PelicanRepository
	result PelicanResult
	next   *time.Time
}

type pelicanIdleRunnerFixture struct {
	PelicanRepository
	pruning, release, finished chan struct{}
	claimed                    chan time.Duration
}

func (f *pelicanIdleRunnerFixture) PruneShowcase(ctx context.Context) error {
	f.pruning <- struct{}{}
	select {
	case <-f.release:
		return nil
	case <-ctx.Done():
		close(f.finished)
		return ctx.Err()
	}
}

func (f *pelicanIdleRunnerFixture) ClaimNext(_ context.Context, now, until time.Time, _ string) (*PelicanPlan, error) {
	select {
	case f.claimed <- until.Sub(now):
	default:
	}
	return nil, nil
}

func TestPelicanRunnerPrunesWhenThereAreNoPlans(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		repo := &pelicanIdleRunnerFixture{
			pruning: make(chan struct{}, 2), claimed: make(chan time.Duration, 1), release: make(chan struct{}), finished: make(chan struct{}),
		}
		runner := NewPelicanRunner(NewPelicanService(repo, PelicanOptions{Now: time.Now}))
		require.NoError(t, runner.Start(t.Context()))
		defer func() { require.NoError(t, runner.Stop(t.Context())) }()
		synctest.Wait()
		require.Len(t, repo.pruning, 1)
		require.Len(t, repo.claimed, 1)
		require.Equal(t, 15*time.Minute, <-repo.claimed)
		repo.release <- struct{}{}
		synctest.Wait()
		time.Sleep(time.Hour)
		synctest.Wait()
		require.Len(t, repo.pruning, 2)
		require.NoError(t, runner.Stop(t.Context()))
		select {
		case <-repo.finished:
		default:
			t.Fatal("退出时应已回收清理任务")
		}
	})
}

func TestPelicanRunnerDeadlineByKind(t *testing.T) {
	for _, tt := range []struct {
		kind string
		wait time.Duration
	}{
		{"pelican", 10 * time.Minute},
		{"custom_svg", 10 * time.Minute},
		{"candy", 4 * time.Minute},
		{"custom_text", 4 * time.Minute},
		{PelicanStateProbe, 4 * time.Minute},
	} {
		t.Run(tt.kind, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				repo := &pelicanFinishFixture{}
				id := int64(4)
				s := NewPelicanService(repo, PelicanOptions{
					Now: time.Now,
					Acquire: func(ctx context.Context, _ PelicanPlan) (PelicanTarget, func(), error) {
						deadline, ok := ctx.Deadline()
						require.True(t, ok)
						require.Equal(t, tt.wait, time.Until(deadline))
						return PelicanTarget{}, nil, ErrPelicanNotFound
					},
				})
				require.NoError(t, s.finishRound(t.Context(), PelicanPlan{QuestionKind: tt.kind, ProviderID: &id}))
				require.Equal(t, "no_provider", repo.result.DetailCode)
			})
		})
	}
}

func (f *pelicanFinishFixture) Finish(_ context.Context, _ PelicanPlan, result PelicanResult, next *time.Time) error {
	f.result, f.next = result, next
	return nil
}

// TestPelicanRunnerUsesGroupModel 计划按分组别名展示，作答请求使用分组映射后的型号。
func TestPelicanRunnerUsesGroupModel(t *testing.T) {
	now := time.Date(2026, 11, 18, 9, 6, 0, 0, time.UTC)
	repo := &pelicanFinishFixture{}
	released := false
	service := NewPelicanService(repo, PelicanOptions{
		Now: func() time.Time { return now },
		Acquire: func(context.Context, PelicanPlan) (PelicanTarget, func(), error) {
			return PelicanTarget{ProviderID: 4, ModelID: "mapped", Protocol: "chat_completions"}, func() { released = true }, nil
		},
		Run: func(_ context.Context, providerID int64, model, kind, round, protocol string, config provider.PelicanQuestionConfig) provider.PelicanEvidence {
			require.Equal(t, int64(4), providerID)
			require.Equal(t, "mapped", model)
			require.Equal(t, "high", config.ReasoningEffort)
			require.Equal(t, "candy", kind)
			require.Contains(t, round, "09:00:00Z")
			require.Equal(t, "chat_completions", protocol)
			return provider.PelicanEvidence{Status: "passed", ReasoningEffort: "high", Answer: "21"}
		},
	})
	plan := PelicanPlan{ReasoningEffort: "high", ID: 3, GroupID: 2, Enabled: true, ModelID: "alias", QuestionKind: "candy", CronExpression: "0 * * * *", RoundAt: now.Truncate(time.Hour)}
	require.NoError(t, service.finishRound(t.Context(), plan))
	require.True(t, released)
	require.Equal(t, "high", repo.result.ReasoningEffort)
	require.Equal(t, "alias", repo.result.ModelID)
	require.Equal(t, "passed", repo.result.Status)
	require.Equal(t, int64(4), *repo.result.ProviderID)
	require.Equal(t, now.Truncate(time.Hour).Add(time.Hour), *repo.next)

	service.options.Acquire = func(context.Context, PelicanPlan) (PelicanTarget, func(), error) {
		return PelicanTarget{}, nil, ErrPelicanNotFound
	}
	plan.Enabled = false
	require.NoError(t, service.finishRound(t.Context(), plan))
	require.Equal(t, "inconclusive", repo.result.Status)
	require.Equal(t, "no_provider", repo.result.DetailCode)
	require.Empty(t, repo.result.Answer)
	require.Nil(t, repo.next)
}

// TestPelicanRunnerStateProbe 计划使用指定账号的双请求探针，保存可公开的观察结果。
func TestPelicanRunnerStateProbe(t *testing.T) {
	id := int64(12)
	now := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	plan := PelicanPlan{ID: 5, GroupID: 4, ProviderID: &id, ModelID: "group-model", QuestionKind: PelicanStateProbe, CronExpression: "0 * * * *", Enabled: true}
	for _, tc := range []struct {
		name, verdict, failure, status, detail string
		err                                    error
	}{
		{"normal", "healthy", "", "passed", "", nil},
		{"changed", "degraded", "", "suspected", "state_changed", nil},
		{"no_state", "inconclusive", "no_ticket", "inconclusive", "no_ticket", nil},
		{"unknown", "private_response", "private_failure", "inconclusive", "incomplete", nil},
		{"invalid_model", "", "", "inconclusive", "model_unsupported", provider.ErrIQUnsupported},
		{"canceled", "", "", "inconclusive", "timeout", context.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &pelicanFinishFixture{}
			var called, released int
			s := NewPelicanService(repo, PelicanOptions{
				Now: func() time.Time { return now },
				Acquire: func(_ context.Context, p PelicanPlan) (PelicanTarget, func(), error) {
					require.Equal(t, id, *p.ProviderID)
					return PelicanTarget{ProviderID: id, ModelID: "mapped-model"}, func() { released++ }, nil
				},
				RunStateProbe: func(_ context.Context, providerID int64, model string) (provider.StateProbeResult, error) {
					called++
					require.Equal(t, id, providerID)
					require.Equal(t, "mapped-model", model)
					return provider.StateProbeResult{
						AccountID: id, Verdict: tc.verdict, Failure: tc.failure,
						Reason: "private_upstream", ReportedModel: "private_model", Model: "private_model",
						MintStatus: 200, ContinueStatus: 200, TicketLength: 24, ContinueTicketLength: 12,
						MintLatencyMs: 34, ContinueLatencyMs: 18, Minted: true, NewTicket: tc.verdict == "degraded", MintCompleted: true, ContinueCompleted: true,
					}, tc.err
				},
			})
			require.NoError(t, s.finishRound(t.Context(), plan))
			require.Equal(t, 1, called)
			require.Equal(t, 1, released)
			require.Equal(t, tc.status, repo.result.Status)
			require.Equal(t, tc.detail, repo.result.DetailCode)
			require.Equal(t, "group-model", repo.result.ModelID)
			require.Equal(t, id, *repo.result.ProviderID)
			require.Equal(t, now.Add(time.Hour), *repo.next)
			require.Contains(t, repo.result.Answer, "HTTP 200")
			require.Contains(t, repo.result.Answer, "34 ms")
			require.Contains(t, repo.result.Answer, "24 字节")
			require.Contains(t, repo.result.Answer, "续接请求 HTTP 200")
			require.Contains(t, repo.result.Answer, "18 ms")
			require.NotContains(t, repo.result.Answer, "private")
			require.NotContains(t, repo.result.Prompt, "private")
		})
	}
}

func TestPelicanRunnerStateProbeNeverUsesAnotherProvider(t *testing.T) {
	id := int64(12)
	plan := PelicanPlan{ID: 5, GroupID: 4, ProviderID: &id, ModelID: "alias", QuestionKind: PelicanStateProbe}
	repo := &pelicanFinishFixture{}
	var selected, released int
	s := NewPelicanService(repo, PelicanOptions{
		Now: time.Now,
		Acquire: func(context.Context, PelicanPlan) (PelicanTarget, func(), error) {
			selected++
			return PelicanTarget{ProviderID: id + 1, ModelID: "other"}, func() { released++ }, nil
		},
	})
	require.NoError(t, s.finishRound(t.Context(), plan))
	require.Equal(t, 1, released)
	require.Nil(t, repo.result.ProviderID)
	require.Equal(t, "inconclusive", repo.result.Status)
	require.Equal(t, "no_provider", repo.result.DetailCode)

	plan.ProviderID = nil
	require.NoError(t, s.finishRound(t.Context(), plan))
	require.Equal(t, 1, selected)
	require.Nil(t, repo.result.ProviderID)
	require.Equal(t, "no_provider", repo.result.DetailCode)
}
