package googleforward_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	forwardcore "github.com/TokenFlux/TokenRouter/internal/gateway/forward"
	gatewayhttp "github.com/TokenFlux/TokenRouter/internal/gateway/httpapi"
	gatewayprovider "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	"github.com/TokenFlux/TokenRouter/internal/gateway/provider/googleforward"
	providercore "github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
)

type geminiResponsesFailingStream struct {
	read bool
}

func TestGeminiForwardAsResponsesReturnsResponsesFormat(t *testing.T) {
	upstreamBody := `{
		"candidates":[{"content":{"parts":[
			{"text":"inspect inputs","thought":true},
			{"text":"calling tool"},
			{"functionCall":{"name":"get_weather","args":{"city":"Tokyo"}}}
		]},"finishReason":"STOP"}],
		"usageMetadata":{"promptTokenCount":7,"candidatesTokenCount":3,"thoughtsTokenCount":5}
	}`
	httpStub := &geminiCompatHTTPUpstreamStub{response: &http.Response{
		StatusCode: http.StatusOK,

		Header: http.Header{"X-Request-Id": []string{"gemini-response-1"}},

		Body: io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := newGeminiFixture(geminiDependencies{httpUpstream: httpStub, cfg: &googleforward.Options{}})
	provider := &gatewayprovider.ExecutionProvider{
		Record: providercore.Record{
			LoadLocation: time.LoadLocation,
			ID:           201,

			Platform: capability.PlatformGemini,

			Type: capability.ProviderTypeAPIKey,

			Concurrency: 1,

			Credentials: map[string]any{
				"api_key": "gemini-key",
				"model_mapping": map[string]any{
					"group-model": "gemini-2.5-pro",
				},
			},
		},
	}
	body := []byte(`{"model":"group-model","input":"weather","tools":[{"type":"function","name":"get_weather","parameters":{"type":"object"}}]}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))

	result, err := svc.ForwardAsResponses(context.Background(), gatewayhttp.NewGoogleBoundary(c, svc.Options, false), provider, body, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "group-model", result.Model)
	require.Equal(t, "gemini-2.5-pro", result.UpstreamModel)
	require.Equal(t, "gemini-response-1", result.RequestID)
	require.Equal(t, 7, result.Usage.InputTokens)
	require.Equal(t, 8, result.Usage.OutputTokens)
	require.Equal(t, "response", gjson.GetBytes(recorder.Body.Bytes(), "object").String())
	require.Equal(t, "group-model", gjson.GetBytes(recorder.Body.Bytes(), "model").String())
	require.Equal(t, "inspect inputs", gjson.GetBytes(recorder.Body.Bytes(), `output.#(type=="reasoning").summary.0.text`).String())
	require.Equal(t, "get_weather", gjson.GetBytes(recorder.Body.Bytes(), `output.#(type=="function_call").name`).String())
	require.Equal(t, "calling tool", gjson.GetBytes(recorder.Body.Bytes(), `output.#(type=="message").content.0.text`).String())
	require.Contains(t, httpStub.lastReq.URL.String(), "/models/gemini-2.5-pro:generateContent")
}

func TestGeminiForwardAsResponsesOAuthCollectsReasoningTextAndTools(t *testing.T) {
	upstreamBody := strings.Join([]string{
		`data: {"response":{"candidates":[{"content":{"parts":[{"text":"plan ","thought":true}]}}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":1,"thoughtsTokenCount":1}}}`,

		`data: {"response":{"candidates":[{"content":{"parts":[{"text":"carefully","thought":true}]}}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":1,"thoughtsTokenCount":2}}}`,

		`data: {"response":{"candidates":[{"content":{"parts":[{"text":"answer"}]}}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2,"thoughtsTokenCount":2}}}`,

		`data: {"response":{"candidates":[{"content":{"parts":[{"functionCall":{"name":"lookup","args":{"id":42}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2,"thoughtsTokenCount":2}}}`,

		"data: [DONE]",

		"",
	}, "\n\n")
	httpStub := &geminiCompatHTTPUpstreamStub{response: &http.Response{
		StatusCode: http.StatusOK,

		Header: http.Header{"Content-Type": []string{"text/event-stream"}},

		Body: io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := newGeminiFixture(geminiDependencies{
		tokenProvider: newGeminiTokenSourceForTest(),

		httpUpstream: httpStub,

		cfg: &googleforward.Options{},
	})
	provider := &gatewayprovider.ExecutionProvider{
		Record: providercore.Record{
			LoadLocation: time.LoadLocation,
			ID:           204,

			Platform: capability.PlatformGemini,

			Type: capability.ProviderTypeOAuth,

			Concurrency: 1,

			Credentials: map[string]any{
				"access_token": "ya29.test-token",
				"project_id":   "project-1",
			},
		},
	}
	body := []byte(`{"model":"gemini-2.5-pro","input":"hello","tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}]}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))

	result, err := svc.ForwardAsResponses(context.Background(), gatewayhttp.NewGoogleBoundary(c, svc.Options, false), provider, body, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Stream)
	require.Equal(t, 5, result.Usage.InputTokens)
	require.Equal(t, 4, result.Usage.OutputTokens)
	require.Equal(t, "plan carefully", gjson.GetBytes(recorder.Body.Bytes(), `output.#(type=="reasoning").summary.0.text`).String())
	require.Equal(t, "answer", gjson.GetBytes(recorder.Body.Bytes(), `output.#(type=="message").content.0.text`).String())
	require.Equal(t, "lookup", gjson.GetBytes(recorder.Body.Bytes(), `output.#(type=="function_call").name`).String())
	require.Contains(t, httpStub.lastReq.URL.String(), "/v1internal:streamGenerateContent?alt=sse")
}

func TestGeminiForwardAsResponsesStreamsReasoningTextToolAndUsage(t *testing.T) {
	upstreamBody := strings.Join([]string{
		`data: {"candidates":[{"content":{"parts":[{"text":"plan","thought":true}]}}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1,"thoughtsTokenCount":1}}`,

		`data: {"candidates":[{"content":{"parts":[{"text":"hello"}]}}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":2,"thoughtsTokenCount":1}}`,

		`data: {"candidates":[{"content":{"parts":[{"functionCall":{"name":"lookup","args":{"id":42}}}]} ,"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":2,"thoughtsTokenCount":1}}`,

		"data: [DONE]",

		"",
	}, "\n\n")
	httpStub := &geminiCompatHTTPUpstreamStub{response: &http.Response{
		StatusCode: http.StatusOK,

		Header: http.Header{"Content-Type": []string{"text/event-stream"}},

		Body: io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := newGeminiFixture(geminiDependencies{httpUpstream: httpStub, cfg: &googleforward.Options{}})
	provider := &gatewayprovider.ExecutionProvider{Record: providercore.Record{
		LoadLocation: time.LoadLocation,
		ID:           202,
		Platform:     capability.PlatformGemini,
		Type:         capability.ProviderTypeAPIKey,
		Concurrency:  1,
		Credentials:  map[string]any{"api_key": "gemini-key"},
	}}
	body := []byte(`{"model":"gemini-2.5-flash","input":"hello","stream":true,"tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}]}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))

	result, err := svc.ForwardAsResponses(context.Background(), gatewayhttp.NewGoogleBoundary(c, svc.Options, false), provider, body, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.NotNil(t, result.FirstTokenMs)
	require.Equal(t, 2, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
	streamBody := recorder.Body.String()
	require.Contains(t, streamBody, "event: response.created")
	require.Contains(t, streamBody, "event: response.reasoning_summary_text.delta")
	require.Contains(t, streamBody, `"delta":"plan"`)
	require.Contains(t, streamBody, "event: response.output_text.delta")
	require.Contains(t, streamBody, `"delta":"hello"`)
	require.Contains(t, streamBody, "event: response.function_call_arguments.done")
	require.Contains(t, streamBody, `"name":"lookup"`)
	require.Contains(t, streamBody, "event: response.completed")
	require.NotContains(t, streamBody, "data: [DONE]")
}

func TestGeminiForwardAsResponsesCommitsStreamBeforeReadFailure(t *testing.T) {
	httpStub := &geminiCompatHTTPUpstreamStub{response: &http.Response{
		StatusCode: http.StatusOK,

		Header: http.Header{"Content-Type": []string{"text/event-stream"}},

		Body: &geminiResponsesFailingStream{},
	}}
	svc := newGeminiFixture(geminiDependencies{httpUpstream: httpStub, cfg: &googleforward.Options{}})
	provider := &gatewayprovider.ExecutionProvider{Record: providercore.Record{
		LoadLocation: time.LoadLocation,
		ID:           203,
		Platform:     capability.PlatformGemini,
		Type:         capability.ProviderTypeAPIKey,
		Concurrency:  1,
		Credentials:  map[string]any{"api_key": "gemini-key"},
	}}
	body := []byte(`{"model":"gemini-2.5-flash","input":"hello","stream":true}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))

	_, err := svc.ForwardAsResponses(context.Background(), gatewayhttp.NewGoogleBoundary(c, svc.Options, false), provider, body, nil)

	require.ErrorContains(t, err, "stream_read_error")
	require.Positive(t, recorder.Body.Len(), "首个字节写出后 handler 必须禁止 failover")
}

func TestGeminiForwardAsResponsesMapsUpstreamError(t *testing.T) {
	httpStub := &geminiCompatHTTPUpstreamStub{response: &http.Response{
		StatusCode: http.StatusBadRequest,

		Header: http.Header{"X-Goog-Request-Id": []string{"gemini-error-1"}},

		Body: io.NopCloser(strings.NewReader(
			`{"error":{"code":400,"message":"invalid generation request","status":"INVALID_ARGUMENT"}}`,
		)),
	}}
	svc := newGeminiFixture(geminiDependencies{httpUpstream: httpStub, cfg: &googleforward.Options{}})
	provider := &gatewayprovider.ExecutionProvider{Record: providercore.Record{
		LoadLocation: time.LoadLocation,
		ID:           205,
		Platform:     capability.PlatformGemini,
		Type:         capability.ProviderTypeAPIKey,
		Concurrency:  1,
		Credentials:  map[string]any{"api_key": "gemini-key"},
	}}
	body := []byte(`{"model":"gemini-2.5-flash","input":"hello"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))

	result, err := svc.ForwardAsResponses(context.Background(), gatewayhttp.NewGoogleBoundary(c, svc.Options, false), provider, body, nil)

	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Equal(t, "invalid_request_error", gjson.GetBytes(recorder.Body.Bytes(), "error.code").String())
	require.Equal(t, "Invalid request", gjson.GetBytes(recorder.Body.Bytes(), "error.message").String())
}

func TestGeminiForwardAsResponsesReturnsFailoverBeforeResponseStarts(t *testing.T) {
	httpStub := &geminiCompatHTTPUpstreamStub{response: &http.Response{
		StatusCode: http.StatusForbidden,

		Header: http.Header{
			"Www-Authenticate": []string{`Bearer error="insufficient_scope"`},

			"X-Goog-Request-Id": []string{"gemini-failover-1"},
		},

		Body: io.NopCloser(strings.NewReader(
			`{"error":{"code":403,"message":"insufficient authentication scope","status":"PERMISSION_DENIED"}}`,
		)),
	}}
	svc := newGeminiFixture(geminiDependencies{httpUpstream: httpStub, cfg: &googleforward.Options{}})
	provider := &gatewayprovider.ExecutionProvider{Record: providercore.Record{
		LoadLocation: time.LoadLocation,
		ID:           206,
		Platform:     capability.PlatformGemini,
		Type:         capability.ProviderTypeAPIKey,
		Concurrency:  1,
		Credentials:  map[string]any{"api_key": "gemini-key"},
	}}
	body := []byte(`{"model":"gemini-2.5-flash","input":"hello"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))

	result, err := svc.ForwardAsResponses(context.Background(), gatewayhttp.NewGoogleBoundary(c, svc.Options, false), provider, body, nil)

	require.Nil(t, result)
	var failoverErr *forwardcore.UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusForbidden, failoverErr.StatusCode)
	require.Zero(t, recorder.Body.Len())
}

func TestGeminiForwardAsChatCompletions_CustomCodesMiss400HiddenAs500(t *testing.T) {
	svc, _ := newGeminiErrorFixture(http.StatusBadRequest, geminiSkippedTestUpstreamBody())
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"hi"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))

	result, err := svc.ForwardAsChatCompletions(context.Background(), gatewayhttp.NewGoogleBoundary(c, svc.Options, false), geminiCustomCodesAPIKeyProvider(), body)

	require.Nil(t, result)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not in custom error codes")
	require.Equal(t, http.StatusInternalServerError, rec.Code)

	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	errObj, ok := got["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "api_error", errObj["type"])
	require.Equal(t, "Upstream gateway error", errObj["message"])
}

func TestGeminiForwardAsChatCompletions_PoolMode400KeepsUpstreamMessage(t *testing.T) {
	svc, _ := newGeminiErrorFixture(http.StatusBadRequest, geminiSkippedTestUpstreamBody())
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"hi"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))

	result, err := svc.ForwardAsChatCompletions(context.Background(), gatewayhttp.NewGoogleBoundary(c, svc.Options, false), geminiPoolModeAPIKeyProvider(), body)

	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, rec.Code, "状态码应保真为上游 400")

	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	errObj, ok := got["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "invalid_request_error", errObj["type"])
	require.Equal(t, geminiSkippedTestUpstreamMsg, errObj["message"], "应回传上游 message")
}

func TestGeminiForwardAsChatCompletions_OAuthRoutesToGeminiAndReturnsChatFormat(t *testing.T) {
	upstreamBody := `data: {"response":{"candidates":[{"content":{"parts":[{"text":"hello from gemini"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":7,"candidatesTokenCount":3}}}` + "\n\n" +
		"data: [DONE]\n\n"
	httpStub := &geminiCompatHTTPUpstreamStub{
		response: &http.Response{
			StatusCode: http.StatusOK,

			Header: http.Header{"Content-Type": []string{"text/event-stream"}},

			Body: io.NopCloser(strings.NewReader(upstreamBody)),
		},
	}
	svc := newGeminiFixture(geminiDependencies{
		tokenProvider: newGeminiTokenSourceForTest(),

		httpUpstream: httpStub,

		cfg: &googleforward.Options{},
	})
	provider := &gatewayprovider.ExecutionProvider{
		Record: providercore.Record{
			LoadLocation: time.LoadLocation,
			ID:           101,

			Platform: capability.PlatformGemini,

			Type: capability.ProviderTypeOAuth,

			Credentials: map[string]any{
				"access_token": "ya29.test-token",

				"project_id": "project-1",

				"model_mapping": map[string]any{
					"gemini-2.5-flash": "gemini-2.5-flash-upstream",
				},
			},

			Concurrency: 1,
		},
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"hi"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))

	result, err := svc.ForwardAsChatCompletions(context.Background(), gatewayhttp.NewGoogleBoundary(c, svc.Options, false), provider, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "gemini-2.5-flash", result.Model)
	require.Equal(t, "gemini-2.5-flash-upstream", result.UpstreamModel)
	require.Equal(t, 7, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
	require.Equal(t, "hello from gemini", gjson.GetBytes(rec.Body.Bytes(), "choices.0.message.content").String())

	require.NotNil(t, httpStub.lastReq)
	require.Contains(t, httpStub.lastReq.URL.String(), "/v1internal:streamGenerateContent?alt=sse")
	require.Equal(t, "Bearer ya29.test-token", httpStub.lastReq.Header.Get("Authorization"))
	require.Empty(t, httpStub.lastReq.Header.Get("x-api-key"))
	require.Empty(t, httpStub.lastReq.Header.Get("anthropic-version"))

	var sent map[string]any
	sentBody, err := io.ReadAll(httpStub.lastReq.Body)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(sentBody, &sent))
	require.Equal(t, "gemini-2.5-flash-upstream", sent["model"])
	require.Equal(t, "project-1", sent["project"])
	require.Contains(t, fmt.Sprint(sent["request"]), "hi")

	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "chat.completion", got["object"])
	require.Equal(t, "gemini-2.5-flash", got["model"])
	choices, ok := got["choices"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, choices)
	choice, ok := choices[0].(map[string]any)
	require.True(t, ok)
	message, ok := choice["message"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "assistant", message["role"])
	require.Equal(t, "hello from gemini", message["content"])
	usage, ok := got["usage"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(7), usage["prompt_tokens"])
	require.Equal(t, float64(3), usage["completion_tokens"])
	require.Equal(t, float64(10), usage["total_tokens"])
}

func TestGeminiForwardAsChatCompletions_StreamsOpenAIChunksFromGeminiSSE(t *testing.T) {
	upstreamBody := `data: {"candidates":[{"content":{"parts":[{"text":"hel"}]}}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1}}` + "\n\n" +
		`data: {"candidates":[{"content":{"parts":[{"text":"hello"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":2}}` + "\n\n" +
		"data: [DONE]\n\n"
	httpStub := &geminiCompatHTTPUpstreamStub{
		response: &http.Response{
			StatusCode: http.StatusOK,

			Header: http.Header{"Content-Type": []string{"text/event-stream"}},

			Body: io.NopCloser(strings.NewReader(upstreamBody)),
		},
	}
	svc := newGeminiFixture(geminiDependencies{
		httpUpstream: httpStub,
		cfg:          &googleforward.Options{},
	})
	provider := &gatewayprovider.ExecutionProvider{
		Record: providercore.Record{
			LoadLocation: time.LoadLocation,
			ID:           102,

			Platform: capability.PlatformGemini,

			Type: capability.ProviderTypeAPIKey,

			Credentials: map[string]any{
				"api_key": "gemini-api-key",
			},

			Concurrency: 1,
		},
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gemini-2.5-flash","stream":true,"stream_options":{"include_usage":true},"messages":[{"role":"user","content":"hi"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))

	result, err := svc.ForwardAsChatCompletions(context.Background(), gatewayhttp.NewGoogleBoundary(c, svc.Options, false), provider, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, result.Stream)
	require.Equal(t, 2, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)

	require.NotNil(t, httpStub.lastReq)
	require.Contains(t, httpStub.lastReq.URL.String(), "/v1beta/models/gemini-2.5-flash:streamGenerateContent?alt=sse")
	require.Equal(t, "gemini-api-key", httpStub.lastReq.Header.Get("x-goog-api-key"))

	out := rec.Body.String()
	require.Contains(t, out, `"object":"chat.completion.chunk"`)
	require.Contains(t, out, `"role":"assistant"`)
	require.Contains(t, out, `"content":"hel"`)
	require.Contains(t, out, `"content":"lo"`)
	require.Contains(t, out, `"usage":{"prompt_tokens":2,"completion_tokens":2,"total_tokens":4}`)
	require.Contains(t, out, "data: [DONE]")
}

func TestGeminiForwardAsChatCompletions_FunctionNamedWebSearchStaysClientSide(t *testing.T) {
	httpStub := &geminiCompatHTTPUpstreamStub{
		response: &http.Response{
			StatusCode: http.StatusOK,

			Header: http.Header{"Content-Type": []string{"application/json"}},

			Body: io.NopCloser(strings.NewReader(
				`{"candidates":[{"content":{"parts":[{"text":"hello"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1}}`,
			)),
		},
	}
	svc := newGeminiFixture(geminiDependencies{
		httpUpstream: httpStub,
		cfg:          &googleforward.Options{},
	})
	provider := &gatewayprovider.ExecutionProvider{
		Record: providercore.Record{
			LoadLocation: time.LoadLocation,
			ID:           103,

			Platform: capability.PlatformGemini,

			Type: capability.ProviderTypeAPIKey,

			Credentials: map[string]any{
				"api_key": "gemini-api-key",
			},

			Concurrency: 1,
		},
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{
		"model":"gemini-3.6-flash-high",
		"messages":[{"role":"user","content":"search and read"}],
		"tools":[
			{"type":"function","function":{"name":"web_search","description":"Search through the Hermes client","parameters":{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}}},
			{"type":"function","function":{"name":"read_file","description":"Read a local file","parameters":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}}}
		]
	}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))

	result, err := svc.ForwardAsChatCompletions(context.Background(), gatewayhttp.NewGoogleBoundary(c, svc.Options, false), provider, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, httpStub.lastReq)

	postedBody, err := io.ReadAll(httpStub.lastReq.Body)
	require.NoError(t, err)

	var posted map[string]any
	require.NoError(t, json.Unmarshal(postedBody, &posted))
	tools, ok := posted["tools"].([]any)
	require.True(t, ok)
	require.Len(t, tools, 1, "Chat Completions function tools must not be promoted to Gemini built-ins by name")

	functionTool, ok := tools[0].(map[string]any)
	require.True(t, ok)
	functionDecls, ok := functionTool["functionDeclarations"].([]any)
	require.True(t, ok)
	require.Len(t, functionDecls, 2)
	webSearchDecl, ok := functionDecls[0].(map[string]any)
	require.True(t, ok)
	readFileDecl, ok := functionDecls[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "web_search", webSearchDecl["name"])
	require.Equal(t, "read_file", readFileDecl["name"])
	require.NotContains(t, functionTool, "googleSearch")
	require.NotContains(t, functionTool, "google_search")
}

func (r *geminiResponsesFailingStream) Read(p []byte) (int, error) {
	if r.read {
		return 0, errors.New("upstream stream failed")
	}
	r.read = true
	return copy(p, []byte("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"hello\"}]}}]}\n\n")), nil
}

func (r *geminiResponsesFailingStream) Close() error { return nil }

// TestGeminiPartialUsageSurvivesProvider 验证服务层返回断流之前的用量。
func TestGeminiPartialUsageSurvivesProvider(t *testing.T) {
	for _, name := range []string{"chat", "responses", "messages", "native"} {
		t.Run(name, func(t *testing.T) {
			stream := "data: " + `{"candidates":[{"content":{"parts":[{"text":"hello"}]}}],"usageMetadata":{"promptTokenCount":100,"cachedContentTokenCount":60,"candidatesTokenCount":10}}` + "\n\n"
			stub := &geminiCompatHTTPUpstreamStub{response: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(&antigravityCompatErrorReader{data: []byte(stream), err: io.ErrUnexpectedEOF})}}
			svc := newGeminiFixture(geminiDependencies{httpUpstream: stub, cfg: &googleforward.Options{}})
			provider := geminiPoolModeAPIKeyProvider()
			body := []byte(`{"model":"gemini-2.5-flash","input":"hello","messages":[{"role":"user","content":"hello"}],"stream":true}`)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/"+name, bytes.NewReader(body))
			var result *forwardcore.MessagesResult
			var err error
			switch name {
			case "chat":
				result, err = svc.ForwardAsChatCompletions(t.Context(), gatewayhttp.NewGoogleBoundary(c, svc.Options, false), provider, body)
			case "responses":
				result, err = svc.ForwardAsResponses(t.Context(), gatewayhttp.NewGoogleBoundary(c, svc.Options, false), provider, body, nil)
			case "messages":
				body = []byte(`{"model":"gemini-2.5-flash","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":true}`)
				result, err = svc.Forward(t.Context(), gatewayhttp.NewGoogleBoundary(c, svc.Options, false), provider, body)
			case "native":
				body = []byte(`{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`)
				result, err = svc.ForwardNative(t.Context(), gatewayhttp.NewGoogleBoundary(c, svc.Options, false), provider, "gemini-2.5-flash", "streamGenerateContent", true, body)
			}
			require.Error(t, err)
			require.NotNil(t, result, "执行器已收到用量，转发层需要把结果交给完成处理")
			require.Equal(t, 40, result.Usage.InputTokens)
			require.Equal(t, 60, result.Usage.CacheReadInputTokens)
		})
	}
}

// TestGeminiDisconnectFlag 验证客户端写入失败状态传到网关。
func TestGeminiDisconnectFlag(t *testing.T) {
	for _, kind := range []string{"responses", "chat", "messages", "native"} {
		t.Run(kind, func(t *testing.T) {
			body := []byte(`{"model":"gemini-2.5-flash","input":"hello","stream":true}`)
			stream := "data: " + `{"candidates":[{"content":{"parts":[{"text":"hello"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":100,"cachedContentTokenCount":60,"candidatesTokenCount":10}}` + "\n\n"
			stub := &geminiCompatHTTPUpstreamStub{response: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream))}}
			svc := newGeminiFixture(geminiDependencies{httpUpstream: stub, cfg: &googleforward.Options{}})
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			c.Writer = &antigravityFailingWriter{ResponseWriter: c.Writer, failAfter: 0}
			var result *forwardcore.MessagesResult
			var err error
			provider := geminiPoolModeAPIKeyProvider()
			output := gatewayhttp.NewGoogleBoundary(c, svc.Options, false)
			switch kind {
			case "responses":
				result, err = svc.ForwardAsResponses(t.Context(), output, provider, body, nil)
			case "chat":
				body = []byte(`{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"hello"}],"stream":true}`)
				result, err = svc.ForwardAsChatCompletions(t.Context(), output, provider, body)
			case "messages":
				body = []byte(`{"model":"gemini-2.5-flash","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":true}`)
				result, err = svc.Forward(t.Context(), output, provider, body)
			case "native":
				body = []byte(`{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`)
				result, err = svc.ForwardNative(t.Context(), output, provider, "gemini-2.5-flash", "streamGenerateContent", true, body)
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			require.True(t, result.ClientDisconnect, "客户端写入已经失败，返回结果仍标为未断开")
		})
	}
}

// TestGeminiStreamErrorIsDelivered 验证 HTTP 200 流中的上游错误会输出给客户端。
func TestGeminiStreamErrorIsDelivered(t *testing.T) {
	body := []byte(`{"model":"gemini-2.5-flash","input":"hello","stream":true}`)
	stream := "data: " + `{"candidates":[{"content":{"parts":[{"text":"hello"}]}}],"usageMetadata":{"promptTokenCount":100,"cachedContentTokenCount":60,"candidatesTokenCount":1}}` + "\n\ndata: " + `{"error":{"code":429,"message":"quota","status":"RESOURCE_EXHAUSTED"}}` + "\n\n"
	stub := &geminiCompatHTTPUpstreamStub{response: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream))}}
	svc := newGeminiFixture(geminiDependencies{httpUpstream: stub, cfg: &googleforward.Options{}})
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	_, err := svc.ForwardAsResponses(t.Context(), gatewayhttp.NewGoogleBoundary(c, svc.Options, false), geminiPoolModeAPIKeyProvider(), body, nil)
	require.Error(t, err)
	require.Contains(t, recorder.Body.String(), `"error":{`, "客户端已接收正文，后续错误需要一个 SSE 错误事件")
}

// TestGeminiNativeStreamErrorIsDelivered 检查原生入口的流式错误输出。
func TestGeminiNativeStreamErrorIsDelivered(t *testing.T) {
	body := []byte(`{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`)
	stream := "data: " + `{"candidates":[{"content":{"parts":[{"text":"hello"}]}}],"usageMetadata":{"promptTokenCount":100,"cachedContentTokenCount":60,"candidatesTokenCount":1}}` + "\n\ndata: " + `{"error":{"code":429,"message":"quota","status":"RESOURCE_EXHAUSTED"}}` + "\n\n"
	stub := &geminiCompatHTTPUpstreamStub{response: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream))}}
	svc := newGeminiFixture(geminiDependencies{httpUpstream: stub, cfg: &googleforward.Options{}})
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-flash:streamGenerateContent", bytes.NewReader(body))
	result, err := svc.ForwardNative(t.Context(), gatewayhttp.NewGoogleBoundary(c, svc.Options, false), geminiPoolModeAPIKeyProvider(), "gemini-2.5-flash", "streamGenerateContent", true, body)
	require.Error(t, err)
	require.NotNil(t, result)
	require.Equal(t, 60, result.Usage.CacheReadInputTokens)
	var failover *forwardcore.UpstreamFailoverError
	require.False(t, errors.As(err, &failover))
	require.Contains(t, recorder.Body.String(), `"text":"hello"`)
	require.Contains(t, recorder.Body.String(), `"error":{`, "Google 原生客户端需要收到上游错误")
}

// TestGeminiEmbeddedErrorBeforeOutputCanRetry 检查未输出内容前的流式错误继续使用池模式重试规则。
func TestGeminiEmbeddedErrorBeforeOutputCanRetry(t *testing.T) {
	payload := "data: " + `{"error":{"code":429,"message":"quota","status":"RESOURCE_EXHAUSTED"}}` + "\n\n"
	stub := &geminiCompatHTTPUpstreamStub{response: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(payload))}}
	svc := newGeminiFixture(geminiDependencies{httpUpstream: stub, cfg: &googleforward.Options{}})
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	body := []byte(`{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/test:streamGenerateContent", bytes.NewReader(body))
	result, err := svc.ForwardNative(t.Context(), gatewayhttp.NewGoogleBoundary(c, svc.Options, false), geminiPoolModeAPIKeyProvider(), "gemini-2.5-flash", "streamGenerateContent", true, body)
	require.Nil(t, result)
	var failover *forwardcore.UpstreamFailoverError
	require.ErrorAs(t, err, &failover)
	require.Equal(t, 429, failover.StatusCode)
	require.True(t, failover.RetryableOnSameProvider)
	require.False(t, c.Writer.Written())
}
