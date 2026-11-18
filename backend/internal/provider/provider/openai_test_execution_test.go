package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/infra/httpclient/tlsfingerprint"
	"github.com/TokenFlux/TokenRouter/internal/provider"
)

// excelBPSProbeStore 记录连接测试导致的提供商错误写入。
type excelBPSProbeStore struct {
	OpenAIProviderTestStore
	errorCalls int
}

func (s *excelBPSProbeStore) SetError(context.Context, int64, string) error {
	s.errorCalls++
	return nil
}

func TestOpenAIProviderTestExcelBPSFailureLeavesProviderActive(t *testing.T) {
	for _, mode := range []string{"responses", "chat", "compact", "legacy_compact"} {
		for name, body := range map[string]string{
			"bps":         `{"error":{"code":"basispoints_upstream_error","message":"Excel BPS authentication failed; request was not replayed"}}`,
			"invalid key": `{"error":{"code":"invalid_api_key","message":"Invalid API key"}}`,
		} {
			t.Run(mode+"/"+name, func(t *testing.T) {
				store := &excelBPSProbeStore{}
				transport := &codexInviteResetHTTPUpstreamStub{responses: []*http.Response{codexInviteResetJSONStatusResponse(401, body)}}
				executor := &OpenAIProviderTest{
					Store: store, Transport: transport,
					ValidateURL:  func(raw string) (string, error) { return raw, nil },
					ApplyRouting: func(*TestRun, *provider.Record, *http.Request, bool) {},
					ResolveTLS:   func(*TestRun, *provider.Record) *tlsfingerprint.Profile { return nil },
				}
				value := &provider.Record{ID: 7461, Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeAPIKey, Status: "active", Schedulable: true, Credentials: map[string]any{"api_key": "fixture"}}
				run := NewTestRun(context.Background(), nil, &geminiTestSinkFixture{})
				defer run.Cancel()
				run.RequestedProtocol = provider.TextProtocolResponses
				testMode := provider.ProviderTestModeDefault
				switch mode {
				case "chat":
					run.RequestedProtocol = provider.TextProtocolChatCompletions
				case "compact":
					testMode = provider.ProviderTestModeCompact
				case "legacy_compact":
					testMode = provider.ProviderTestModeLegacyCompact
				}
				err := executor.Execute(run, value, "gpt-6-sol", provider.DefaultProviderTestPrompt(), testMode, "text")
				require.ErrorContains(t, err, "401")
				require.Len(t, transport.bodies, 1)
				if name == "bps" {
					require.Zero(t, store.errorCalls)
					require.Equal(t, "active", value.Status)
					require.True(t, value.Schedulable)
				} else {
					require.Equal(t, 1, store.errorCalls)
				}
			})
		}
	}
}

func TestOpenAIProviderTestFallsBackWhenPreferredModelRejected(t *testing.T) {
	for _, protocol := range []provider.TextProtocol{provider.TextProtocolResponses, provider.TextProtocolChatCompletions} {
		t.Run(string(protocol), func(t *testing.T) {
			transport := &codexInviteResetHTTPUpstreamStub{responses: []*http.Response{
				codexInviteResetJSONStatusResponse(400, `{"error":{"message":"The 'gpt-6.1-sol' model is not supported when using Codex with a ChatGPT account."}}`),
				codexInviteResetJSONStatusResponse(400, `{"error":{"code":"model_not_found","message":"fallback unavailable"}}`),
			}}
			executor := &OpenAIProviderTest{
				Transport:    transport,
				ValidateURL:  func(raw string) (string, error) { return raw, nil },
				ApplyRouting: func(*TestRun, *provider.Record, *http.Request, bool) {},
				ResolveTLS:   func(*TestRun, *provider.Record) *tlsfingerprint.Profile { return nil },
			}
			value := &provider.Record{ID: 1, Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeAPIKey, Credentials: map[string]any{"api_key": "fixture", "model_whitelist": []string{"gpt-6.1-sol", "gpt-6-astra"}}}
			run := NewTestRun(context.Background(), nil, &geminiTestSinkFixture{})
			defer run.Cancel()
			run.RequestedProtocol = protocol
			require.Error(t, executor.Execute(run, value, "gpt-6.1-sol", provider.DefaultProviderTestPrompt(), "default", "text"))
			require.Len(t, transport.bodies, 2)
			var first, second map[string]any
			require.NoError(t, json.Unmarshal([]byte(transport.bodies[0]), &first))
			require.NoError(t, json.Unmarshal([]byte(transport.bodies[1]), &second))
			require.Equal(t, "gpt-6.1-sol", first["model"])
			require.Equal(t, "gpt-6-astra", second["model"])
			require.True(t, run.ModelFallbackTried)
		})
	}
}

func TestOpenAITestModelUnavailableRejectsUnrelatedErrors(t *testing.T) {
	for _, status := range []int{401, 403, 429, 503} {
		require.False(t, openAITestModelUnavailable(status, []byte(`{"error":{"code":"model_not_found"}}`)))
	}
	require.False(t, openAITestModelUnavailable(400, []byte(`{"error":{"message":"invalid tool schema"}}`)))
	require.False(t, openAITestModelUnavailable(404, []byte(`not found`)))
}
