package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/TokenFlux/TokenRouter/internal/egress"
	"github.com/TokenFlux/TokenRouter/internal/infra/httpclient/tlsfingerprint"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/upstream"
)

type stateTransport struct {
	do func(*http.Request, string, int64, int, *tlsfingerprint.Profile) (*http.Response, error)
}

func (s stateTransport) DoWithTLS(r *http.Request, proxy string, id int64, slots int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	return s.do(r, proxy, id, slots, profile)
}

// TestStateProbeRechecksWhitelist 两步之间收窄全局名单时，探测应在第二次发送前结束。
func TestStateProbeRechecksWhitelist(t *testing.T) {
	global := provider.NewOpenAIOAuthModelPolicy([]string{"gpt-test"})
	rec := &provider.Record{
		ID: 37, Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeOAuth,
		Credentials:               map[string]any{"access_token": "local-test"},
		OpenAIOAuthModelWhitelist: func(context.Context) (provider.OpenAIOAuthModelPolicy, error) { return global, nil },
	}
	requests := 0
	transport := stateTransport{do: func(*http.Request, string, int64, int, *tlsfingerprint.Profile) (*http.Response, error) {
		requests++
		global = provider.NewOpenAIOAuthModelPolicy([]string{"gpt-other"})
		return &http.Response{
			StatusCode: 200, Header: http.Header{stateProbeHeader: []string{"first"}},
			Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")),
		}, nil
	}}
	targets := &TestTargets{Read: func(context.Context, int64) (*provider.Record, error) { return rec, nil }, OpenAI: &OpenAIProviderTest{Transport: transport}}
	result, err := provider.NewTestService(targets, provider.TestOptions{}).RunStateProbe(t.Context(), rec.ID, "gpt-test")
	require.NoError(t, err)
	require.Equal(t, 1, requests)
	require.True(t, result.Minted)
	require.Equal(t, "model_unsupported", result.Failure)
}

// TestStateProbeOwnSession 检查两次请求的账号、会话和完成事件，以及对外可见的证据。
func TestStateProbeOwnSession(t *testing.T) {
	const complete = "data: {\"type\":\"response.created\",\"response\":{\"model\":\"reported\"}}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"model\":\"reported\"}}\n\n"
	for _, tt := range []struct {
		name                   string
		first, second          string
		status                 int
		body, verdict, failure string
		calls                  int
	}{
		{"不回状态", "own-state", "", 200, complete, "healthy", "", 2},
		{"返回原状态", "own-state", "own-state", 200, complete, "healthy", "", 2},
		{"返回不同状态", "own-state", "new-state", 200, complete, "degraded", "", 2},
		{"缺少首状态", "", "", 200, complete, "inconclusive", "no_ticket", 1},
		{"首步断流", "own-state", "", 200, "data: {\"type\":\"response.created\"}\n\n", "inconclusive", "stream_error", 1},
		{"请求被限流", "own-state", "", 429, "upstream-private-error", "inconclusive", "rate_limited", 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := &provider.Record{ID: 37, Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeOAuth, Concurrency: 2, Credentials: map[string]any{"access_token": "test-access-token", "chatgpt_account_id": "own-account-id", "model_mapping": map[string]any{"alias": "gpt-chosen", "gpt-chosen": "wrong-second-mapping"}}}
			var session string
			calls := 0
			transport := stateTransport{do: func(r *http.Request, proxy string, id int64, slots int, _ *tlsfingerprint.Profile) (*http.Response, error) {
				calls++
				require.Equal(t, rec.ID, id)
				require.Equal(t, 2, slots)
				require.Empty(t, proxy)
				require.Equal(t, "Bearer test-access-token", r.Header.Get("Authorization"))
				require.NotEmpty(t, r.Header.Get("session_id"))
				require.Empty(t, r.Header.Get("x-openai-internal-codex-responses-lite"))
				raw, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.Equal(t, "gpt-chosen", gjson.GetBytes(raw, "model").String())
				require.True(t, gjson.GetBytes(raw, "stream").Bool())
				require.False(t, gjson.GetBytes(raw, "store").Bool())
				headers := http.Header{"Content-Type": []string{"text/event-stream"}}
				if calls == 1 {
					session = r.Header.Get("session_id")
					require.Empty(t, r.Header.Get("x-codex-turn-state"))
					require.Empty(t, r.Header.Get("Cookie"))
					headers.Set("x-codex-turn-state", tt.first)
					headers.Set("Set-Cookie", "own_http_session=private-cookie; Secure; Path=/")
				} else {
					require.Equal(t, session, r.Header.Get("session_id"))
					require.Equal(t, "own-state", r.Header.Get("x-codex-turn-state"))
					require.Contains(t, r.Header.Get("Cookie"), "own_http_session=private-cookie")
					headers.Set("x-codex-turn-state", tt.second)
				}
				return &http.Response{StatusCode: tt.status, Header: headers, Body: io.NopCloser(strings.NewReader(tt.body))}, nil
			}}
			loader := &TestTargets{Read: func(context.Context, int64) (*provider.Record, error) { return rec, nil }, OpenAI: &OpenAIProviderTest{Transport: transport}}
			s := provider.NewTestService(loader, provider.TestOptions{})
			r, err := s.RunStateProbe(t.Context(), 37, "alias")
			require.NoError(t, err)
			require.Equal(t, tt.verdict, r.Verdict)
			require.Equal(t, tt.failure, r.Failure)
			require.Equal(t, tt.calls, calls)
			require.Equal(t, tt.status, r.MintStatus)
			require.Equal(t, len(tt.first), r.TicketLength)
			require.Equal(t, "gpt-chosen", r.Model)
			if calls == 2 {
				require.Equal(t, 200, r.ContinueStatus)
				require.Equal(t, len(tt.second), r.ContinueTicketLength)
				require.Equal(t, "reported", r.ReportedModel)
			}
			data, err := json.Marshal(r)
			require.NoError(t, err)
			for _, secret := range []string{"test-access-token", "own_http_session", "private-cookie", "own-state", "new-state", "upstream-private-error", session} {
				require.NotContains(t, string(data), secret)
			}
		})
	}
}

// TestStateProbeContinuationFailures 检查已获首状态之后的失败不会被记为续接正常或状态变化。
func TestStateProbeContinuationFailures(t *testing.T) {
	const completed = "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n"
	for _, tt := range []struct {
		name              string
		status            int
		state, body, code string
	}{
		{"断流", 200, "changed", "data: {\"type\":\"response.created\"}\n\n", "stream_error"},
		{"不完整", 200, "changed", "data: {\"type\":\"response.incomplete\"}\n\n", "stream_error"},
		{"没有终态的 done", 200, "changed", "data: {\"type\":\"response.done\"}\n\n", "stream_error"},
		{"续接状态非法", 200, "one\ntwo", completed, "invalid_state"},
		{"响应超过上限", 200, "changed", strings.Repeat(" ", stateProbeWireLimit+1), "output_limit"},
		{"输出超过上限", 200, "changed", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"" + strings.Repeat("a", stateProbeTextLimit+1) + "\"}\n\n" + completed, "output_limit"},
		{"套餐不支持模型", 400, "", `{"error":{"code":"model_not_supported","message":"private detail"}}`, "model_unsupported"},
		{"账号授权失效", 401, "", "", "account_error"},
		{"重定向", 302, "", "", "upstream_error"},
		{"服务不可用", 503, "", "", "upstream_error"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			proxyID := int64(101)
			rec := &provider.Record{ID: 37, Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeOAuth, Credentials: map[string]any{"access_token": "local-secret", "chatgpt_account_id": "own-id"}, ProxyID: &proxyID, Proxy: &egress.Proxy{ID: proxyID, Protocol: "http", Host: "proxy.example", Port: 8080}}
			profile := &tlsfingerprint.Profile{Name: "account-profile"}
			calls := 0
			transport := stateTransport{do: func(req *http.Request, proxy string, id int64, _ int, actual *tlsfingerprint.Profile) (*http.Response, error) {
				calls++
				require.Equal(t, rec.ID, id)
				require.Same(t, profile, actual)
				require.Equal(t, "http://proxy.example:8080", proxy)
				require.Equal(t, "own-id", req.Header.Get("chatgpt-account-id"))
				require.True(t, upstream.HTTPUpstreamRedirectsDisabled(req.Context()))
				limit, ok := req.Context().Deadline()
				require.True(t, ok)
				require.Positive(t, time.Until(limit))
				require.LessOrEqual(t, time.Until(limit), stateProbeStepTimeout)
				if calls == 1 {
					return &http.Response{StatusCode: 200, Header: http.Header{stateProbeHeader: []string{"first-state"}}, Body: io.NopCloser(strings.NewReader(completed))}, nil
				}
				return &http.Response{StatusCode: tt.status, Header: http.Header{stateProbeHeader: []string{tt.state}}, Body: io.NopCloser(strings.NewReader(tt.body))}, nil
			}}
			targets := &TestTargets{Read: func(context.Context, int64) (*provider.Record, error) { return rec, nil }, OpenAI: &OpenAIProviderTest{Transport: transport, ResolveTLS: func(_ *TestRun, r *provider.Record) *tlsfingerprint.Profile { require.Same(t, rec, r); return profile }}}
			result, err := provider.NewTestService(targets, provider.TestOptions{}).RunStateProbe(t.Context(), 37, "gpt-test")
			require.NoError(t, err)
			require.Equal(t, 2, calls)
			require.True(t, result.Minted)
			require.True(t, result.MintCompleted)
			require.False(t, result.ContinueCompleted)
			require.False(t, result.NewTicket)
			require.Equal(t, "inconclusive", result.Verdict)
			require.Equal(t, tt.code, result.Failure)
			require.Equal(t, tt.status, result.ContinueStatus)
		})
	}
}

// TestStateProbeStopsInFlight 检查取消信号到达当前传输后，同一个账号可以再次测试。
func TestStateProbeStopsInFlight(t *testing.T) {
	rec := &provider.Record{ID: 65, Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeOAuth, Credentials: map[string]any{"access_token": "local-secret"}}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := make(chan struct{})
	transport := stateTransport{do: func(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
		close(started)
		<-req.Context().Done()
		return nil, req.Context().Err()
	}}
	targets := &TestTargets{Read: func(context.Context, int64) (*provider.Record, error) { return rec, nil }, OpenAI: &OpenAIProviderTest{Transport: transport}}
	service := provider.NewTestService(targets, provider.TestOptions{})
	done := make(chan error, 1)
	go func() { _, err := service.RunStateProbe(ctx, 65, "gpt-test"); done <- err }()
	<-started
	_, err := service.RunStateProbe(t.Context(), 65, "gpt-test")
	require.ErrorIs(t, err, provider.ErrIQBusy)
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	targets.OpenAI.Transport = stateTransport{do: func(_ *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
		return nil, context.DeadlineExceeded
	}}
	out, err := service.RunStateProbe(t.Context(), 65, "gpt-test")
	require.NoError(t, err)
	require.Equal(t, "timeout", out.Failure)
	require.Empty(t, out.Model)
}

// TestStateProbeReportedModelPrivacy 过滤模型字段中回显的凭据，并保留跨行 SSE 里的实际模型名。
func TestStateProbeReportedModelPrivacy(t *testing.T) {
	for _, name := range []string{"local-oauth-secret", "state-secret", "account-secret", "cookie-secret", "wire-model"} {
		t.Run(name, func(t *testing.T) {
			rec := &provider.Record{ID: 70, Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeOAuth, Credentials: map[string]any{"access_token": "local-oauth-secret", "chatgpt_account_id": "account-secret"}}
			body := "data: {\"type\":\"response.completed\",\n" + "data: \"response\":{\"model\":\"" + name + "\",\"status\":\"completed\"}}\n\n"
			targets := &TestTargets{Read: func(context.Context, int64) (*provider.Record, error) { return rec, nil }, OpenAI: &OpenAIProviderTest{Transport: stateTransport{do: func(*http.Request, string, int64, int, *tlsfingerprint.Profile) (*http.Response, error) {
				h := make(http.Header)
				h.Set(stateProbeHeader, "state-secret")
				h.Set("Set-Cookie", "sid=cookie-secret; Secure; Path=/")
				return &http.Response{StatusCode: 200, Header: h, Body: io.NopCloser(strings.NewReader(body))}, nil
			}}}}
			result, err := provider.NewTestService(targets, provider.TestOptions{}).RunStateProbe(t.Context(), 70, "gpt-test")
			require.NoError(t, err)
			require.Equal(t, "healthy", result.Verdict)
			if name == "wire-model" {
				require.Equal(t, name, result.ReportedModel)
				return
			}
			out, err := json.Marshal(result)
			require.NoError(t, err)
			require.NotContains(t, string(out), name)
		})
	}
}
