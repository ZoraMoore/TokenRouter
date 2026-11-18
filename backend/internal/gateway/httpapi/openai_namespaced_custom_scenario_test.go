package httpapi

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	gatewaymedia "github.com/TokenFlux/TokenRouter/internal/gateway/media"
	gatewayprovider "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	providercore "github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
)

// 通过真实 Forward 分派验证 API Key 透传路径，覆盖请求清理和响应恢复。
func TestOpenAIAPIKeyNamespacedCustomForward(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		name := "json"
		if stream {
			name = "sse"
		}
		t.Run(name, func(t *testing.T) {
			body := []byte(`{"model":"gpt-6-astra","stream":` + strconv.FormatBool(stream) + `,"tools":[{"type":"namespace","name":"functions","tools":[{"type":"custom","name":"exec"},{"type":"function","name":"wait","parameters":{"type":"object"}}]}],"input":[{"type":"custom_tool_call","name":"exec","namespace":"functions","call_id":"previous","input":"return 1"},{"type":"custom_tool_call_output","call_id":"previous","output":"1"}]}`)
			item := `{"type":"function_call","id":"fc_new","call_id":"next","name":"functions__exec","arguments":"{\"input\":\"return 42\"}","status":"completed"}`
			response := `{"id":"resp_namespace","status":"completed","output":[` + item + `],"usage":{"input_tokens":1,"output_tokens":1}}`
			contentType := "application/json"
			if stream {
				contentType = "text/event-stream"
				response = "data: " + `{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_new","call_id":"next","name":"functions__exec","arguments":"","status":"in_progress"}}` + "\n\n" +
					"data: " + `{"type":"response.function_call_arguments.done","output_index":0,"item_id":"fc_new","arguments":"{\"input\":\"return 42\"}"}` + "\n\n" +
					"data: " + `{"type":"response.output_item.done","output_index":0,"item":` + item + `}` + "\n\n" +
					"data: " + `{"type":"response.completed","response":` + response + `}` + "\n\n"
			}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			upstream := &auxiliaryHTTPRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(response))}}
			account := &gatewayprovider.ExecutionProvider{Record: providercore.Record{LoadLocation: time.LoadLocation, ID: 9001, Platform: capability.PlatformOpenAI, Type: capability.ProviderTypeAPIKey, Credentials: map[string]any{"api_key": "test-key", "base_url": "https://relay.example/v1"}, Extra: map[string]any{"openai_passthrough": true, "openai_text_route_mode": "force_responses"}}}
			result, err := openAIClientToolsTestService(upstream).Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, "functions__exec", gjson.GetBytes(upstream.lastBody, "tools.0.name").String())
			require.Equal(t, "function", gjson.GetBytes(upstream.lastBody, "tools.0.type").String())
			require.Equal(t, "functions__exec", gjson.GetBytes(upstream.lastBody, "input.0.name").String())
			require.Equal(t, "function_call", gjson.GetBytes(upstream.lastBody, "input.0.type").String())
			output := recorder.Body.String()
			require.Contains(t, output, `"namespace":"functions"`)
			require.Contains(t, output, `"name":"exec"`)
			require.Contains(t, output, `"type":"custom_tool_call"`)
			require.Contains(t, output, `"input":"return 42"`)
			require.NotContains(t, output, "functions__exec")
		})
	}
}

// Codex Desktop 的 Responses Lite 请求把 namespace 目录放在 input.additional_tools；
// Excel sidecar 还原后的 custom 调用必须由 API Key 透传路径原样交还客户端。
func TestOpenAIAPIKeyResponsesLiteAdditionalToolsForward(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"model":"gpt-6-astra",
		"stream":true,
		"input":[
			{"type":"additional_tools","role":"developer","tools":[
				{"type":"namespace","name":"functions","tools":[
					{"type":"custom","name":"exec"}
				]}
			]},
			{"type":"message","role":"user","content":"inspect"}
		]
	}`)
	sse := strings.Join([]string{
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"custom_tool_call","id":"ctc_new","call_id":"next","namespace":"functions","name":"exec","input":"","status":"in_progress"}}`,
		`data: {"type":"response.custom_tool_call_input.delta","output_index":0,"item_id":"ctc_new","delta":"return 42"}`,
		`data: {"type":"response.custom_tool_call_input.done","output_index":0,"item_id":"ctc_new","call_id":"next","namespace":"functions","name":"exec","input":"return 42"}`,
		`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"custom_tool_call","id":"ctc_new","call_id":"next","namespace":"functions","name":"exec","input":"return 42","status":"completed"}}`,
		`data: {"type":"response.completed","response":{"id":"resp_namespace_lite","status":"completed","output":[{"type":"custom_tool_call","id":"ctc_new","call_id":"next","namespace":"functions","name":"exec","input":"return 42","status":"completed"}],"usage":{"input_tokens":1,"output_tokens":1}}}`,
	}, "\n\n") + "\n\n"
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set(gatewaymedia.ResponsesLiteHeader, "true")
	upstream := &auxiliaryHTTPRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(sse)),
	}}
	account := &gatewayprovider.ExecutionProvider{Record: providercore.Record{
		LoadLocation: time.LoadLocation,
		ID:           9002,
		Platform:     capability.PlatformOpenAI,
		Type:         capability.ProviderTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "test-key",
			"base_url": "https://relay.example/v1",
		},
		Extra: map[string]any{
			"openai_passthrough":     true,
			"openai_text_route_mode": "force_responses",
		},
	}}

	result, err := openAIClientToolsTestService(upstream).Forward(
		context.Background(), c, account, body,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "additional_tools", gjson.GetBytes(upstream.lastBody, "input.0.type").String())
	require.Equal(t, "functions", gjson.GetBytes(upstream.lastBody, "input.0.tools.0.name").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "tools").Exists())
	require.Equal(t, false, gjson.GetBytes(upstream.lastBody, "parallel_tool_calls").Bool())
	output := recorder.Body.String()
	require.Contains(t, output, `"type":"custom_tool_call"`)
	require.Contains(t, output, `"namespace":"functions"`)
	require.Contains(t, output, `"name":"exec"`)
	require.Contains(t, output, `"input":"return 42"`)
}
