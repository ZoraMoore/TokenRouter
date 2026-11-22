//go:build integration

package postgres

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/provider/operation"
	"github.com/TokenFlux/TokenRouter/internal/testutil/postgrescontainer"
)

func TestOperationStoreResumesCheckpointAndReusesInFlightOperation(t *testing.T) {
	db := postgrescontainer.New(t)
	ctx := t.Context()
	store := NewOperationStore(db)

	queued, err := store.Enqueue(ctx, operation.Job{Kind: operation.Relogin, ProviderID: 42, ProviderName: "测试账号"})
	require.NoError(t, err)
	require.Equal(t, operation.Queued, queued.State)

	duplicate, err := store.Enqueue(ctx, operation.Job{Kind: operation.TrafficPush, ProviderID: 42, GroupID: 2})
	require.NoError(t, err)
	require.Equal(t, queued.ID, duplicate.ID)
	require.Equal(t, operation.Relogin, duplicate.Kind)

	claim, err := store.Claim(ctx, operation.Relogin)
	require.NoError(t, err)
	require.Equal(t, queued.ID, claim.ID)
	data := json.RawMessage(`{"external_operation_id":"test-operation"}`)
	require.NoError(t, store.SaveCheckpoint(ctx, claim.ID, "relogin_wait_sync", data))
	require.NoError(t, store.Finish(ctx, claim.ID, operation.Queued, "等待同步", operation.Result{}, 0))

	claim, err = NewOperationStore(db).Claim(ctx, operation.Relogin)
	require.NoError(t, err)
	require.Equal(t, "relogin_wait_sync", claim.Stage)
	require.JSONEq(t, string(data), string(claim.Data))
	require.NoError(t, store.Finish(ctx, claim.ID, operation.Succeeded, "", operation.Result{ProviderID: 87}, 0))

	history, err := store.List(ctx, 87)
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Equal(t, claim.ID, history[0].ID)
	require.Equal(t, operation.Succeeded, history[0].State)
	public, err := json.Marshal(history[0])
	require.NoError(t, err)
	require.NotContains(t, string(public), "test-operation")

	groupIDs := []int64{18, 19}
	push, err := store.Enqueue(ctx, operation.Job{Kind: operation.TrafficPush, ProviderID: 42, GroupIDs: groupIDs})
	require.NoError(t, err)
	require.Equal(t, groupIDs, push.TrafficGroups())
	claimedPush, err := NewOperationStore(db).Claim(ctx, operation.TrafficPush)
	require.NoError(t, err)
	require.Equal(t, push.ID, claimedPush.ID)
	require.Equal(t, groupIDs, claimedPush.TrafficGroups())

	unconfirmed := 1
	progress := operation.Result{
		ProviderID:          42,
		MigratedSessions:    2,
		PeakConcurrency:     3,
		TargetConcurrency:   5,
		UnconfirmedSessions: &unconfirmed,
	}
	require.NoError(t, store.Finish(ctx, push.ID, operation.Queued, "", progress, 0))
	resumed, err := NewOperationStore(db).Claim(ctx, operation.TrafficPush)
	require.NoError(t, err)
	require.Equal(t, progress, resumed.Result)

	unconfirmed = 0
	progress.PeakConcurrency = progress.TargetConcurrency
	require.NoError(t, store.Finish(ctx, push.ID, operation.Succeeded, "", progress, 0))
	completed, err := store.Get(ctx, push.ID)
	require.NoError(t, err)
	require.Equal(t, operation.Succeeded, completed.State)
	require.Equal(t, progress, completed.Result)
}
