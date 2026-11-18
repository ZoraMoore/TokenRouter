package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/TokenFlux/TokenRouter/internal/infra/httpclient/tlsfingerprint"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
	"github.com/TokenFlux/TokenRouter/internal/upstream/qoder"
)

// 本场景贯通 pelican.go、test_targets.go、各平台的 test_execution 和响应流读取。
type pelicanHTTPFixture struct {
	requests    []string
	paths       []string
	status      int
	stream      string
	contentType string
}

type pelicanQoderFixture struct {
	stream, request, model string
}

// TestModelTraceTextProtocols 检查三次独立请求的模型、档位、系统提示和终态。
func TestModelTraceTextProtocols(t *testing.T) {
	numbers := strings.Repeat("1,7,9,", 110)
	for _, tt := range []struct{ name, platform, kind, protocol, system string }{
		{"Responses 密钥", provider.PlatformOpenAI, provider.ProviderTypeAPIKey, "responses", "none"},
		{"Responses OAuth", provider.PlatformOpenAI, provider.ProviderTypeOAuth, "responses", "required_responses_instructions"},
		{"Chat 密钥", provider.PlatformOpenAI, provider.ProviderTypeAPIKey, "chat_completions", "none"},
		{"Messages 密钥", provider.PlatformAnthropic, provider.ProviderTypeAPIKey, "messages", "none"},
		{"Messages OAuth", provider.PlatformAnthropic, provider.ProviderTypeOAuth, "messages", "required_claude_code"},
		{"Messages setup-token", provider.PlatformAnthropic, provider.ProviderTypeSetupToken, "messages", "required_claude_code"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			record := &provider.Record{ID: 34, Platform: tt.platform, Type: tt.kind, Credentials: map[string]any{
				"access_token": "fixture", "api_key": "fixture", "model_mapping": map[string]any{"alias": "selected-model", "selected-model": "wrong-hop"},
			}}
			var body string
			switch tt.protocol {
			case "responses":
				body = `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"` + numbers + `"}]}]}`
			case "chat_completions":
				record.Extra = map[string]any{provider.ExtraKeyTextRouteMode: string(provider.TextRouteModeForceChatCompletions)}
				body = `{"choices":[{"finish_reason":"stop","message":{"content":"` + numbers + `"}}]}`
			case "messages":
				body = `{"type":"message","stop_reason":"end_turn","content":[{"type":"text","text":"` + numbers + `"}]}`
			}
			transport := &pelicanHTTPFixture{status: http.StatusOK, stream: body, contentType: "application/json"}
			s := pelicanServiceForHTTP(record, transport)
			var result provider.ModelTraceResult
			err := s.RunModelTrace(t.Context(), record.ID, provider.IQTestRequest{ModelID: "alias", ReasoningEffort: "high"}, func(name string, data any) error {
				if name == "modeltrace_complete" {
					var ok bool
					result, ok = data.(provider.ModelTraceResult)
					require.True(t, ok)
				}
				return nil
			})
			require.NoError(t, err)
			require.Len(t, transport.requests, 3)
			require.Equal(t, "complete", result.Status)
			require.Equal(t, 3, result.Analysis.UsedOutputs)
			for i, raw := range transport.requests {
				require.Equal(t, "selected-model", gjson.Get(raw, "model").String())
				require.NotContains(t, raw, "wrong-hop")
				require.NotContains(t, raw, "SVG")
				require.False(t, gjson.Get(raw, "tools").Exists())
				sample := result.Samples[i]
				require.True(t, sample.Accepted)
				require.Equal(t, "high", sample.ReasoningEffort)
				require.Equal(t, "selected-model", sample.MappedModel)
				require.Equal(t, tt.protocol, sample.Environment.Protocol)
				require.Equal(t, tt.system, sample.Environment.SystemPrompt)
				require.Equal(t, tt.system == "required_claude_code", gjson.Get(raw, "system").Exists())
				require.Equal(t, tt.system == "required_responses_instructions", gjson.Get(raw, "instructions").Exists())
				require.Contains(t, raw, sample.Challenge.Prompt)
			}
		})
	}
}

func (f *pelicanHTTPFixture) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	f.requests = append(f.requests, string(body))
	f.paths = append(f.paths, req.URL.Path)
	header := make(http.Header)
	header.Set("Content-Type", f.contentType)
	return &http.Response{StatusCode: f.status, Body: io.NopCloser(strings.NewReader(f.stream)), Header: header}, nil
}

func (f *pelicanHTTPFixture) DoWithTLS(req *http.Request, proxy string, id int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return f.Do(req, proxy, id, concurrency)
}

func TestPelicanUsesOneCompletedProviderAnswer(t *testing.T) {
	const claudeAnswer = "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"21\"}}\n"
	const claudeStop = "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\ndata: {\"type\":\"message_stop\"}\n"
	const geminiAnswer = "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"21\"}]}}]}\n"
	const geminiStop = "data: {\"candidates\":[{\"finishReason\":\"STOP\"}]}\n"
	const chatAnswer = "data: {\"choices\":[{\"delta\":{\"content\":\"21\"}}]}\n"
	const chatStop = "data: {\"choices\":[{\"finish_reason\":\"stop\"}]}\ndata: [DONE]\n"
	const responseAnswer = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"21\"}\n"
	const responseStop = "data: {\"type\":\"response.completed\"}\n"
	for _, tt := range []struct {
		name, platform, wire, stream, code string
	}{
		{"Claude 正常", provider.PlatformAnthropic, "messages", claudeAnswer + claudeStop, ""},
		{"Claude 流提前结束", provider.PlatformAnthropic, "messages", claudeAnswer, "incomplete"},
		{"Claude 预算耗尽后发出 stop", provider.PlatformAnthropic, "messages", claudeAnswer + "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"max_tokens\"}}\ndata: {\"type\":\"message_stop\"}\n", "incomplete"},
		{"Gemini 完成", provider.PlatformGemini, "gemini", geminiAnswer + geminiStop, ""},
		{"Gemini 断流", provider.PlatformGemini, "gemini", geminiAnswer, "incomplete"},
		{"Gemini 思考段不计入答案", provider.PlatformGemini, "gemini", "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"wrong\",\"thought\":true},{\"text\":\"21\"}]},\"finishReason\":\"STOP\"}]}\n", ""},
		{"Gemini 截断", provider.PlatformGemini, "gemini", geminiAnswer + "data: {\"candidates\":[{\"finishReason\":\"MAX_TOKENS\"}]}\n", "incomplete"},
		{"OpenAI 完成", provider.PlatformOpenAI, "responses", responseAnswer + responseStop, ""},
		{"OpenAI 在最终文字事件中给出答案", provider.PlatformOpenAI, "responses", "data: {\"type\":\"response.output_text.done\",\"text\":\"21\"}\n" + responseStop, ""},
		{"OpenAI 流提前结束", provider.PlatformOpenAI, "responses", responseAnswer, "incomplete"},
		{"OpenAI 不完整事件", provider.PlatformOpenAI, "responses", responseAnswer + "data: {\"type\":\"response.incomplete\"}\n", "incomplete"},
		{"OpenAI 错误事件", provider.PlatformOpenAI, "responses", responseAnswer + "data: {\"type\":\"error\",\"error\":{\"message\":\"upstream-private-fixture\"}}\n", "upstream_error"},
		{"OpenAI 超大单行", provider.PlatformOpenAI, "responses", "data: " + strings.Repeat(" ", (1<<20)+20), "output_limit"},
		{"Grok 完成", provider.PlatformGrok, "responses", responseAnswer + responseStop, ""},
		{"Kimi 多协议只发一道", provider.PlatformKimi, "adaptive", chatAnswer + chatStop, ""},
		{"DeepSeek 多协议只发一道", provider.PlatformDeepseek, "adaptive", chatAnswer + chatStop, ""},
		{"智谱 多协议只发一道", provider.PlatformZhipu, "adaptive", chatAnswer + chatStop, ""},
		{"Chat 提前截断", provider.PlatformKimi, "adaptive", chatAnswer + "data: {\"choices\":[{\"finish_reason\":\"length\"}]}\ndata: [DONE]\n", "incomplete"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			creds := map[string]any{"api_key": "fixture", "model_mapping": map[string]any{"selected": "upstream-model", "upstream-model": "wrong-second-hop"}}
			if tt.wire == "adaptive" {
				creds["api_protocol"] = "adaptive"
				creds[provider.UpstreamProtocolsKey] = []string{"openai_chat_completions", "anthropic_messages"}
			}
			record := &provider.Record{ID: 14, Platform: tt.platform, Type: provider.ProviderTypeAPIKey, Credentials: creds}
			transport := &pelicanHTTPFixture{stream: tt.stream, status: http.StatusOK}
			service := pelicanServiceForHTTP(record, transport)
			evidence := service.RunPelican(t.Context(), record.ID, "selected", "candy", "round", "")
			require.Len(t, transport.requests, 1)
			require.Contains(t, transport.requests[0], "五角星形 7 6 4")
			require.NotContains(t, transport.requests[0], "wrong-second-hop")
			if tt.wire == "gemini" {
				require.Contains(t, transport.paths[0], "/upstream-model:")
			} else {
				require.Equal(t, "upstream-model", gjson.Get(transport.requests[0], "model").String())
			}
			if tt.wire == "messages" {
				require.EqualValues(t, 32000, gjson.Get(transport.requests[0], "max_tokens").Int())
			}
			require.Equal(t, tt.code, evidence.DetailCode)
			require.Equal(t, "default", evidence.ReasoningEffort)
			if tt.code == "" {
				require.Equal(t, "passed", evidence.Status)
				require.Equal(t, "21", evidence.Answer)
			} else {
				require.Equal(t, "inconclusive", evidence.Status)
				require.Empty(t, evidence.Answer)
			}
		})
	}
}

func TestPelicanGradesCompletedJSONResponse(t *testing.T) {
	for _, tt := range []struct {
		name, protocol, body, code string
	}{
		{"Responses 正常", "responses", `{"status":"completed","error":null,"output":[{"type":"message","content":[{"type":"output_text","text":"21"}]}]}`, ""},
		{"Responses 文本兼容单段对象", "responses", `{"status":"completed","output":{"type":"message","content":{"type":"output_text","text":"21"}}}`, ""},
		{"Responses 缺少完成标记", "responses", `{"output":[{"type":"message","content":[{"type":"output_text","text":"21"}]}]}`, "incomplete"},
		{"Chat 正常", "chat_completions", `{"choices":[{"message":{"role":"assistant","content":"21"},"finish_reason":"stop"}]}`, ""},
		{"Chat 错误报文", "chat_completions", `{"error":{"message":"upstream-private-fixture"}}`, "upstream_error"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			record := &provider.Record{ID: 5, Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeAPIKey, Credentials: map[string]any{"api_key": "fixture"}}
			transport := &pelicanHTTPFixture{status: http.StatusOK, stream: tt.body, contentType: "application/json; charset=utf-8"}
			evidence := pelicanServiceForHTTP(record, transport).RunPelican(t.Context(), record.ID, "selected", "candy", "round", tt.protocol)
			require.Equal(t, tt.code, evidence.DetailCode)
			if tt.code == "" {
				require.Equal(t, "passed", evidence.Status)
			} else {
				require.Empty(t, evidence.Answer)
			}
		})
	}
}

func TestPelicanDoesNotSubstituteUnavailableModel(t *testing.T) {
	record := &provider.Record{ID: 4, Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeAPIKey, Credentials: map[string]any{"api_key": "fixture", "model_whitelist": []string{"gpt-6.1-sol", "gpt-6-astra"}}}
	transport := &pelicanHTTPFixture{status: http.StatusBadRequest, stream: `{"error":{"code":"model_not_found","message":"upstream-private-fixture"}}`}
	service := pelicanServiceForHTTP(record, transport)
	evidence := service.RunPelican(t.Context(), record.ID, "gpt-6.1-sol", "candy", "round", "")
	require.Len(t, transport.requests, 1)
	require.Equal(t, "gpt-6.1-sol", gjson.Get(transport.requests[0], "model").String())
	require.Equal(t, "inconclusive", evidence.Status)
	require.Equal(t, "upstream_error", evidence.DetailCode)
	rendered, err := json.Marshal(evidence)
	require.NoError(t, err)
	require.NotContains(t, string(rendered), "upstream-private-fixture")
}

func TestPelicanRejectsDisabledSelectedProtocol(t *testing.T) {
	record := &provider.Record{
		ID: 8, Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeAPIKey,
		Credentials: map[string]any{"api_key": "fixture", provider.UpstreamProtocolsKey: []string{"openai_responses"}},
	}
	transport := &pelicanHTTPFixture{status: http.StatusOK}
	evidence := pelicanServiceForHTTP(record, transport).RunPelican(t.Context(), record.ID, "fixture-model", "candy", "round", "chat_completions")
	require.Empty(t, transport.paths)
	require.Equal(t, "upstream_error", evidence.DetailCode)
}

func TestPelicanUsesSelectedProtocol(t *testing.T) {
	for _, tt := range []struct{ platform, selected, path, body string }{
		{provider.PlatformOpenAI, "chat_completions", "/v1/chat/completions", "data: {\"choices\":[{\"delta\":{\"content\":\"21\"},\"finish_reason\":\"stop\"}]}\n"},
		{provider.PlatformDeepseek, "anthropic", "/v1/messages", "data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"21\"}}\ndata: {\"type\":\"message_stop\"}\n"},
		{provider.PlatformDeepseek, "responses", "/responses", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"21\"}\ndata: {\"type\":\"response.completed\"}\n"},
	} {
		t.Run(tt.platform+tt.selected, func(t *testing.T) {
			record := &provider.Record{ID: 14, Platform: tt.platform, Type: provider.ProviderTypeAPIKey, Credentials: map[string]any{
				"api_key": "fixture", provider.UpstreamProtocolsKey: []string{"anthropic_messages", "openai_chat_completions", "openai_responses"},
			}}
			transport := &pelicanHTTPFixture{stream: tt.body, status: http.StatusOK}
			service := pelicanServiceForHTTP(record, transport)
			evidence := service.RunPelican(t.Context(), record.ID, "model", "candy", "round", tt.selected)
			require.Equal(t, "passed", evidence.Status)
			require.Len(t, transport.paths, 1)
			require.True(t, strings.HasSuffix(transport.paths[0], tt.path), transport.paths[0])
		})
	}
}

func TestPelicanBedrockRequiresStopReason(t *testing.T) {
	for _, reason := range []string{"end_turn", "max_tokens", ""} {
		record := &provider.Record{ID: 1, Platform: provider.PlatformAnthropic, Type: provider.ProviderTypeBedrock, Credentials: map[string]any{"auth_mode": "apikey", "api_key": "fixture", "aws_region": "us-east-1"}}
		transport := &pelicanHTTPFixture{stream: `{"content":[{"type":"thinking","thinking":"private"},{"type":"text","text":"21"}],"stop_reason":"` + reason + `"}`, status: http.StatusOK}
		evidence := pelicanServiceForHTTP(record, transport).RunPelican(t.Context(), record.ID, "claude-sonnet-4-5", "candy", "round", "")
		require.Len(t, transport.requests, 1)
		require.EqualValues(t, 32000, gjson.Get(transport.requests[0], "max_tokens").Int())
		if reason == "end_turn" {
			require.Equal(t, "passed", evidence.Status)
		} else {
			require.Equal(t, "incomplete", evidence.DetailCode)
			require.Empty(t, evidence.Answer)
		}
	}
}

func TestPelicanAntigravityUsesMappedModelAndCompletedStream(t *testing.T) {
	for _, model := range []string{"gemini-3-pro", "claude-sonnet-4-5"} {
		for _, complete := range []bool{true, false} {
			record := &provider.Record{ID: 2, Platform: provider.PlatformAntigravity, Type: provider.ProviderTypeOAuth, Credentials: map[string]any{"project_id": "fixture-project", "access_token": "fixture", "expires_at": time.Now().Add(time.Hour).Format(time.RFC3339), "model_mapping": map[string]any{"selected": model}}}
			stream := "data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"21\"}]}}]}}\n"
			if complete {
				stream += "data: {\"response\":{\"candidates\":[{\"finishReason\":\"STOP\"}]}}\n"
			}
			transport := &pelicanHTTPFixture{stream: stream, status: http.StatusOK}
			probe := &AntigravityProbe{Tokens: &provider.AntigravityTokenSource{}, Retry: &AntigravityRetry{
				Do: transport.Do, BaseURL: func(*provider.Record) string { return "https://fixture.invalid" }, BodyLimit: func() int64 { return 1 << 20 },
				Health: &provider.AntigravityHealth{},
			}}
			service := provider.NewTestService(&TestTargets{Read: func(context.Context, int64) (*provider.Record, error) { return record, nil }, Antigravity: &AntigravityProviderTest{Probe: probe.Execute}}, provider.TestOptions{})
			evidence := service.RunPelican(t.Context(), record.ID, "selected", "candy", "round", "")
			require.Len(t, transport.requests, 1)
			require.Equal(t, model, gjson.Get(transport.requests[0], "model").String())
			require.Contains(t, transport.requests[0], "五角星形 7 6 4")
			require.Greater(t, gjson.Get(transport.requests[0], "request.generationConfig.maxOutputTokens").Int(), int64(1024))
			require.Equal(t, "default", evidence.ReasoningEffort)
			if complete {
				require.Equal(t, "passed", evidence.Status)
			} else {
				require.Equal(t, "incomplete", evidence.DetailCode)
			}
		}
	}
}

func (f *pelicanQoderFixture) GetSession(context.Context, *provider.Record) (*qoder.SessionContext, error) {
	return &qoder.SessionContext{Identity: &qoder.AuthIdentity{SecurityOauthToken: "fixture"}}, nil
}

func (*pelicanQoderFixture) GetUserInfo(context.Context, string) (*qoder.UserInfo, error) {
	return nil, errors.New("测智题不需要查询用户资料")
}

func (f *pelicanQoderFixture) StreamRequestContext(_ context.Context, _ *qoder.SessionContext, _ string, body []byte, headers map[string]string) (*http.Response, error) {
	f.request = string(body)
	f.model = headers["x-model-key"]
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(f.stream))}, nil
}

func TestPelicanQoderKeepsOnlyCompletedText(t *testing.T) {
	for _, reason := range []string{"stop", "length", ""} {
		body, err := json.Marshal(map[string]any{"body": `{"choices":[{"delta":{"content":"21"},"finish_reason":"` + reason + `"}]}`})
		require.NoError(t, err)
		f := &pelicanQoderFixture{stream: "data: " + string(body) + "\n"}
		record := &provider.Record{ID: 6, Platform: provider.PlatformQoder, Type: provider.ProviderTypeCosy, Credentials: map[string]any{"site": "global"}}
		service := provider.NewTestService(&TestTargets{Read: func(context.Context, int64) (*provider.Record, error) { return record, nil }, Qoder: &QoderProviderTest{Client: f, UserInfo: f, Sessions: f}}, provider.TestOptions{})
		evidence := service.RunPelican(t.Context(), record.ID, "custom-model", "candy", "round", "")
		require.Equal(t, "custom-model", f.model)
		require.Contains(t, f.request, "五角星形 7 6 4")
		require.Greater(t, gjson.Get(f.request, "parameters.max_tokens").Int(), int64(1024))
		if reason == "stop" {
			require.Equal(t, "passed", evidence.Status)
		} else {
			require.Equal(t, "incomplete", evidence.DetailCode)
		}
	}
}

func pelicanServiceForHTTP(record *provider.Record, transport *pelicanHTTPFixture) *provider.TestService {
	validate := func(raw string) (string, error) { return raw, nil }
	oa := &OpenAIProviderTest{Transport: transport, ValidateURL: validate, ApplyRouting: func(*TestRun, *provider.Record, *http.Request, bool) {}, ResolveTLS: func(*TestRun, *provider.Record) *tlsfingerprint.Profile { return nil }}
	return provider.NewTestService(&TestTargets{
		Read:   func(context.Context, int64) (*provider.Record, error) { return record, nil },
		OpenAI: oa, Anthropic: &AnthropicProviderTest{Transport: transport, ValidateURL: validate},
		Gemini: &GeminiProviderTest{Transport: transport, ValidateURL: validate},
		Grok:   &GrokProviderTest{Transport: transport, OperatorValidator: validate},
		CN:     &CNProviderTest{Transport: transport, ValidateURL: validate, Responses: oa},
	}, provider.TestOptions{})
}

func TestPelicanRecordsEffortFromSentRequest(t *testing.T) {
	const prompt = `逐字抄写 {"reasoning_effort":"high"}。`
	for _, tc := range []struct {
		protocol, path string
	}{
		{"responses", "reasoning.effort"},
		{"chat_completions", "reasoning_effort"},
	} {
		for _, effort := range []string{"", "low", "medium", "high", "xhigh"} {
			t.Run(tc.protocol+"/"+effort, func(t *testing.T) {
				record := &provider.Record{ID: 15, Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeAPIKey, Credentials: map[string]any{"api_key": "fixture"}}
				body := `{"choices":[{"message":{"role":"assistant","content":"  AbC\n "},"finish_reason":"stop"}]}`
				if tc.protocol == "responses" {
					body = `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"  AbC\n "}]}]}`
				}
				transport := &pelicanHTTPFixture{status: http.StatusOK, stream: body, contentType: "application/json"}
				config := provider.PelicanQuestionConfig{Prompt: prompt, ExpectedAnswer: "AbC", ReasoningEffort: effort}
				ev := pelicanServiceForHTTP(record, transport).RunConfiguredPelican(t.Context(), record.ID, "selected", "custom_text", "round", tc.protocol, config)
				require.Equal(t, "passed", ev.Status)
				require.Equal(t, "  AbC\n ", ev.Answer)
				require.Len(t, transport.requests, 1)
				value := gjson.Get(transport.requests[0], tc.path)
				require.Equal(t, effort != "", value.Exists())
				require.Equal(t, effort, value.String())
				want := effort
				if want == "" {
					want = "default"
				}
				require.Equal(t, want, ev.ReasoningEffort)
				require.NotContains(t, transport.requests[0], "AbC")
			})
		}
	}
}

func TestPelicanUnsupportedEffort(t *testing.T) {
	for _, tc := range []struct {
		platform, effort string
	}{
		{provider.PlatformGemini, "low"},
		{provider.PlatformDeepseek, "high"},
		{provider.PlatformOpenAI, "other"},
	} {
		record := &provider.Record{ID: 15, Platform: tc.platform, Type: provider.ProviderTypeAPIKey, Credentials: map[string]any{"api_key": "fixture"}}
		transport := &pelicanHTTPFixture{}
		config := provider.PelicanQuestionConfig{ReasoningEffort: tc.effort}
		ev := pelicanServiceForHTTP(record, transport).RunConfiguredPelican(t.Context(), record.ID, "selected", "candy", "round", "", config)
		require.Equal(t, "inconclusive", ev.Status)
		require.Equal(t, "unsupported_effort", ev.DetailCode)
		require.Empty(t, ev.ReasoningEffort)
		require.Empty(t, transport.requests)
	}
}

func TestPelicanModelRejectsSentEffort(t *testing.T) {
	record := &provider.Record{ID: 15, Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeAPIKey, Credentials: map[string]any{"api_key": "fixture"}}
	transport := &pelicanHTTPFixture{status: http.StatusBadRequest, stream: `{"error":{"code":"unsupported_value","param":"reasoning.effort","message":"private-upstream-message"}}`}
	config := provider.PelicanQuestionConfig{ReasoningEffort: "xhigh"}
	ev := pelicanServiceForHTTP(record, transport).RunConfiguredPelican(t.Context(), record.ID, "selected", "candy", "round", "responses", config)
	require.Len(t, transport.requests, 1)
	require.Equal(t, "xhigh", gjson.Get(transport.requests[0], "reasoning.effort").String())
	require.Equal(t, "xhigh", ev.ReasoningEffort)
	require.Equal(t, "unsupported_effort", ev.DetailCode)
	require.Empty(t, ev.Answer)
}

func TestPelicanQoderReportsImplicitEffort(t *testing.T) {
	f := &pelicanQoderFixture{stream: "data: [DONE]\n"}
	record := &provider.Record{ID: 6, Platform: provider.PlatformQoder, Type: provider.ProviderTypeCosy, Credentials: map[string]any{"site": "global"}}
	service := provider.NewTestService(&TestTargets{Read: func(context.Context, int64) (*provider.Record, error) { return record, nil }, Qoder: &QoderProviderTest{Client: f, Sessions: f}}, provider.TestOptions{})
	ev := service.RunPelican(t.Context(), record.ID, "qwen3.8-max", "candy", "round", "")
	require.Equal(t, "none", gjson.Get(f.request, "parameters.reasoning_effort").String())
	require.Equal(t, "none", ev.ReasoningEffort)
	require.Equal(t, "empty_output", ev.DetailCode)
}

func TestPelicanUsesWholeFinalResponse(t *testing.T) {
	record := &provider.Record{ID: 5, Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeOAuth, Credentials: map[string]any{"access_token": "fixture"}}
	stream := `data: {"type":"response.output_text.done","text":"one"}
data: {"type":"response.completed","response":{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"one"},{"type":"output_text","text":"\ntwo "}]}]}}
`
	transport := &pelicanHTTPFixture{status: http.StatusOK, stream: stream}
	config := provider.PelicanQuestionConfig{Prompt: "请分两行回答。", ExpectedAnswer: "one\ntwo", ReasoningEffort: "high"}
	ev := pelicanServiceForHTTP(record, transport).RunConfiguredPelican(t.Context(), record.ID, "selected", "custom_text", "round", "responses", config)
	require.Len(t, transport.requests, 1)
	require.Equal(t, "high", gjson.Get(transport.requests[0], "reasoning.effort").String())
	require.Equal(t, "high", ev.ReasoningEffort)
	require.Equal(t, "passed", ev.Status)
	require.Equal(t, "one\ntwo ", ev.Answer)
}

// TestPelicanDrawingReadsCompletedArtwork 用推理事件和最终快照覆盖分组计划与手动测智两个入口。
func TestPelicanDrawingReadsCompletedArtwork(t *testing.T) {
	const prefix = `<svg xmlns="http://www.w3.org/2000/svg"><rect width="8" height="8"/><desc>`
	const suffix = "</desc></svg>"
	drawing := prefix + strings.Repeat("x", (2<<20)-len(prefix)-len(suffix)) + suffix
	snapshot := `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":` + strconv.Quote(drawing) + `}]}]}`
	reasoning := "data: " + `{"type":"response.reasoning_summary_text.delta","delta":"` + strings.Repeat("r", 8<<10) + `"}` + "\n"
	stream := strings.Repeat(reasoning, 600) +
		"data: " + `{"type":"response.output_text.delta","delta":"<svg>"}` + "\n" +
		"data: " + `{"type":"response.completed","response":` + snapshot + "}\n"

	for _, tt := range []struct {
		name, kind, effort, contentType, body string
		manual                                bool
	}{
		{"分组内置题", "pelican", "high", "text/event-stream", stream, false},
		{"分组自定义题", "custom_svg", "xhigh", "application/json", snapshot, false},
		{"手动默认题", "", "xhigh", "text/event-stream", stream, true},
		{"手动自定义题", "custom_svg", "high", "application/json", snapshot, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			record := &provider.Record{ID: 2, Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeOAuth, Credentials: map[string]any{"access_token": "fixture"}}
			transport := &pelicanHTTPFixture{status: http.StatusOK, stream: tt.body, contentType: tt.contentType}
			service := pelicanServiceForHTTP(record, transport)
			const prompt = "画一个正方形。"
			var status, answer, detail string
			if tt.manual {
				result, err := service.RunIQQuestion(t.Context(), record.ID, provider.IQTestRequest{ModelID: "selected", QuestionKind: tt.kind, Prompt: prompt, ReasoningEffort: tt.effort})
				require.NoError(t, err)
				status, answer, detail = result.Status, result.Answer, result.DetailCode
			} else {
				ev := service.RunConfiguredPelican(t.Context(), record.ID, "selected", tt.kind, "round", "responses", provider.PelicanQuestionConfig{Prompt: prompt, ReasoningEffort: tt.effort})
				status, answer, detail = ev.Status, ev.Answer, ev.DetailCode
			}
			require.Len(t, transport.requests, 1)
			require.Equal(t, tt.effort, gjson.Get(transport.requests[0], "reasoning.effort").String())
			require.False(t, gjson.Get(transport.requests[0], "max_output_tokens").Exists())
			require.Empty(t, detail)
			require.Equal(t, "pending_review", status)
			require.Len(t, answer, len(drawing))
			require.True(t, answer == drawing)
		})
	}
}

// TestPelicanDrawingRejectsUnfinishedOutput 完整的 XML 还需要上游成功结束本轮生成。
func TestPelicanDrawingRejectsUnfinishedOutput(t *testing.T) {
	const answer = "data: " + `{"type":"response.output_text.done","text":"<svg><path/></svg>"}` + "\n"
	for _, end := range []string{"", `data: {"type":"response.incomplete"}`, `data: {"type":"response.completed","response":{"status":"incomplete"}}`} {
		record := &provider.Record{ID: 2, Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeAPIKey, Credentials: map[string]any{"api_key": "fixture"}}
		transport := &pelicanHTTPFixture{status: http.StatusOK, stream: answer + end}
		ev := pelicanServiceForHTTP(record, transport).RunPelican(t.Context(), record.ID, "selected", "pelican", "round", "responses")
		require.Equal(t, "incomplete", ev.DetailCode)
		require.Empty(t, ev.Answer)
	}
}

// TestPelicanDrawingResponsesSnapshots 对照分段文字和终态报文，保留模型完成的整幅图。
func TestPelicanDrawingResponsesSnapshots(t *testing.T) {
	const start = `<svg xmlns="http://www.w3.org/2000/svg">`
	const end = "<circle/></svg>"
	const drawing = start + end
	delta := `{"type":"response.output_text.delta","delta":` + strconv.Quote(drawing) + `}`
	first := `{"type":"response.output_text.done","output_index":0,"content_index":0,"text":` + strconv.Quote(start) + `}`
	second := `{"type":"response.output_text.done","output_index":0,"content_index":1,"text":` + strconv.Quote(end) + `}`
	const completed = `{"type":"response.completed","response":{"status":"completed","output":[]}}`
	for _, tt := range []struct {
		name, detail string
		frames       []string
	}{
		{name: "终态省略作答内容", frames: []string{delta, completed}},
		{name: "终态只有推理摘要", frames: []string{delta, `{"type":"response.completed","response":{"output":[{"type":"reasoning","summary":[]}]}}`}},
		{name: "两段文字分别给出快照", frames: []string{first, second, completed}},
		{name: "分段索引缺失且没有完整快照", detail: "incomplete", frames: []string{
			`{"type":"response.output_text.done","output_index":0,"content_index":1,"text":` + strconv.Quote(drawing) + `}`, completed,
		}},
		{name: "消息快照后出现增量且没有完整快照", detail: "incomplete", frames: []string{
			`{"type":"response.output_item.done","output_index":0,"item":{"type":"message","content":[{"type":"output_text","text":` + strconv.Quote(drawing) + `}]}}`,
			`{"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"extra"}`, completed,
		}},
		{name: "重复分段快照", frames: []string{first, first, second, completed}},
		{name: "缺少索引的连续分段快照", frames: []string{
			`{"type":"response.output_text.done","text":` + strconv.Quote(start) + `}`,
			`{"type":"response.output_text.done","text":` + strconv.Quote(end) + `}`, completed,
		}},
		{name: "内容段的结束事件重发已完成的文字并接续下一段", frames: []string{
			`{"type":"response.output_text.done","text":` + strconv.Quote(start) + `}`,
			`{"type":"response.content_part.done","part":{"type":"output_text","text":` + strconv.Quote(start) + `}}`,
			`{"type":"response.content_part.done","part":{"type":"output_text","text":` + strconv.Quote(end) + `}}`, completed,
		}},
		{name: "兼容端终态直接给单段对象", frames: []string{`{"type":"response.completed","response":{"output":{"type":"message","content":{"type":"output_text","text":` + strconv.Quote(drawing) + `}}}}`}},
		{name: "分段快照补全增量", frames: []string{`{"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"<sv"}`, first, second, completed}},
		{name: "消息条目携带完成的图", frames: []string{`{"type":"response.output_item.done","output_index":0,"item":{"type":"message","status":"completed","content":[{"type":"output_text","text":` + strconv.Quote(drawing) + `}]}}`, completed}},
		{name: "空文字覆盖流中的图", detail: "empty_output", frames: []string{delta, `{"type":"response.completed","response":{"output":[{"type":"message","content":[{"type":"output_text","text":""}]}]}}`}},
		{name: "拒绝覆盖流中的图", detail: "incomplete", frames: []string{delta, `{"type":"response.completed","response":{"output":[{"type":"message","content":[{"type":"refusal","refusal":"declined"}]}]}}`}},
		{name: "流中出现拒绝", detail: "incomplete", frames: []string{delta, `{"type":"response.refusal.delta","delta":"declined"}`, completed}},
		{name: "终态记录截断", detail: "incomplete", frames: []string{delta, `{"type":"response.completed","response":{"incomplete_details":{"reason":"max_output_tokens"}}}`}},
		{name: "终态包含错误", detail: "upstream_error", frames: []string{delta, `{"type":"response.completed","response":{"error":{"code":"server_error"}}}`}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			record := &provider.Record{ID: 2, Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeAPIKey, Credentials: map[string]any{"api_key": "fixture"}}
			body := "data: " + strings.Join(tt.frames, "\n\ndata: ") + "\n\n"
			transport := &pelicanHTTPFixture{status: http.StatusOK, stream: body}
			ev := pelicanServiceForHTTP(record, transport).RunPelican(t.Context(), record.ID, "selected", "pelican", "round", "responses")
			require.Equal(t, tt.detail, ev.DetailCode)
			if tt.detail == "" {
				require.Equal(t, "pending_review", ev.Status)
				require.Equal(t, drawing, ev.Answer)
			} else {
				require.Equal(t, "inconclusive", ev.Status)
				require.Empty(t, ev.Answer)
			}
		})
	}
}

func TestPelicanDrawingCaptureBudgets(t *testing.T) {
	const drawing = `<svg xmlns="http://www.w3.org/2000/svg"><path d="M0 0L1 1"/></svg>`
	snapshot := "data: " + `{"type":"response.completed","response":{"output":[{"content":[{"type":"output_text","text":` + strconv.Quote(drawing) + `}]}]}}` + "\n"
	reasoning := "data: " + `{"type":"response.reasoning_summary_text.delta","delta":"` + strings.Repeat("r", 16<<10) + `"}` + "\n"
	for _, tt := range []struct {
		name, kind, eventType, detail string
		blockBytes, reasoningFrames   int
	}{
		{name: "推理记录超过八 MiB 后收到小图", kind: "pelican", reasoningFrames: 520},
		{name: "文本题按原始响应上限终止", kind: "candy", reasoningFrames: 70, detail: "output_limit"},
		{name: "最终快照覆盖较长草稿", kind: "pelican", eventType: "response.output_text.delta", blockBytes: (2 << 20) + 1},
		{name: "可见文字暂存超过四 MiB", kind: "pelican", eventType: "response.output_text.delta", blockBytes: (4 << 20) + 1, detail: "output_limit"},
		{name: "单条推理记录超过八 MiB", kind: "pelican", eventType: "response.reasoning_summary_text.delta", blockBytes: 8 << 20, detail: "output_limit"},
		{name: "原始传输超过三十二 MiB", kind: "pelican", reasoningFrames: 2048, detail: "output_limit"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var body strings.Builder
			body.Grow(tt.reasoningFrames*len(reasoning) + tt.blockBytes + len(snapshot) + 128)
			for range tt.reasoningFrames {
				_, _ = body.WriteString(reasoning)
			}
			if tt.eventType != "" {
				_, _ = body.WriteString(`data: {"type":"` + tt.eventType + `","delta":"`)
				_, _ = body.WriteString(strings.Repeat("x", tt.blockBytes))
				_, _ = body.WriteString("\"}\n")
			}
			_, _ = body.WriteString(snapshot)
			record := &provider.Record{ID: 2, Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeAPIKey, Credentials: map[string]any{"api_key": "fixture"}}
			transport := &pelicanHTTPFixture{status: http.StatusOK, stream: body.String()}
			ev := pelicanServiceForHTTP(record, transport).RunPelican(t.Context(), record.ID, "selected", tt.kind, "round", "responses")
			require.Equal(t, tt.detail, ev.DetailCode)
			if tt.detail == "" {
				require.Equal(t, "pending_review", ev.Status)
				require.Equal(t, drawing, ev.Answer)
			} else {
				require.Equal(t, "inconclusive", ev.Status)
				require.Empty(t, ev.Answer)
			}
		})
	}
}

func TestPelicanRecordsNoEffortBeforeUpstreamRequest(t *testing.T) {
	record := &provider.Record{ID: 5, Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeAPIKey}
	transport := &pelicanHTTPFixture{}
	ev := pelicanServiceForHTTP(record, transport).RunConfiguredPelican(t.Context(), record.ID, "selected", "candy", "round", "", provider.PelicanQuestionConfig{ReasoningEffort: "high"})
	require.Empty(t, transport.requests)
	require.Empty(t, ev.ReasoningEffort)
	require.Equal(t, "upstream_error", ev.DetailCode)
}

// TestIQModelTraceRestrictsMappedModelAndProtocol 检查资格预检和允许的实际端点。
func TestIQModelTraceRestrictsMappedModelAndProtocol(t *testing.T) {
	for _, tt := range []struct {
		name, mapped, protocol string
		allowed                bool
	}{
		{"明示仅用 Chat", "gpt-test", string(capability.ProtocolOpenAIChatCompletions), true},
		{"仅有图片协议", "gpt-test", string(capability.ProtocolImagesGenerations), false},
		{"别名映射到图片模型", "gpt-image-1", string(capability.ProtocolOpenAIResponses), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			record := &provider.Record{ID: 45, Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeAPIKey, Credentials: map[string]any{
				"api_key": "fixture", "model_mapping": map[string]any{"alias": tt.mapped}, provider.UpstreamProtocolsKey: []string{tt.protocol},
			}}
			transport := &pelicanHTTPFixture{status: http.StatusOK, contentType: "application/json", stream: `{"choices":[{"finish_reason":"stop","message":{"content":"1 2 3"}}]}`}
			s := pelicanServiceForHTTP(record, transport)
			events := 0
			err := s.RunModelTrace(t.Context(), record.ID, provider.IQTestRequest{ModelID: "alias"}, func(_ string, _ any) error { events++; return nil })
			if !tt.allowed {
				require.ErrorIs(t, err, provider.ErrIQUnsupported)
				require.Zero(t, events)
				require.Empty(t, transport.paths)
				return
			}
			require.NoError(t, err)
			require.Equal(t, 5, events)
			require.Len(t, transport.paths, 3)
			for _, path := range transport.paths {
				require.Equal(t, "/v1/chat/completions", path)
			}
		})
	}
}
