package provider

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
)

// stateProbeFixture 的同一账号结果供手动请求和计划请求复用。
type stateProbeFixture struct {
	info  TestTargetInfo
	probe func(context.Context, string) StateProbeResult
}

func (f *stateProbeFixture) LoadTestTarget(context.Context, TestRequest) (TestTarget, error) {
	return f, nil
}

func (f *stateProbeFixture) Information() TestTargetInfo { return f.info }
func (f *stateProbeFixture) Execute(context.Context, PreparedTestRequest, TestEventSink) error {
	return ErrIQUnsupported
}

func (f *stateProbeFixture) ProbeTurnState(ctx context.Context, model string) StateProbeResult {
	return f.probe(ctx, model)
}

func TestRunStateProbeVerdictsAndAccountLock(t *testing.T) {
	info := TestTargetInfo{ProviderSnapshot: ProviderSnapshot{ID: 1, Platform: PlatformOpenAI, Type: ProviderTypeOAuth, EnabledProtocols: []capability.ProtocolID{capability.ProtocolOpenAIResponses}}}
	f := &stateProbeFixture{info: info}
	service := NewTestService(f, TestOptions{})
	for _, tt := range []struct {
		name    string
		out     StateProbeResult
		verdict string
	}{
		{"状态延续", StateProbeResult{Minted: true, MintCompleted: true, ContinueCompleted: true, MintStatus: 200, ContinueStatus: 200}, "healthy"},
		{"状态变化", StateProbeResult{Minted: true, MintCompleted: true, ContinueCompleted: true, MintStatus: 200, ContinueStatus: 200, NewTicket: true}, "degraded"},
		{"中途断流", StateProbeResult{Minted: true, MintCompleted: true, MintStatus: 200, ContinueStatus: 200, Failure: "stream_error"}, "inconclusive"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f.probe = func(ctx context.Context, model string) StateProbeResult {
				require.Equal(t, "alias", model)
				_, err := service.RunStateProbe(ctx, 1, model)
				require.ErrorIs(t, err, ErrIQBusy)
				return tt.out
			}
			got, err := service.RunStateProbe(t.Context(), 1, "alias")
			require.NoError(t, err)
			require.Equal(t, tt.verdict, got.Verdict)
			require.Equal(t, "alias", got.RequestedModel)
			require.NotEmpty(t, got.Reason)
			require.NotZero(t, got.FinishedAt)
		})
	}
}

func TestSupportsStateProbe(t *testing.T) {
	record := &Record{Platform: PlatformOpenAI, Type: ProviderTypeOAuth}
	require.True(t, SupportsStateProbe(record))
	require.False(t, SupportsStateProbe(nil))
	for _, key := range []string{"personalAccessToken", "agentIdentity"} {
		record.Credentials = map[string]any{"auth_mode": key}
		require.False(t, SupportsStateProbe(record))
	}
	record.Credentials = map[string]any{UpstreamProtocolsKey: []string{}}
	require.False(t, SupportsStateProbe(record))
	record.Credentials = nil
	parent := int64(1)
	record.ParentProviderID = &parent
	require.False(t, SupportsStateProbe(record))
	record.ParentProviderID = nil
	record.Type = ProviderTypeAPIKey
	require.False(t, SupportsStateProbe(record))
	for _, model := range []string{"", "gpt-image-1", "openai/text-embedding-3-large", "https://private.example", "gpt\nx"} {
		require.False(t, SupportsStateProbeModel(model), model)
	}
	require.True(t, SupportsStateProbeModel("gpt-5.4"))
}

func TestRunStateProbeDefaultAndCancelled(t *testing.T) {
	f := &stateProbeFixture{info: TestTargetInfo{ProviderSnapshot: ProviderSnapshot{ID: 28, Platform: PlatformOpenAI, Type: ProviderTypeOAuth, EnabledProtocols: []capability.ProtocolID{capability.ProtocolOpenAIResponses}}}}
	calls := 0
	f.probe = func(_ context.Context, model string) StateProbeResult {
		calls++
		require.Equal(t, StateProbeDefaultModel, model)
		return StateProbeResult{Failure: "account_error"}
	}
	s := NewTestService(f, TestOptions{})
	got, err := s.RunStateProbe(t.Context(), 28, "")
	require.NoError(t, err)
	require.Equal(t, "gpt-6-astra", got.RequestedModel)
	require.Empty(t, got.Model)
	require.Equal(t, "inconclusive", got.Verdict)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = s.RunStateProbe(ctx, 28, "")
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, calls)
	_, err = s.RunStateProbe(t.Context(), 29, "gpt-test")
	require.ErrorIs(t, err, ErrIQUnavailable)
	require.Equal(t, 1, calls)
}
