package provider

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/provider/operation"
)

const reloginTestAccountID = "a123456789abcdef0123456789abcdef"

type reloginStoreStub struct {
	records map[int64]*Record
}

type reloginRemoteStub struct {
	account ReviveAccount
	state   string
	delete  func(string) (string, error)
	sync    func(string) (string, error)
}

func TestReloginRequiresRecognizedCurrentUpstream401(t *testing.T) {
	for _, tt := range []struct {
		name     string
		status   string
		message  string
		eligible bool
	}{
		{"gateway revocation", StatusError, "Token revoked (401): session revoked", true},
		{"probe unauthorized", StatusError, "Authentication failed (401): Unauthorized", true},
		{"legacy upstream", StatusError, "Upstream HTTP 401: Unauthorized", true},
		{"recovered status", StatusActive, "Token revoked (401): session revoked", false},
		{"refresh invalid grant", StatusError, "invalid_grant", false},
		{"unrelated auth failure", StatusError, "ReviveToken returned HTTP 401", false},
		{"403 includes a status from history", StatusError, "Forbidden (403): upstream HTTP 401 last week", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := &Record{Status: tt.status, Platform: PlatformOpenAI, Type: ProviderTypeOAuth, ErrorMessage: tt.message}
			require.Equal(t, tt.eligible, verifiedRelogin401(p))
		})
	}
}

func TestReloginCheckpointsWritesAndRestoresWithoutEnablingPausedProvider(t *testing.T) {
	ctx := context.Background()
	until := time.Now().Add(-time.Hour).Truncate(time.Second)
	old := &Record{
		ID: 7, Name: "archived", Platform: PlatformOpenAI, Type: ProviderTypeOAuth,
		Status: StatusError, ErrorMessage: "Token revoked (401): expired grant",
		Credentials: map[string]any{"email": "acct@example.org", "access_token": "prior-secret", "chatgpt_account_id": "workspace"},
		GroupIDs:    []int64{14, 21}, Concurrency: 8, Priority: 1, ExpiresAt: &until, AutoPauseOnExpired: true,
	}
	store := &reloginStoreStub{records: map[int64]*Record{7: old}}
	external := &reloginRemoteStub{
		account: ReviveAccount{ID: reloginTestAccountID, Email: "ACCT@example.org", CredentialSaved: true, PoolState: "present", ProviderID: &old.ID},
	}
	svc := &ReloginService{providers: store, remote: external}
	preview, err := svc.Preview(ctx, 7)
	require.NoError(t, err)
	require.True(t, preview.Eligible)
	job := operation.Job{ID: "a123456789abcdef0123456789abcdef", ProviderID: 7}
	checkpoint := func(stage string, data any) error {
		job.Stage = stage
		job.Data, err = json.Marshal(data)
		require.NoError(t, err)
		require.NotContains(t, string(job.Data), "prior-secret")
		return nil
	}

	var deleteKey string
	external.delete = func(key string) (string, error) {
		require.Equal(t, reloginDeletePending, job.Stage)
		require.Contains(t, string(job.Data), key)
		if deleteKey == "" {
			deleteKey = key
			return "", operation.RetryLater("response lost")
		}
		require.Equal(t, deleteKey, key)
		return "d123456789abcdef0123456789abcdef", nil
	}
	external.sync = func(key string) (string, error) {
		require.Equal(t, reloginSyncPending, job.Stage)
		require.Contains(t, string(job.Data), key)
		require.NotEqual(t, deleteKey, key)
		return "b123456789abcdef0123456789abcdef", nil
	}

	_, err = svc.Run(ctx, job, checkpoint)
	require.Error(t, err)
	require.Equal(t, reloginDeletePending, job.Stage)

	// 删除已生效时，响应丢失的重试仍使用同一个幂等键取回操作 ID。
	external.account.ProviderID = nil
	external.account.PoolState = "removed"
	delete(store.records, old.ID)
	_, err = svc.Run(ctx, job, checkpoint)
	require.Error(t, err)
	require.Equal(t, reloginWaitDelete, job.Stage)

	external.state = "running"
	_, err = svc.Run(ctx, job, checkpoint)
	require.Error(t, err)
	require.Equal(t, reloginWaitDelete, job.Stage)

	external.state = "succeeded"
	_, err = svc.Run(ctx, job, checkpoint)
	require.Error(t, err)
	require.Equal(t, reloginWaitSync, job.Stage)

	replacement := &Record{
		ID: 82, Name: "imported", Platform: PlatformOpenAI, Type: ProviderTypeOAuth,
		Status: StatusActive, Schedulable: true, Concurrency: 2,
		Credentials: map[string]any{"email": "acct@example.org", "access_token": "new-secret", "chatgpt_account_id": "workspace"},
	}
	store.records[replacement.ID] = replacement
	external.account.ProviderID = &replacement.ID
	external.account.PoolState = "present"
	result, err := svc.Run(ctx, job, checkpoint)
	require.NoError(t, err)
	require.Equal(t, replacement.ID, result.ProviderID)
	require.True(t, reloginSettingsOf(old).Matches(store.records[replacement.ID]))
	require.Equal(t, "new-secret", store.records[replacement.ID].GetCredential("access_token"))
}

func TestReloginRefusesDeletionAfterBindingChanges(t *testing.T) {
	old := &Record{ID: 7, Name: "old", Platform: PlatformOpenAI, Type: ProviderTypeOAuth, Status: StatusError, ErrorMessage: "Token revoked (401)", Credentials: map[string]any{"email": "acct@example.org"}}
	external := &reloginRemoteStub{account: ReviveAccount{ID: reloginTestAccountID, Email: "acct@example.org", CredentialSaved: true, PoolState: "present", ProviderID: &old.ID}}
	store := &reloginStoreStub{records: map[int64]*Record{old.ID: old}}
	svc := &ReloginService{providers: store, remote: external}
	job := operation.Job{ID: "a123456789abcdef0123456789abcdef", ProviderID: old.ID}
	_, err := svc.Run(context.Background(), job, func(stage string, data any) error {
		id := int64(80)
		external.account.ProviderID = &id
		return nil
	})
	require.ErrorContains(t, err, "绑定")
}

func TestScheduledReloginKeepsReplacementPaused(t *testing.T) {
	replacement := &Record{
		ID: 82, Platform: PlatformOpenAI, Type: ProviderTypeOAuth, Status: StatusActive, Concurrency: 2,
		Credentials: map[string]any{"email": "acct@example.org"},
	}
	store := &reloginStoreStub{records: map[int64]*Record{82: replacement}}
	remote := &reloginRemoteStub{account: ReviveAccount{
		ID: reloginTestAccountID, Email: "acct@example.org", CredentialSaved: true, PoolState: "present", ProviderID: &replacement.ID,
	}}
	state, err := json.Marshal(reloginState{
		Email: "acct@example.org", AccountID: reloginTestAccountID, ReplacementID: 82,
		Settings:            ReloginSettings{Schedulable: true, Concurrency: 8},
		ReplacementSettings: reloginSettingsOf(replacement),
	})
	require.NoError(t, err)
	job := operation.Job{ProviderID: 7, ExpectedCredentialVersion: "observed", Stage: reloginRestore, Data: state}
	service := &ReloginService{providers: store, remote: remote}
	_, err = service.Run(t.Context(), job, func(string, any) error { return nil })
	require.NoError(t, err)
	require.False(t, replacement.Schedulable)
	require.Equal(t, 8, replacement.Concurrency)
}

// TestScheduledReloginStartsWith401SchedulingOff 检查确认 401 的停调账号可以提交重登。
func TestScheduledReloginStartsWith401SchedulingOff(t *testing.T) {
	old := &Record{
		ID: 7, Platform: PlatformOpenAI, Type: ProviderTypeOAuth, Status: StatusError,
		ErrorMessage: "Token revoked (401)", Credentials: map[string]any{"email": "acct@example.org"},
	}
	remote := &reloginRemoteStub{
		account: ReviveAccount{
			ID: reloginTestAccountID, Email: "acct@example.org", CredentialSaved: true, PoolState: "present", ProviderID: &old.ID,
		},
		delete: func(key string) (string, error) {
			require.NotEmpty(t, key)
			return "d123456789abcdef0123456789abcdef", nil
		},
	}
	service := &ReloginService{providers: &reloginStoreStub{records: map[int64]*Record{7: old}}, remote: remote}
	job := operation.Job{ID: reloginTestAccountID, ProviderID: old.ID, ExpectedCredentialVersion: RefreshCredentialIdentity(old)}
	var stage string
	_, err := service.Run(t.Context(), job, func(name string, data any) error {
		stage = name
		state, ok := data.(reloginState)
		require.True(t, ok)
		require.False(t, state.Settings.Schedulable)
		return nil
	})
	require.ErrorContains(t, err, "等待 ReviveToken 完成号池绑定移除")
	require.Equal(t, reloginWaitDelete, stage)
}

func TestScheduledReloginDeclinesWhenObservedCredentialsChanged(t *testing.T) {
	old := &Record{
		ID: 7, Platform: PlatformOpenAI, Type: ProviderTypeOAuth, Status: StatusError,
		ErrorMessage: "Token revoked (401)", Schedulable: true,
		Credentials: map[string]any{"email": "acct@example.org"},
	}
	remote := &reloginRemoteStub{account: ReviveAccount{
		ID: reloginTestAccountID, Email: "acct@example.org", CredentialSaved: true, PoolState: "present", ProviderID: &old.ID,
	}}
	service := &ReloginService{providers: &reloginStoreStub{records: map[int64]*Record{7: old}}, remote: remote}
	_, err := service.Run(t.Context(), operation.Job{ProviderID: 7, ExpectedCredentialVersion: "old-credentials"},
		func(string, any) error {
			t.Fatal("凭据变化后开始了重登")
			return nil
		})
	require.ErrorContains(t, err, "凭据已改变")
}

func (s *reloginStoreStub) GetByID(_ context.Context, id int64) (*Record, error) {
	if record := s.records[id]; record != nil {
		return CloneRecord(record), nil
	}
	return nil, ErrProviderNotFound
}

func (s *reloginStoreStub) ListShadowsByParent(context.Context, int64) ([]*Record, error) {
	return nil, nil
}

func (s *reloginStoreStub) FindReloginProviders(_ context.Context, email string) ([]Record, error) {
	found := []Record{}
	for _, record := range s.records {
		if reloginEmail(record) == email {
			found = append(found, *CloneRecord(record))
		}
	}
	return found, nil
}

func (s *reloginStoreStub) ValidateGroupIDs(context.Context, []int64) error { return nil }

func (s *reloginStoreStub) RestoreReloginSettings(_ context.Context, id int64, email, credentialVersion string, expected, desired ReloginSettings) (bool, error) {
	r := s.records[id]
	if reloginEmail(r) != email || RefreshCredentialIdentity(r) != credentialVersion || !expected.Matches(r) {
		return false, errors.New("configuration changed")
	}
	r.GroupIDs = desired.GroupIDs
	r.Concurrency = desired.Concurrency
	r.Priority = desired.Priority
	r.LoadFactor = desired.LoadFactor
	r.ExpiresAt = desired.ExpiresAt
	r.AutoPauseOnExpired = desired.AutoPauseOnExpired
	r.Schedulable = desired.Schedulable
	return true, nil
}

func (s *reloginRemoteStub) Configured() bool { return true }
func (s *reloginRemoteStub) FindAccount(context.Context, string) (string, error) {
	return s.account.ID, nil
}

func (s *reloginRemoteStub) GetAccount(context.Context, string) (ReviveAccount, error) {
	return s.account, nil
}

func (s *reloginRemoteStub) DeletePoolAccount(_ context.Context, _ string, key string) (string, error) {
	return s.delete(key)
}

func (s *reloginRemoteStub) SyncAccount(_ context.Context, _ string, key string) (string, error) {
	return s.sync(key)
}

func (s *reloginRemoteStub) GetOperation(_ context.Context, id string) (ReviveOperation, error) {
	return ReviveOperation{ID: id, State: s.state, AccountID: s.account.ID}, nil
}
