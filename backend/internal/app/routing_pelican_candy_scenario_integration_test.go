//go:build integration

package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	postgresinfra "github.com/TokenFlux/TokenRouter/internal/infra/postgres"
	providerpostgres "github.com/TokenFlux/TokenRouter/internal/provider/postgres"
	"github.com/TokenFlux/TokenRouter/internal/routing"
	routingpostgres "github.com/TokenFlux/TokenRouter/internal/routing/postgres"
	"github.com/TokenFlux/TokenRouter/internal/testutil/postgrescontainer"
)

type candyEvents struct {
	providerSchedulerEvents
	fail bool
}

func (b *candyEvents) Write(ctx context.Context, exec postgresinfra.Executor, event providerpostgres.ProviderEvent, id, group *int64, payload any) error {
	if b.fail {
		return errors.New("测试调度事件写入失败")
	}
	return b.providerSchedulerEvents.Write(ctx, exec, event, id, group, payload)
}

// TestCandyDispositionTransaction 跨领取者恢复轮次，校验停调和调度事件同事务提交。
func TestCandyDispositionTransaction(t *testing.T) {
	db := postgrescontainer.New(t)
	ctx := t.Context()
	events := &candyEvents{}
	providers := providerpostgres.NewProviderStore(nil, db, providerpostgres.ProviderStoreOptions{Events: events})
	firstStore, secondStore := routingpostgres.NewPelicanTestStore(db), routingpostgres.NewPelicanTestStore(db)
	firstStore.BindCandyDisabler(providers.DisableAfterCandyAnswer)
	secondStore.BindCandyDisabler(providers.DisableAfterCandyAnswer)

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
		ok, err := store.BeginCandyAttempt(ctx, plan, roundID, routing.PelicanTarget{ProviderID: id, GroupUpdatedAt: groupUpdated})
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
	// 活跃上报可以和作答并行。调度事件写入失败时，账号和作答一起回滚。
	_, err = db.ExecContext(ctx, `UPDATE providers SET last_used_at=NOW(), updated_at=NOW() WHERE id=$1`, accounts[1])
	require.NoError(t, err)
	events.fail = true
	_, err = secondStore.FinishCandyAttempt(ctx, second, round.ID, wrong(accounts[1]), nil)
	require.Error(t, err)
	require.True(t, schedulable(accounts[1]))
	history, err := firstStore.ListResults(ctx, first.ID)
	require.NoError(t, err)
	require.Empty(t, history)
	events.fail = false
	action, err := secondStore.FinishCandyAttempt(ctx, second, round.ID, wrong(accounts[1]), nil)
	require.NoError(t, err)
	require.Equal(t, routing.PelicanScheduleDisabled, action)
	require.False(t, schedulable(accounts[1]))
	var outbox int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM scheduler_outbox WHERE provider_id=$1 AND event_type='provider_changed'`, accounts[1]).Scan(&outbox))
	require.Equal(t, 1, outbox)
	history, err = firstStore.ListResults(ctx, first.ID)
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Equal(t, round.ID, history[0].RoundID)
	detail, err := firstStore.Result(ctx, history[0].ID, []int64{group})
	require.NoError(t, err)
	require.Equal(t, routing.PelicanScheduleDisabled, detail.ScheduleAction)
	require.Nil(t, detail.ProviderID)
	require.Empty(t, detail.RoundID)

	third := claim(firstStore, "third-candy-lease")
	_, err = firstStore.OpenCandyRound(ctx, third, []int64{accounts[2]})
	require.NoError(t, err)
	begin(firstStore, third, round.ID, accounts[2])
	// 人工重新启用会推进版本。迟到的错答留在历史。
	_, err = db.ExecContext(ctx, `UPDATE providers SET schedulable=false WHERE id=$1`, accounts[2])
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE providers SET schedulable=true WHERE id=$1`, accounts[2])
	require.NoError(t, err)
	action, err = firstStore.FinishCandyAttempt(ctx, third, round.ID, wrong(accounts[2]), nil)
	require.NoError(t, err)
	require.Equal(t, routing.PelicanScheduleAccountChanged, action)
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
}
