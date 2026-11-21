//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/testutil/postgrescontainer"
)

// TestMaintenancePelicanRecheckResumesOneFrozenAccount 覆盖样本重放、分组模型映射和复测续跑。
func TestMaintenancePelicanRecheckResumesOneFrozenAccount(t *testing.T) {
	ctx := t.Context()
	db := postgrescontainer.New(t)
	store := NewMaintenanceStore(db, NewProviderStore(nil, db, ProviderStoreOptions{}), time.Minute)
	runID := queueFrozenPelicanSample(t, db, 21, 211)
	again, err := store.Recheck(ctx, 211)
	require.NoError(t, err)
	require.Equal(t, runID, again)
	_, err = store.Recheck(ctx, 212)
	require.ErrorIs(t, err, provider.ErrMaintenanceNotFound)
	work, err := store.Claim(ctx)
	require.NoError(t, err)
	require.Equal(t, "pelican", work.Trigger)
	require.Equal(t, "probe", work.Item.Stage)
	require.Equal(t, "alias", work.Plan.Model)
	require.Equal(t, "mapped", work.Item.Model)
	require.Equal(t, "gpt-6-astra", work.Item.ActualModel)
	require.EqualValues(t, 21, work.Item.ProviderID)
	require.EqualValues(t, 41, work.Item.GroupID)
	require.Positive(t, work.Guard.Revision)
	current, err := store.Guard(ctx, 21, 41)
	require.NoError(t, err)
	require.NoError(t, store.Finish(ctx, work, provider.MaintenanceUpdate{
		MaintenanceResult: provider.MaintenanceResult{Stage: "candy", Probe: &provider.StateProbeResult{Verdict: "degraded"}},
		Guard:             current,
	}))
	restarted := NewMaintenanceStore(db, NewProviderStore(nil, db, ProviderStoreOptions{}), time.Minute)
	second, err := restarted.Claim(ctx)
	require.NoError(t, err)
	require.Equal(t, "candy", second.Item.Stage)
	require.NoError(t, restarted.Finish(ctx, second, provider.MaintenanceUpdate{
		MaintenanceResult: provider.MaintenanceResult{Outcome: "suspended", Answer: "28"}, Action: "suspend", Guard: current,
	}))
	detail, err := store.Run(ctx, runID)
	require.NoError(t, err)
	require.Equal(t, "finished", detail.Status)
	require.EqualValues(t, 211, detail.PelicanResultID)
	require.Equal(t, "29", detail.Items[0].InitialAnswer)
	require.Equal(t, "28", detail.Items[0].Answer)
	var on bool
	require.NoError(t, db.QueryRowContext(ctx, "SELECT schedulable FROM providers WHERE id=21").Scan(&on))
	require.False(t, on)
}

// queueFrozenPelicanSample 为独立账号保存错答样本，重放入队请求并核对同一事务中的运行编号。
func queueFrozenPelicanSample(t *testing.T, db *sql.DB, providerID, resultID int64) int64 {
	t.Helper()
	ctx := t.Context()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `INSERT INTO groups(id,name) VALUES (41,'group'),(42,'other') ON CONFLICT DO NOTHING`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `INSERT INTO providers(id,name,platform,type) VALUES ($1,'account','openai','oauth')`, providerID)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `INSERT INTO provider_groups(provider_id,group_id) VALUES ($1,41),($1,42)`, providerID)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `INSERT INTO pelican_test_plans(id,group_id,model_id,question_kind,cron_expression,enabled,suspend_on_wrong_answer)
		VALUES (1,41,'alias','candy','0 0 * * *',false,true) ON CONFLICT DO NOTHING`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `INSERT INTO pelican_candy_attempts(round_id,provider_id,plan_id,group_id,provider_revision,group_updated_at,membership_created_at,request_model,actual_model)
		SELECT 'round',p.id,1,41,p.pelican_candy_revision,g.updated_at,pg.created_at,'mapped','gpt-6-astra'
		FROM groups g JOIN provider_groups pg ON pg.group_id=g.id JOIN providers p ON p.id=pg.provider_id WHERE g.id=41 AND p.id=$1`, providerID)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `INSERT INTO pelican_test_results(id,plan_id,provider_id,model_id,question_kind,status,detail_code,prompt,criteria,answer,tested_at,latency_ms,round_id,schedule_action)
		VALUES ($1,1,$2,'alias','candy','suspected','answer_mismatch','','[]','29',now(),10,'round','recheck_queued')`, resultID, providerID)
	require.NoError(t, err)
	id, err := QueuePelicanRecheck(ctx, tx, resultID)
	require.NoError(t, err)
	again, err := QueuePelicanRecheck(ctx, tx, resultID)
	require.NoError(t, err)
	require.Equal(t, id, again)
	require.NoError(t, tx.Commit())
	return id
}

// TestPelicanRecheckOutcomes 从初测入队运行到最终调度，记录各分支实际派发的检测。
func TestPelicanRecheckOutcomes(t *testing.T) {
	db := postgrescontainer.New(t)
	ctx := t.Context()
	store := NewMaintenanceStore(db, NewProviderStore(nil, db, ProviderStoreOptions{}), time.Minute)
	// 行号对应上游返回的不同结果，便于核对同一账号的请求次数。
	expected := []struct {
		outcome         string
		probes, candies int
	}{
		{"stable", 1, 0},
		{"candy_passed", 1, 1},
		{"suspended", 1, 1},
		{"inconclusive", 1, 1},
		{"inconclusive", 1, 0},
		{"inconclusive", 0, 0},
		{"inconclusive", 1, 1},
		{"inconclusive", 1, 1},
		{"inconclusive", 1, 1},
		{"inconclusive", 1, 0},
	}
	runs := make([]int64, len(expected))
	for i := range expected {
		runs[i] = queueFrozenPelicanSample(t, db, int64(21+i), int64(211+i))
	}
	_, err := db.ExecContext(ctx, `UPDATE providers SET credentials='{"version":"new"}' WHERE id=26`)
	require.NoError(t, err)
	var mu sync.Mutex
	probes, candies := map[int64]int{}, map[int64]int{}
	service := provider.NewMaintenanceService(store, provider.MaintenancePlanOptions{})
	runner := provider.NewMaintenanceRunner(service, store, provider.MaintenanceRunnerOptions{
		Resolve: func(_ context.Context, draft provider.MaintenanceDraft, target provider.MaintenanceTarget) (provider.MaintenanceTarget, error) {
			assert.Equal(t, "alias", draft.Model)
			assert.EqualValues(t, 41, target.GroupID)
			return target, nil
		},
		Load: func(_ context.Context, id int64) (*provider.Record, error) {
			return &provider.Record{ID: id, Type: provider.ProviderTypeOAuth, Platform: provider.PlatformOpenAI}, nil
		},
		Acquire:         func(context.Context, *provider.Record) (func(), error) { return func() {}, nil },
		ActiveOperation: NewOperationStore(db).Active,
		Probe: func(_ context.Context, id int64, model string) (provider.StateProbeResult, error) {
			mu.Lock()
			probes[id]++
			mu.Unlock()
			assert.Equal(t, "mapped", model)
			s := provider.StateProbeResult{AccountID: id, Model: "gpt-6-astra", MintStatus: 200, ContinueStatus: 200, MintCompleted: true, ContinueCompleted: true, Minted: true, NewTicket: true, Verdict: "degraded"}
			switch id {
			case 21:
				s.Verdict, s.NewTicket = "healthy", false
			case 25:
				s.ContinueCompleted = false
			case 30:
				s.Model = ""
			}
			return s, nil
		},
		Candy: func(_ context.Context, id int64, model string) provider.PelicanEvidence {
			mu.Lock()
			candies[id]++
			mu.Unlock()
			assert.Equal(t, "mapped", model)
			s := provider.PelicanEvidence{ActualModel: "gpt-6-astra", Prompt: provider.PelicanQuestionDefaults()[1].QuestionPrompt, HTTPStatus: 200, Answer: "29", Status: "suspected", DetailCode: "answer_mismatch"}
			switch id {
			case 22:
				s.Answer, s.Status, s.DetailCode = "21", "passed", ""
			case 24:
				s.HTTPStatus, s.ActualModel = 429, ""
			case 27:
				s.ActualModel = "different"
			case 28:
				s.ActualModel = ""
			case 29:
				s.Status, s.DetailCode, s.Answer = "inconclusive", "incomplete", ""
			}
			return s
		},
	})
	require.NoError(t, runner.Start(ctx))
	t.Cleanup(func() {
		stop, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, runner.Stop(stop))
	})
	for i, want := range expected {
		id := int64(21 + i)
		require.EventuallyWithT(t, func(c *assert.CollectT) {
			run, err := store.Run(ctx, runs[i])
			require.NoError(c, err)
			require.Equal(c, "finished", run.Status)
			require.Equal(c, want.outcome, run.Items[0].Outcome)
			require.Equal(c, "29", run.Items[0].InitialAnswer)
			if want.candies > 0 {
				require.NotEmpty(c, run.Items[0].CandyPrompt)
			}
		}, 10*time.Second, 50*time.Millisecond)
		mu.Lock()
		nProbe, nCandy := probes[id], candies[id]
		mu.Unlock()
		require.Equal(t, want.probes, nProbe)
		require.Equal(t, want.candies, nCandy)
		var schedulable bool
		require.NoError(t, db.QueryRowContext(ctx, `SELECT schedulable FROM providers WHERE id=$1`, id).Scan(&schedulable))
		require.Equal(t, want.outcome != "suspended", schedulable)
	}
}
