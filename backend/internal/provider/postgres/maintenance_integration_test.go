//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/provider/operation"
	"github.com/TokenFlux/TokenRouter/internal/testutil/postgrescontainer"
)

func TestMaintenanceStoreDeduplicatesDueRunsAndResumesLeasedWork(t *testing.T) {
	db := postgrescontainer.New(t)
	ctx := t.Context()
	first := NewMaintenanceStore(db, NewProviderStore(nil, db, ProviderStoreOptions{}), time.Minute)
	second := NewMaintenanceStore(db, NewProviderStore(nil, db, ProviderStoreOptions{}), time.Minute)
	now := time.Now().UTC()
	due := now.Add(-time.Minute)
	plan, err := first.SavePlan(ctx, provider.MaintenancePlan{
		MaintenanceDraft: provider.MaintenanceDraft{
			Name: "巡检", Kind: "iq", Scope: "providers", ProviderIDs: []int64{10, 11},
			Model: "gpt-6-astra", CronExpression: "*/5 * * * *", Enabled: true,
		},
		NextRunAt: &due,
	})
	require.NoError(t, err)
	targets := []provider.MaintenanceTarget{{ProviderID: 10}, {ProviderID: 11, Reason: "inactive"}}
	next := now.Add(5 * time.Minute)
	runID, err := first.QueueRun(ctx, plan, "schedule", *plan.NextRunAt, &next, targets)
	require.NoError(t, err)
	duplicate, err := second.QueueRun(ctx, plan, "schedule", *plan.NextRunAt, &next, targets)
	require.NoError(t, err)
	require.Equal(t, runID, duplicate)

	work, err := first.Claim(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 10, work.Item.ProviderID)
	_, err = second.Claim(ctx)
	require.ErrorIs(t, err, provider.ErrMaintenanceIdle)

	_, err = db.ExecContext(ctx, "UPDATE provider_maintenance_items SET lease_until=now()-interval '1 second' WHERE id=$1", work.ID)
	require.NoError(t, err)
	resumed, err := second.Claim(ctx)
	require.NoError(t, err)
	require.Equal(t, work.ID, resumed.ID)
	require.NotEqual(t, work.Lease, resumed.Lease)
	finish := provider.MaintenanceUpdate{MaintenanceResult: provider.MaintenanceResult{Outcome: "stable"}}
	require.ErrorIs(t, first.Finish(ctx, work, finish), provider.ErrMaintenanceChanged)
	require.NoError(t, second.Finish(ctx, resumed, finish))

	detail, err := first.Run(ctx, runID)
	require.NoError(t, err)
	require.Equal(t, "finished", detail.Status)
	require.Equal(t, 2, detail.Total)
	require.Equal(t, 2, detail.Completed)
	require.Equal(t, 0, detail.Remaining)
	require.Equal(t, "stable", detail.Items[0].Outcome)
	require.Equal(t, "skipped", detail.Items[1].Outcome)
}

func TestMaintenanceStoreTracksReloginAcrossPauseAndReplacement(t *testing.T) {
	db := postgrescontainer.New(t)
	ctx := t.Context()
	_, err := db.ExecContext(ctx, `
		INSERT INTO providers (id,name,platform,type,status,schedulable,error_message)
		VALUES (61,'old','openai','oauth','error',FALSE,'Upstream HTTP 401: expired'),
		       (62,'new','openai','oauth','active',FALSE,''),
		       (63,'other','openai','oauth','active',TRUE,'')`)
	require.NoError(t, err)
	providers := NewProviderStore(nil, db, ProviderStoreOptions{})
	store := NewMaintenanceStore(db, providers, time.Minute)
	ops := NewOperationStore(db)
	plan, err := store.SavePlan(ctx, provider.MaintenancePlan{MaintenanceDraft: provider.MaintenanceDraft{
		Name: "重登", Kind: "relogin", Scope: "providers", ProviderIDs: []int64{61, 62, 63},
		CronExpression: "*/15 * * * *", Enabled: true,
	}, NextRunAt: new(time.Now().Add(time.Minute))})
	require.NoError(t, err)
	runID, err := store.QueueRun(ctx, plan, "manual", time.Now(), nil, []provider.MaintenanceTarget{{ProviderID: 61}, {ProviderID: 63}})
	require.NoError(t, err)

	work, err := store.Claim(ctx)
	require.NoError(t, err)
	snapshot, err := store.Guard(ctx, 61, 0)
	require.NoError(t, err)
	update := provider.MaintenanceUpdate{
		MaintenanceResult: provider.MaintenanceResult{HTTPStatus: 401},
		Action:            "begin_relogin", Guard: snapshot, CredentialVersion: "observed-credentials",
	}
	require.NoError(t, store.Finish(ctx, work, update))
	job, err := ops.Claim(ctx, operation.Relogin)
	require.NoError(t, err)
	require.Equal(t, "observed-credentials", job.ExpectedCredentialVersion)
	require.NoError(t, ops.SaveCheckpoint(ctx, job.ID, "relogin_wait_sync", json.RawMessage(`{"operation":"submitted"}`)))

	plan.Enabled, plan.NextRunAt = false, nil
	_, err = store.SavePlan(ctx, plan)
	require.NoError(t, err)
	during, err := store.Run(ctx, runID)
	require.NoError(t, err)
	require.Equal(t, job.ID, during.Items[0].OperationID)
	require.Equal(t, "cancelled", during.Items[1].State)
	require.Equal(t, 1, during.Running)
	require.NoError(t, ops.Finish(ctx, job.ID, operation.Succeeded, "", operation.Result{ProviderID: 62}, 0))

	restarted := NewMaintenanceStore(db, providers, time.Minute)
	work, err = restarted.Claim(ctx)
	require.NoError(t, err)
	require.Equal(t, job.ID, work.Item.OperationID)
	require.Equal(t, "relogin", work.Item.Stage)
	require.NoError(t, restarted.Finish(ctx, work, provider.MaintenanceUpdate{
		MaintenanceResult: provider.MaintenanceResult{Stage: "verify"}, ReplacementID: 62,
	}))
	updated, err := restarted.Plan(ctx, plan.ID)
	require.NoError(t, err)
	require.Equal(t, []int64{62, 63}, updated.ProviderIDs)

	verify, err := restarted.Claim(ctx)
	require.NoError(t, err)
	require.Equal(t, "verify", verify.Item.Stage)
	fresh, err := restarted.Guard(ctx, 62, 0)
	require.NoError(t, err)
	require.NoError(t, restarted.Finish(ctx, verify, provider.MaintenanceUpdate{
		Guard: fresh, MaintenanceResult: provider.MaintenanceResult{Outcome: "relogin_succeeded", HTTPStatus: 200},
	}))
	detail, err := restarted.Run(ctx, runID)
	require.NoError(t, err)
	require.Equal(t, "relogin_succeeded", detail.Items[0].Outcome)
	require.EqualValues(t, 62, detail.Items[0].ReplacementID)
	var schedulable bool
	require.NoError(t, db.QueryRowContext(ctx, "SELECT schedulable FROM providers WHERE id=62").Scan(&schedulable))
	require.False(t, schedulable)
}

func TestMaintenanceStoreClearsOnlyTheObserved401(t *testing.T) {
	db := postgrescontainer.New(t)
	ctx := t.Context()
	_, err := db.ExecContext(ctx, `
		INSERT INTO providers (id,name,platform,type,status,schedulable,error_message,rate_limit_reset_at)
		VALUES (61,'recovered','openai','oauth','error',FALSE,'Upstream HTTP 401: expired',now()+interval '1 hour'),
		       (62,'changed','openai','oauth','error',TRUE,'Upstream HTTP 401: expired',NULL)`)
	require.NoError(t, err)
	store := NewMaintenanceStore(db, NewProviderStore(nil, db, ProviderStoreOptions{}), time.Minute)
	plan, err := store.SavePlan(ctx, provider.MaintenancePlan{MaintenanceDraft: provider.MaintenanceDraft{
		Name: "复检", Kind: "relogin", Scope: "providers", ProviderIDs: []int64{61, 62}, CronExpression: "*/15 * * * *",
	}})
	require.NoError(t, err)
	runID, err := store.QueueRun(ctx, plan, "manual", time.Now(), nil, []provider.MaintenanceTarget{{ProviderID: 61}, {ProviderID: 62}})
	require.NoError(t, err)
	for range 2 {
		work, err := store.Claim(ctx)
		require.NoError(t, err)
		guard, err := store.Guard(ctx, work.Item.ProviderID, 0)
		require.NoError(t, err)
		if work.Item.ProviderID == 62 {
			_, err := db.ExecContext(ctx, `UPDATE providers SET credentials='{"access_token":"updated"}' WHERE id=62`)
			require.NoError(t, err)
		}
		require.NoError(t, store.Finish(ctx, work, provider.MaintenanceUpdate{
			Guard: guard, Action: "clear_401",
			MaintenanceResult: provider.MaintenanceResult{Outcome: "auth_recovered", HTTPStatus: 200},
		}))
	}
	detail, err := store.Run(ctx, runID)
	require.NoError(t, err)
	require.Equal(t, "auth_recovered", detail.Items[0].Outcome)
	require.Equal(t, "changed", detail.Items[1].Detail)
	var status string
	var stillLimited, schedulable bool
	require.NoError(t, db.QueryRowContext(ctx, "SELECT status, rate_limit_reset_at>now(), schedulable FROM providers WHERE id=61").
		Scan(&status, &stillLimited, &schedulable))
	require.Equal(t, "active", status)
	require.True(t, stillLimited)
	require.False(t, schedulable)
	require.NoError(t, db.QueryRowContext(ctx, "SELECT status FROM providers WHERE id=62").Scan(&status))
	require.Equal(t, "error", status)
}

func TestMaintenanceRunnerChecksAllAccountsAndSuspendsOnlyConfirmedWrongAnswer(t *testing.T) {
	db := postgrescontainer.New(t)
	ctx := t.Context()
	_, err := db.ExecContext(ctx, `
		INSERT INTO groups (id,name) VALUES (41,'test'),(42,'other');
		INSERT INTO providers (id,name,platform,type) VALUES
			(21,'stable','openai','oauth'),(22,'passes','openai','oauth'),
			(23,'wrong','openai','oauth'),(24,'incomplete','openai','oauth'),(25,'candy-incomplete','openai','oauth');
		INSERT INTO provider_groups (provider_id,group_id) VALUES (21,41),(22,41),(23,41),(24,41),(25,41),(23,42)`)
	require.NoError(t, err)
	store := NewMaintenanceStore(db, NewProviderStore(nil, db, ProviderStoreOptions{}), time.Minute)
	plan, err := store.SavePlan(ctx, provider.MaintenancePlan{MaintenanceDraft: provider.MaintenanceDraft{
		Name: "巡检", Kind: "iq", Scope: "groups", GroupIDs: []int64{41, 42}, Model: "gpt-6-astra", CronExpression: "0 */6 * * *",
	}})
	require.NoError(t, err)
	var targets []provider.MaintenanceTarget
	for id := int64(21); id <= 25; id++ {
		targets = append(targets, provider.MaintenanceTarget{ProviderID: id, GroupID: 41, Model: plan.Model, ActualModel: plan.Model})
	}
	runID, err := store.QueueRun(ctx, plan, "manual", time.Now(), nil, targets)
	require.NoError(t, err)

	service := provider.NewMaintenanceService(store, provider.MaintenancePlanOptions{})
	runner := provider.NewMaintenanceRunner(service, store, provider.MaintenanceRunnerOptions{
		Resolve: func(_ context.Context, _ provider.MaintenanceDraft, item provider.MaintenanceTarget) (provider.MaintenanceTarget, error) {
			return item, nil
		},
		Load: func(_ context.Context, id int64) (*provider.Record, error) {
			return &provider.Record{ID: id, Type: provider.ProviderTypeOAuth, Platform: provider.PlatformOpenAI}, nil
		},
		Acquire:         func(context.Context, *provider.Record) (func(), error) { return func() {}, nil },
		ActiveOperation: NewOperationStore(db).Active,
		Probe: func(_ context.Context, id int64, model string) (provider.StateProbeResult, error) {
			sample := provider.StateProbeResult{
				AccountID: id, Model: model, Verdict: "healthy", MintCompleted: true, ContinueCompleted: true,
				Minted: true, MintStatus: 200, ContinueStatus: 200,
			}
			switch id {
			case 22, 23, 25:
				sample.Verdict, sample.NewTicket = "degraded", true
			case 24:
				sample.ContinueCompleted, sample.Failure = false, "stream_error"
			}
			return sample, nil
		},
		Candy: func(_ context.Context, id int64, _ string) provider.PelicanEvidence {
			switch id {
			case 22:
				return provider.PelicanEvidence{Status: "passed", Answer: "21", HTTPStatus: 200}
			case 23:
				return provider.PelicanEvidence{Status: "suspected", DetailCode: "answer_mismatch", Answer: "18", HTTPStatus: 200}
			}
			return provider.PelicanEvidence{Status: "inconclusive", DetailCode: "incomplete", HTTPStatus: 200}
		},
	})
	require.NoError(t, runner.Start(ctx))
	t.Cleanup(func() {
		stop, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, runner.Stop(stop))
	})
	require.EventuallyWithT(t, func(check *assert.CollectT) {
		detail, err := store.Run(ctx, runID)
		require.NoError(check, err)
		require.Equal(check, 5, detail.Completed)
		require.Equal(check, "stable", detail.Items[0].Outcome)
		require.Equal(check, "candy_passed", detail.Items[1].Outcome)
		require.Equal(check, "suspended", detail.Items[2].Outcome)
		require.Equal(check, "18", detail.Items[2].Answer)
		require.Equal(check, 200, detail.Items[2].HTTPStatus)
		require.Equal(check, "inconclusive", detail.Items[3].Outcome)
		require.Equal(check, "inconclusive", detail.Items[4].Outcome)
		require.Equal(check, "incomplete", detail.Items[4].Detail)
		require.Equal(check, "candy", detail.Items[4].Stage)
	}, 8*time.Second, 50*time.Millisecond)
	var schedulable int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM providers WHERE id BETWEEN 21 AND 25 AND schedulable").Scan(&schedulable))
	require.Equal(t, 4, schedulable)
	var wrong bool
	require.NoError(t, db.QueryRowContext(ctx, "SELECT schedulable FROM providers WHERE id=23").Scan(&wrong))
	require.False(t, wrong)
}
