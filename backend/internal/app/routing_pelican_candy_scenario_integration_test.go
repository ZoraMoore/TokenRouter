//go:build integration

package app

import (
	"context"
	"database/sql"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/provider"
	providerpostgres "github.com/TokenFlux/TokenRouter/internal/provider/postgres"
	"github.com/TokenFlux/TokenRouter/internal/routing"
	routingpostgres "github.com/TokenFlux/TokenRouter/internal/routing/postgres"
	"github.com/TokenFlux/TokenRouter/internal/testutil/postgrescontainer"
)

// TestCandyDispositionTransaction 跨领取者恢复轮次，校验分组样本和定向复检同事务提交。
func TestCandyDispositionTransaction(t *testing.T) {
	db := postgrescontainer.New(t)
	ctx := t.Context()
	// 在入队写入后注入错误，检验整个初测事务可以重试。
	fail := false
	queue := func(ctx context.Context, tx *sql.Tx, resultID int64) (int64, error) {
		id, err := providerpostgres.QueuePelicanRecheck(ctx, tx, resultID)
		if err == nil && fail {
			return 0, errors.New("测试复检入队失败")
		}
		return id, err
	}
	firstStore, secondStore := routingpostgres.NewPelicanTestStore(db), routingpostgres.NewPelicanTestStore(db)
	firstStore.BindCandyRecheck(queue)
	secondStore.BindCandyRecheck(queue)

	var group int64
	var groupUpdated time.Time
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO groups (name, status)
		VALUES ('candy-disposition','active') RETURNING id,updated_at`).Scan(&group, &groupUpdated))
	model := "gpt-6-astra"
	saved, err := firstStore.SavePlan(ctx, routing.PelicanPlan{
		GroupID: group, ModelID: model, QuestionKind: "candy", CronExpression: "0 * * * *", SuspendOnWrongAnswer: true,
	})
	require.NoError(t, err)
	var accounts []int64
	for _, name := range []string{"candy-first", "candy-second", "candy-third"} {
		var id int64
		require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO providers(name,platform,type,status,schedulable)
			VALUES ($1,'openai','oauth','active',true) RETURNING id`, name).Scan(&id))
		_, err := db.ExecContext(ctx, `INSERT INTO provider_groups(provider_id,group_id) VALUES ($1,$2)`, id, group)
		require.NoError(t, err)
		accounts = append(accounts, id)
	}
	schedulable := func(id int64) bool {
		t.Helper()
		var value bool
		require.NoError(t, db.QueryRowContext(ctx, `SELECT schedulable FROM providers WHERE id=$1`, id).Scan(&value))
		return value
	}
	claim := func(store *routingpostgres.PelicanTestStore, token string) routing.PelicanPlan {
		t.Helper()
		now := time.Now()
		plan, err := store.ClaimNext(ctx, now, now.Add(5*time.Minute), token)
		require.NoError(t, err)
		require.NotNil(t, plan)
		return *plan
	}
	begin := func(store *routingpostgres.PelicanTestStore, plan routing.PelicanPlan, roundID string, id int64) {
		t.Helper()
		ok, err := store.BeginCandyAttempt(ctx, plan, roundID, routing.PelicanTarget{ProviderID: id, ModelID: model, ActualModel: model, GroupUpdatedAt: groupUpdated})
		require.NoError(t, err)
		require.True(t, ok)
	}
	wrong := func(id int64) routing.PelicanResult {
		return routing.PelicanResult{ProviderID: &id, Answer: "20", PelicanSummary: routing.PelicanSummary{
			Status: "suspected", DetailCode: "answer_mismatch", TestedAt: time.Now(),
		}}
	}

	require.NoError(t, firstStore.QueuePlan(ctx, saved.ID))
	first := claim(firstStore, "first-candy-lease")
	round, err := firstStore.OpenCandyRound(ctx, first, accounts)
	require.NoError(t, err)
	begin(firstStore, first, round.ID, accounts[0])
	_, err = db.ExecContext(ctx, `UPDATE pelican_test_plans SET lease_until=NOW()-interval '1 second' WHERE id=$1`, first.ID)
	require.NoError(t, err)
	second := claim(secondStore, "second-candy-lease")
	reopened, err := secondStore.OpenCandyRound(ctx, second, accounts[1:])
	require.NoError(t, err)
	require.Equal(t, accounts, reopened.CandidateIDs)
	require.Equal(t, []int64{accounts[0]}, reopened.TriedIDs)
	_, err = firstStore.FinishCandyAttempt(ctx, first, round.ID, wrong(accounts[0]), nil)
	require.ErrorIs(t, err, routing.ErrPelicanConflict)
	require.True(t, schedulable(accounts[0]))

	begin(secondStore, second, round.ID, accounts[1])
	// 活跃上报可以和作答并行。复检入队失败时，初测作答和轮次进度一起回滚。
	_, err = db.ExecContext(ctx, `UPDATE providers SET last_used_at=NOW(), updated_at=NOW() WHERE id=$1`, accounts[1])
	require.NoError(t, err)
	fail = true
	_, err = secondStore.FinishCandyAttempt(ctx, second, round.ID, wrong(accounts[1]), nil)
	require.Error(t, err)
	require.True(t, schedulable(accounts[1]))
	history, err := firstStore.ListResults(ctx, first.ID)
	require.NoError(t, err)
	require.Empty(t, history)
	var runs int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM provider_maintenance_runs").Scan(&runs))
	require.Zero(t, runs)
	fail = false
	action, err := secondStore.FinishCandyAttempt(ctx, second, round.ID, wrong(accounts[1]), nil)
	require.NoError(t, err)
	require.Equal(t, routing.PelicanScheduleRecheckQueued, action)
	require.True(t, schedulable(accounts[1]))
	var outbox int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM scheduler_outbox WHERE provider_id=$1 AND event_type='provider_changed'`, accounts[1]).Scan(&outbox))
	require.Zero(t, outbox)
	history, err = firstStore.ListResults(ctx, first.ID)
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Equal(t, round.ID, history[0].RoundID)
	require.NotNil(t, history[0].RecheckRunID)
	adminDetail, err := firstStore.Result(ctx, history[0].ID, nil)
	require.NoError(t, err)
	require.Equal(t, history[0].RecheckRunID, adminDetail.RecheckRunID)
	require.Equal(t, "20", adminDetail.Answer)
	detail, err := firstStore.Result(ctx, history[0].ID, []int64{group})
	require.NoError(t, err)
	require.Equal(t, routing.PelicanScheduleRecheckQueued, detail.ScheduleAction)
	require.Nil(t, detail.RecheckRunID)
	require.Nil(t, detail.ProviderID)
	require.Empty(t, detail.RoundID)

	third := claim(firstStore, "third-candy-lease")
	_, err = firstStore.OpenCandyRound(ctx, third, []int64{accounts[2]})
	require.NoError(t, err)
	begin(firstStore, third, round.ID, accounts[2])
	// 人工重新启用会推进版本。定向复检须使用初测时的版本。
	_, err = db.ExecContext(ctx, `UPDATE providers SET schedulable=false WHERE id=$1`, accounts[2])
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE providers SET schedulable=true WHERE id=$1`, accounts[2])
	require.NoError(t, err)
	action, err = firstStore.FinishCandyAttempt(ctx, third, round.ID, wrong(accounts[2]), nil)
	require.NoError(t, err)
	require.Equal(t, routing.PelicanScheduleRecheckQueued, action)
	afterThird, err := firstStore.ListResults(ctx, first.ID)
	require.NoError(t, err)
	require.Len(t, afterThird, 2)
	require.NotNil(t, afterThird[0].RecheckRunID)
	require.True(t, schedulable(accounts[2]))

	final := claim(secondStore, "last-candy-lease")
	_, err = secondStore.OpenCandyRound(ctx, final, nil)
	require.NoError(t, err)
	_, err = secondStore.FinishCandyAttempt(ctx, final, round.ID, routing.PelicanResult{PelicanSummary: routing.PelicanSummary{
		Status: "inconclusive", DetailCode: "candy_exhausted", TestedAt: time.Now(),
	}}, nil)
	require.NoError(t, err)
	plans, err := firstStore.ListPlans(ctx)
	require.NoError(t, err)
	require.Len(t, plans, 1)
	require.Equal(t, "idle", plans[0].RunState)

	maintenance := providerpostgres.NewMaintenanceStore(db, providerpostgres.NewProviderStore(nil, db, providerpostgres.ProviderStoreOptions{}), time.Minute)
	service := provider.NewMaintenanceService(maintenance, provider.MaintenancePlanOptions{})
	var candyCalls atomic.Int32
	runner := provider.NewMaintenanceRunner(service, maintenance, provider.MaintenanceRunnerOptions{
		Resolve: func(_ context.Context, _ provider.MaintenanceDraft, target provider.MaintenanceTarget) (provider.MaintenanceTarget, error) {
			return target, nil
		},
		Load: func(_ context.Context, id int64) (*provider.Record, error) {
			return &provider.Record{ID: id, Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeOAuth}, nil
		},
		ActiveOperation: providerpostgres.NewOperationStore(db).Active,
		Acquire:         func(context.Context, *provider.Record) (func(), error) { return func() {}, nil },
		Probe: func(_ context.Context, id int64, requested string) (provider.StateProbeResult, error) {
			if id != accounts[1] || requested != model {
				return provider.StateProbeResult{}, errors.New("复检目标与初测不符")
			}
			return provider.StateProbeResult{
				AccountID: id, Model: model, Verdict: "healthy", Minted: true,
				MintCompleted: true, ContinueCompleted: true, MintStatus: 200, ContinueStatus: 200,
			}, nil
		},
		Candy: func(context.Context, int64, string) provider.PelicanEvidence {
			candyCalls.Add(1)
			return provider.PelicanEvidence{}
		},
	})
	require.NoError(t, runner.Start(ctx))
	t.Cleanup(func() {
		stop, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, runner.Stop(stop))
	})
	require.EventuallyWithT(t, func(check *assert.CollectT) {
		stable, err := maintenance.Run(ctx, *adminDetail.RecheckRunID)
		require.NoError(check, err)
		require.Equal(check, 1, stable.Completed)
		require.Equal(check, "stable", stable.Items[0].Outcome)
		require.Equal(check, "20", stable.Items[0].InitialAnswer)
		require.Empty(check, stable.Items[0].Answer)
		changed, err := maintenance.Run(ctx, *afterThird[0].RecheckRunID)
		require.NoError(check, err)
		require.Equal(check, 1, changed.Completed)
		require.Equal(check, "inconclusive", changed.Items[0].Outcome)
		require.Equal(check, "changed", changed.Items[0].Detail)
	}, 8*time.Second, 50*time.Millisecond)
	require.Zero(t, candyCalls.Load())
	require.True(t, schedulable(accounts[1]))
	require.True(t, schedulable(accounts[2]))
}
