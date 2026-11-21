package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/provider"
	providerpostgres "github.com/TokenFlux/TokenRouter/internal/provider/postgres"
	"github.com/TokenFlux/TokenRouter/internal/routing"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
)

type maintenanceTargetFixture struct {
	groups  map[int64]*routing.Group
	members map[int64][]provider.Record
	records map[int64]*provider.Record
}

// TestMaintenanceTargetsGroupOrder 检查跨组账号选择首个可用分组，并分别保存两层模型映射。
func TestMaintenanceTargetsGroupOrder(t *testing.T) {
	records := []provider.Record{
		{
			ID: 11, Name: "待测账号", Status: provider.StatusActive, Schedulable: true,
			Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeOAuth,
			Credentials: map[string]any{"model_mapping": map[string]any{"group-text": "gpt-6-astra"}},
		},
		{
			ID: 22, Name: "API Key", Status: provider.StatusActive, Schedulable: true,
			Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeAPIKey,
		},
	}
	fixture := maintenanceTargetFixture{
		groups: map[int64]*routing.Group{
			1: {
				ID: 1, Status: routing.StatusActive,
				AllowedProtocols: []capability.ProtocolID{capability.ProtocolEmbeddings},
			},
			2: {
				ID: 2, Status: routing.StatusActive,
				AllowedProtocols: []capability.ProtocolID{capability.ProtocolOpenAIResponses},
				RoutingPolicy: routing.GroupRoutingPolicy{
					Enabled: true, ModelMapping: map[string]string{"public-model": "group-text"},
				},
			},
			3: {
				ID: 3, Status: routing.StatusActive,
				AllowedProtocols: []capability.ProtocolID{capability.ProtocolOpenAIResponses},
				RoutingPolicy: routing.GroupRoutingPolicy{
					Enabled: true, ModelMapping: map[string]string{"public-model": "gpt-6-sol"},
				},
			},
		},
		members: map[int64][]provider.Record{1: records, 2: records, 3: records},
	}
	targets := maintenanceTargets{providers: fixture, groups: fixture}
	draft := provider.MaintenanceDraft{Kind: "iq", Scope: "groups", GroupIDs: []int64{1, 2, 3}, Model: "public-model"}
	got, err := targets.list(t.Context(), draft)

	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, int64(11), got[0].ProviderID)
	require.Equal(t, int64(2), got[0].GroupID)
	require.Equal(t, "group-text", got[0].Model)
	require.Equal(t, "gpt-6-astra", got[0].ActualModel)
	require.Empty(t, got[0].Reason)
	require.Equal(t, "unsupported", got[1].Reason)
}

// TestMaintenanceTargetsExplicitModel 检查指定账号仍受全局模型名单和文字模型资格限制。
func TestMaintenanceTargetsExplicitModel(t *testing.T) {
	record := &provider.Record{
		ID: 11, Status: provider.StatusActive, Schedulable: true,
		Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeOAuth,
		OpenAIOAuthModelWhitelist: func(context.Context) (provider.OpenAIOAuthModelPolicy, error) {
			return provider.NewOpenAIOAuthModelPolicy([]string{"gpt-6-astra", "gpt-image-1"}), nil
		},
	}
	fixture := maintenanceTargetFixture{records: map[int64]*provider.Record{record.ID: record}}
	targets := maintenanceTargets{providers: fixture, groups: fixture}
	for _, test := range []struct {
		model  string
		reason string
	}{
		{model: "gpt-6-astra"},
		{model: "gpt-6-sol", reason: "model_unavailable"},
		{model: "gpt-image-1", reason: "model_unavailable"},
	} {
		t.Run(test.model, func(t *testing.T) {
			draft := provider.MaintenanceDraft{Kind: "iq", Scope: "providers", ProviderIDs: []int64{record.ID}, Model: test.model}
			got, err := targets.list(t.Context(), draft)
			require.NoError(t, err)
			require.Len(t, got, 1)
			require.Equal(t, test.reason, got[0].Reason)
		})
	}
}

// TestMaintenanceTargetsResolveFrozenGroup 检查已冻结的账号或模型发生变化时，本轮不会改用其他分组。
func TestMaintenanceTargetsResolveFrozenGroup(t *testing.T) {
	record := &provider.Record{
		ID: 11, Status: provider.StatusActive, Schedulable: true,
		Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeOAuth, GroupIDs: []int64{1, 2},
	}
	group := &routing.Group{
		ID: 1, Status: routing.StatusActive,
		AllowedProtocols: []capability.ProtocolID{capability.ProtocolOpenAIResponses},
	}
	fixture := maintenanceTargetFixture{
		groups:  map[int64]*routing.Group{1: group},
		members: map[int64][]provider.Record{1: {*record}},
		records: map[int64]*provider.Record{record.ID: record},
	}
	targets := maintenanceTargets{providers: fixture, groups: fixture}
	draft := provider.MaintenanceDraft{Kind: "iq", Scope: "groups", GroupIDs: []int64{1, 2}, Model: "gpt-6-astra"}
	list, err := targets.list(t.Context(), draft)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Empty(t, list[0].Reason)

	group.RoutingPolicy = routing.GroupRoutingPolicy{Enabled: true, ModelMapping: map[string]string{"gpt-6-astra": "gpt-6-sol"}}
	mapped, err := targets.resolve(t.Context(), draft, list[0])
	require.NoError(t, err)
	require.Equal(t, "changed", mapped.Reason)

	record.GroupIDs = []int64{2}
	removed, err := targets.resolve(t.Context(), draft, list[0])
	require.NoError(t, err)
	require.Equal(t, "changed", removed.Reason)
	require.Equal(t, int64(1), removed.GroupID)
}

// TestMaintenanceTargetsReloginCandidates 检查 401 冷却和已停调的认证错误都能进复检名单。
func TestMaintenanceTargetsReloginCandidates(t *testing.T) {
	until := time.Now().Add(time.Minute)
	records := []provider.Record{
		{
			ID: 11, Status: provider.StatusActive, Schedulable: true, Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeOAuth,
			TempUnschedulableUntil: &until, TempUnschedulableReason: "OAuth 401: invalid token",
		},
		{
			ID: 12, Status: provider.StatusError, Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeOAuth,
			ErrorMessage: "Token revoked (401): provider authentication permanently revoked",
		},
		{
			ID: 13, Status: provider.StatusActive, Schedulable: true, Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeOAuth,
		},
	}
	fixture := maintenanceTargetFixture{
		groups:  map[int64]*routing.Group{1: {ID: 1, Status: routing.StatusActive}},
		members: map[int64][]provider.Record{1: records},
		records: map[int64]*provider.Record{12: &records[1], 13: &records[2]},
	}
	targets := maintenanceTargets{providers: fixture, groups: fixture}
	draft := provider.MaintenanceDraft{Kind: "relogin", Scope: "groups", GroupIDs: []int64{1}}
	got, err := targets.list(t.Context(), draft)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, int64(11), got[0].ProviderID)
	require.Empty(t, got[0].Reason)
	require.Equal(t, int64(12), got[1].ProviderID)
	require.Empty(t, got[1].Reason)

	draft.Scope = "providers"
	draft.ProviderIDs = []int64{12, 13}
	got, err = targets.list(t.Context(), draft)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Empty(t, got[0].Reason)
	require.Equal(t, "no_401", got[1].Reason)
	refreshed, err := targets.resolve(t.Context(), draft, got[0])
	require.NoError(t, err)
	require.Empty(t, refreshed.Reason)
}

// TestMaintenanceTargetsRespectInactiveAccounts 检查停调、停用与到期状态下的任务资格。
func TestMaintenanceTargetsRespectInactiveAccounts(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name   string
		kind   string
		status string
		expiry *time.Time
	}{
		{name: "测智跳过错误账号", kind: provider.MaintenanceIQ, status: provider.StatusError},
		{name: "重登跳过已停用账号", kind: provider.MaintenanceRelogin, status: provider.StatusDisabled},
		{name: "重登跳过已到期账号", kind: provider.MaintenanceRelogin, status: provider.StatusError, expiry: new(now.Add(-time.Minute))},
		{name: "重登跳过关闭调度的活跃账号", kind: provider.MaintenanceRelogin, status: provider.StatusActive},
	} {
		t.Run(tc.name, func(t *testing.T) {
			draft := provider.MaintenanceDraft{Kind: tc.kind}
			record := &provider.Record{
				ID: 12, Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeOAuth,
				Status: tc.status, ExpiresAt: tc.expiry, ErrorMessage: "Token revoked (401): expired",
				TempUnschedulableUntil: new(now.Add(time.Minute)), TempUnschedulableReason: "OAuth 401: invalid token",
			}
			require.Equal(t, "inactive", describeMaintenanceTarget(t.Context(), draft, record, nil).Reason)
		})
	}
}

func (f maintenanceTargetFixture) GetByID(_ context.Context, id int64) (*provider.Record, error) {
	if record := f.records[id]; record != nil {
		return record, nil
	}
	return nil, provider.ErrProviderNotFound
}

func (f maintenanceTargetFixture) GetByIDs(_ context.Context, ids []int64) ([]*provider.Record, error) {
	var records []*provider.Record
	for _, id := range ids {
		if record := f.records[id]; record != nil {
			records = append(records, record)
		}
	}
	return records, nil
}

func (f maintenanceTargetFixture) QueryProvidersByGroup(_ context.Context, groupID int64, _ providerpostgres.ProviderGroupQueryOptions) ([]provider.Record, error) {
	return f.members[groupID], nil
}

func (f maintenanceTargetFixture) ListAllWithFilters(context.Context, string, string, string, string, int64, string) ([]provider.Record, error) {
	return nil, nil
}

func (f maintenanceTargetFixture) GetByIDLite(_ context.Context, id int64) (*routing.Group, error) {
	if group := f.groups[id]; group != nil {
		return group, nil
	}
	return nil, routing.ErrGroupNotFound
}
