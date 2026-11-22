package provider

import (
	"context"
	"errors"
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

// 复检场景经过 testing.go、test_targets.go、openai_test_execution.go 和 test_answer_stream.go。
type scheduledAuthTransport func(*http.Request, string, int64, int, *tlsfingerprint.Profile) (*http.Response, error)

func (f scheduledAuthTransport) DoWithTLS(req *http.Request, proxy string, id int64, concurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	return f(req, proxy, id, concurrency, profile)
}

// TestScheduledAuthReadsSingleUpstream 将目录、模型映射与自动探针传输接入认证复检。
func TestScheduledAuthReadsSingleUpstream(t *testing.T) {
	const partial = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"OK\"}\n\n"
	const terminal = "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n"
	for _, tc := range []struct {
		name     string
		code     int
		body     string
		complete bool
		fail     bool
		only     string
	}{
		{name: "正常完成", code: 200, body: partial + terminal, complete: true},
		{name: "账号只有一个自选模型", code: 200, body: partial + terminal, complete: true, only: "gpt-6-sol"},
		{name: "收到文字后断流", code: 200, body: partial, fail: true},
		{name: "收到状态不明的结束事件", code: 200, body: partial + "data: {\"type\":\"response.done\"}\n\n", fail: true},
		{name: "上游凭据失效", code: 401, body: "private upstream body"},
		{name: "上游限流", code: 429},
		{name: "模型不可用", code: 404, body: `{"error":{"code":"model_not_found"}}`},
		{name: "上游重定向", code: 302},
		{name: "网络失败", fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proxyID := int64(7)
			record := &provider.Record{
				ID: 43, Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeOAuth, Status: provider.StatusError,
				Concurrency: 2, ProxyID: &proxyID, Proxy: &egress.Proxy{Protocol: "http", Host: "127.0.0.1", Port: 8888},
				Credentials: map[string]any{
					"access_token":  "fixture",
					"model_mapping": map[string]any{"gpt-6.1-sol": "gpt-selected", "gpt-selected": "converted-twice"},
				},
			}
			expectedModel := "gpt-selected"
			if tc.only != "" {
				record.Credentials["model_whitelist"] = []string{tc.only}
				expectedModel = tc.only
			}
			var reads, requests int
			profile := &tlsfingerprint.Profile{Name: "auth-fixture"}
			transport := scheduledAuthTransport(func(req *http.Request, proxy string, id int64, concurrency int, tls *tlsfingerprint.Profile) (*http.Response, error) {
				requests++
				require.Equal(t, record.ID, id)
				require.Equal(t, record.Concurrency, concurrency)
				require.Equal(t, record.Proxy.URL(), proxy)
				require.Same(t, profile, tls)
				require.Equal(t, "https://chatgpt.com/backend-api/codex/responses", req.URL.String())
				require.Equal(t, "Bearer fixture", req.Header.Get("Authorization"))
				require.True(t, upstream.HTTPUpstreamRedirectsDisabled(req.Context()))
				deadline, ok := req.Context().Deadline()
				require.True(t, ok)
				require.LessOrEqual(t, time.Until(deadline), time.Minute)
				payload, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				require.Equal(t, expectedModel, gjson.GetBytes(payload, "model").String())
				if tc.code == 0 {
					return nil, errors.New("private transport detail")
				}
				return &http.Response{StatusCode: tc.code, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			policy := &OpenAIProbePolicy{Available: true}
			targets := &TestTargets{
				Read: func(_ context.Context, id int64) (*provider.Record, error) {
					require.Equal(t, record.ID, id)
					reads++
					return record, nil
				},
				OpenAI: &OpenAIProviderTest{
					// 任何健康状态或用量写入都会调用未装配的方法并使测试失败。
					Store: &struct{ OpenAIProviderTestStore }{}, Transport: transport,
					Prepare: policy.Prepare, ApplyRouting: policy.ApplyTestRouting,
					ResolveTLS: func(*TestRun, *provider.Record) *tlsfingerprint.Profile { return profile },
				},
			}
			service := provider.NewTestService(targets, provider.TestOptions{})
			status, complete, err := service.ObserveScheduledAuth(t.Context(), record.ID, "")
			if tc.fail {
				require.Error(t, err)
				require.NotContains(t, err.Error(), "private")
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.code, status)
			require.Equal(t, tc.complete, complete)
			require.Equal(t, 1, reads)
			require.Equal(t, 1, requests)
		})
	}
}
