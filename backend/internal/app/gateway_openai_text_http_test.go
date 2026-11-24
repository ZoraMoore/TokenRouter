package app

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/TokenFlux/TokenRouter/internal/apikey"
	keyhttp "github.com/TokenFlux/TokenRouter/internal/apikey/httpapi"
	"github.com/TokenFlux/TokenRouter/internal/apikey/testkit"
	"github.com/TokenFlux/TokenRouter/internal/app/lifecycle"
	"github.com/TokenFlux/TokenRouter/internal/billing"
	billingtestkit "github.com/TokenFlux/TokenRouter/internal/billing/testkit"
	"github.com/TokenFlux/TokenRouter/internal/config"
	"github.com/TokenFlux/TokenRouter/internal/egress"
	"github.com/TokenFlux/TokenRouter/internal/gateway/failover"
	forwardcore "github.com/TokenFlux/TokenRouter/internal/gateway/forward"
	gatewayhttp "github.com/TokenFlux/TokenRouter/internal/gateway/httpapi"
	httptestkit "github.com/TokenFlux/TokenRouter/internal/gateway/httpapi/testkit"
	"github.com/TokenFlux/TokenRouter/internal/gateway/moderationflow"
	gatewayprovider "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	"github.com/TokenFlux/TokenRouter/internal/gateway/provider/googleforward"
	"github.com/TokenFlux/TokenRouter/internal/gateway/provider/messageforward"
	"github.com/TokenFlux/TokenRouter/internal/gateway/provider/selection"
	"github.com/TokenFlux/TokenRouter/internal/gateway/requeststate"
	"github.com/TokenFlux/TokenRouter/internal/gateway/searchtools"
	gatewaytelemetry "github.com/TokenFlux/TokenRouter/internal/gateway/telemetry"
	gatewaytestkit "github.com/TokenFlux/TokenRouter/internal/gateway/testkit"
	textflow "github.com/TokenFlux/TokenRouter/internal/gateway/text"
	"github.com/TokenFlux/TokenRouter/internal/identity"
	"github.com/TokenFlux/TokenRouter/internal/identity/httpapi/authctx"
	"github.com/TokenFlux/TokenRouter/internal/infra/httpclient"
	"github.com/TokenFlux/TokenRouter/internal/infra/httpclient/tlsfingerprint"
	"github.com/TokenFlux/TokenRouter/internal/infra/telemetry/logging"
	"github.com/TokenFlux/TokenRouter/internal/moderation"
	opscore "github.com/TokenFlux/TokenRouter/internal/ops"
	opsprovider "github.com/TokenFlux/TokenRouter/internal/ops/provider"
	"github.com/TokenFlux/TokenRouter/internal/protocol"
	protocolopenai "github.com/TokenFlux/TokenRouter/internal/protocol/openai"
	providercore "github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
	routingtestkit "github.com/TokenFlux/TokenRouter/internal/routing/testkit"
	"github.com/TokenFlux/TokenRouter/internal/scheduler"
	xai "github.com/TokenFlux/TokenRouter/internal/upstream/grok"
)

// mockTempUnscheduler 记录 TempUnscheduleRetryableError 的调用信息。
type mockTempUnscheduler struct {
	calls []tempUnscheduleCall
}

type tempUnscheduleCall struct {
	providerID  int64
	failoverErr *forwardcore.UpstreamFailoverError
}

type countingGatewaySchedulerCache struct {
	*fakeSchedulerCache
	snapshotCalls atomic.Int64
}

// cyberSessionBlockHandlerCacheStub 模拟已命中的会话屏蔽缓存，并记录读取次数。
type cyberSessionBlockHandlerCacheStub struct {
	blocked   bool
	readCalls int
}

type openAIHTTPPassthroughFailoverUpstream struct {
	httpclient.
		UpstreamTransport
	mu          sync.Mutex
	providerIDs []int64
}

type openAIHTTPPassthroughAuthFailoverUpstream struct {
	httpclient.
		UpstreamTransport
	mu          sync.Mutex
	providerIDs []int64
	statusCode  int
}

type openAIHTTPPassthroughSSERateLimitUpstream struct {
	httpclient.
		UpstreamTransport
	mu          sync.Mutex
	providerIDs []int64
}

// openAIResponsesFailoverProviderRepo 为 failover 用例提供按平台选号和提供商回读。
type openAIResponsesFailoverProviderRepo struct {
	gatewayprovider.ExecutionProviderStore

	providers []gatewayprovider.ExecutionProvider
}

// openAIResponsesFailoverCancelUpstream 固定返回 HTTP 520，可在首次上游调用时
// 触发回调（用于模拟“上游在途期间客户端断开”）。
type openAIResponsesFailoverCancelUpstream struct {
	httpclient.
		UpstreamTransport
	mu          sync.Mutex
	providerIDs []int64
	onFirstDo   func()
}

// mixedHTTPTransport 通过本地 HTTP server 记录选中的提供商和上游端点。
type mixedHTTPTransport struct {
	mu        sync.Mutex
	providers []int64
}

type mixedHTTPNoSearch struct{}

type responsesWireTransport func(*http.Request, int64) (*http.Response, error)

// newEmptyGenericSelectionFixture 使用默认预算构造执行入口，提供商和窗口来源留空。
func newEmptyGenericSelectionFixture() *selection.Generic {
	return selection.NewGeneric(selection.GenericDependencies{}, selection.DefaultOptions())
}

func TestChatCompletionsRejectsGPTImageModelsBeforeScheduling(t *testing.T) {
	for _, model := range []string{"gpt-image-1", "gpt-image-1.5", "gpt-image-2"} {
		for _, tc := range []struct {
			name string
			call func(*gin.Context)
		}{
			{
				name: "gateway",
				call: newGatewayExecutionHandlerForTest(nil).ChatCompletions,
			},
			{
				name: "openai_gateway",
				call: newOpenAIImageChatRejectionHandler(t).ChatCompletions,
			},
		} {
			t.Run(tc.name+"/"+model, func(t *testing.T) {
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				body := []byte(`{"model":"` + model + `","messages":[{"role":"user","content":"draw"}]}`)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
				setImageChatTestAuth(c)

				tc.call(c)

				require.Equal(t, http.StatusBadRequest, recorder.Code)
				require.Equal(t, "invalid_request_error", gjson.Get(recorder.Body.String(), "error.type").String())
				require.Contains(t, gjson.Get(recorder.Body.String(), "error.message").String(), "Chat Completions")
				_, selected := c.Get(gatewayhttp.OpsProviderIDKey)
				require.False(t, selected, "rejection must happen before provider selection")
			})
		}
	}
}

// TestChatCompletionsRejectsGroupMappedImageModel 验证两个 Chat Completions 入口都按分组映射模型 G 校验端点能力。
func TestChatCompletionsRejectsGroupMappedImageModel(t *testing.T) {
	groupID := int64(4349)
	pricingConfigService := newGatewayExecutionPricingConfigServiceForTest(groupID, capability.PlatformOpenAI, routingtestkit.Configuration{
		ID:           4349,
		Status:       billing.StatusActive,
		ModelMapping: map[string]string{"draw-alias": "gpt-image-1"},
	})

	tests := []struct {
		name string
		call func(*gin.Context)
	}{
		{
			name: "gateway",
			call: newGatewayExecutionHandlerWithPricingConfigForTest(nil, pricingConfigService).ChatCompletions,
		},
		{
			name: "openai_gateway",
			call: newOpenAIImageChatRejectionHandlerWithPricingConfig(t, pricingConfigService).ChatCompletions,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			body := []byte(`{"model":"draw-alias","messages":[{"role":"user","content":"draw"}]}`)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			setImageChatTestAuthForGroup(c, groupID)

			tt.call(c)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Contains(t, gjson.Get(recorder.Body.String(), "error.message").String(), "Chat Completions")
			_, selected := c.Get(gatewayhttp.OpsProviderIDKey)
			require.False(t, selected, "分组映射后的端点拒绝必须发生在提供商选择之前")
		})
	}
}

// TestOpenAIChatCompletionsImageModelRejectionDoesNotAcquireConcurrency 确认无效请求不会占用并发额度。
func TestOpenAIChatCompletionsImageModelRejectionDoesNotAcquireConcurrency(t *testing.T) {
	var acquireCalls atomic.Int64
	cache := &httptestkit.ConcurrencyHooks{
		AcquireUserSlotFn: func(context.Context, int64, int, string) (bool, error) {
			acquireCalls.Add(1)
			return true, nil
		},
	}
	h := newOpenAIImageChatRejectionHandlerWithCache(t, cache)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(
		`{"model":"gpt-image-2","messages":[{"role":"user","content":"draw"}]}`,
	))
	setImageChatTestAuth(c)

	h.ChatCompletions(c)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Zero(t, acquireCalls.Load(), "rejection must happen before user/provider concurrency and scheduling")
}

func newOpenAIImageChatRejectionHandler(t *testing.T) *gatewayHTTPEndpointsFixture {
	t.Helper()
	return newOpenAIImageChatRejectionHandlerWithCache(t, &httptestkit.ConcurrencyHooks{})
}

func newOpenAIImageChatRejectionHandlerWithCache(t *testing.T, cache *httptestkit.ConcurrencyHooks) *gatewayHTTPEndpointsFixture {
	t.Helper()
	return newOpenAIImageChatRejectionHandlerWithService(t, cache, &gatewayExecutionFixture{}, newExecutionAvailabilityForTest(

		// newOpenAIImageChatRejectionHandlerWithChannel 构造带分组映射的 OpenAI Chat 测试处理器。
		nil, nil, nil), newEmptyCompatibleSelectionFixture())
}

func setImageChatTestAuth(c *gin.Context) {
	setImageChatTestAuthForGroup(c, 0)
}

// setImageChatTestAuthForGroup 注入带可选分组的图片端点测试身份。
func setImageChatTestAuthForGroup(c *gin.Context, groupID int64) {
	apiKey := &apikey.APIKey{ID: 4348, UserID: 4348, User: &identity.User{ID: 4348}}
	if groupID > 0 {
		apiKey.GroupID = &groupID
		apiKey.Group = &routing.Group{ID: groupID}
	}
	c.Set(string(keyhttp.ContextKeyAPIKey), apiKey)
	c.Set(string(authctx.ContextKeyUser), authctx.AuthSubject{UserID: apiKey.UserID, Concurrency: 1})
}

func (m *mockTempUnscheduler) TempUnscheduleRetryableError(_ context.Context, providerID int64, failoverErr *forwardcore.UpstreamFailoverError) {
	m.calls = append(m.calls, tempUnscheduleCall{providerID: providerID, failoverErr: failoverErr})
}

func newGatewayExecutionHandlerForTest(repo gatewayprovider.ExecutionProviderStore) *messageEndpointsFixture {
	return newGatewayExecutionHandlerWithPricingConfigForTest(repo, nil)
}

func (c *countingGatewaySchedulerCache) GetSnapshot(ctx context.Context, bucket scheduler.SchedulerBucket) ([]scheduler.SnapshotProvider, bool, error) {
	c.snapshotCalls.Add(1)
	return c.fakeSchedulerCache.GetSnapshot(ctx, bucket)
}

func TestGatewayHandlerPreCancelledCompatibleRequestsDoNotSelectProvider(t *testing.T) {
	groupID := int64(9100)
	group := &routing.Group{ID: groupID, Hydrated: true, Status: billing.StatusActive}
	provider := &gatewayprovider.ExecutionProvider{
		Record: providercore.Record{
			LoadLocation: time.LoadLocation, ID: 9101, Platform: capability.PlatformAnthropic, Type: capability.ProviderTypeAPIKey,
			Status: billing.StatusActive, Schedulable: true, Concurrency: 1,
			ProviderGroups: []providercore.GroupMembership{{ProviderID: 9101, GroupID: groupID}},
		},
	}
	schedulerCache := &countingGatewaySchedulerCache{fakeSchedulerCache: &fakeSchedulerCache{providers: []*gatewayprovider.ExecutionProvider{provider}}}
	schedulerSnapshot := scheduler.NewSnapshotService(schedulerCache, nil, nil, nil, nil, scheduler.SnapshotBindings{})
	gatewayService, gatewayServiceChoices, messages := newGenericExecutionAndSelectionFixture(
		nil, &fakeGroupRepo{group: group}, nil, nil, nil,
		schedulerSnapshot, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, responseHeaderFilterForTest(nil),
	)
	gatewayService.Recorder = newHTTPCompletionFixture(nil, nil,
		nil, nil, nil, nil, nil, false)

	cfg := &config.Config{}
	billingCacheService := newBillingEligibilityFixture(cfg)
	billingCacheService.Start()
	t.Cleanup(billingCacheService.Stop)
	h := newMessageEndpointsFixture(gatewayService, messages, newFundingAdmissionFixture(billingCacheService, cfg), gatewayhttp.NewConcurrencyHelper(scheduler.NewConcurrencyService(&fakeConcurrencyCache{}, scheduler.Diagnostics{
		Logf:  logging.LegacyPrintf,
		Event: logging.Event,
	},
	), gatewayhttp.SSEPingFormatClaude, 0), gatewayhttp.MessagesHTTPOptions{MaxBodyBytes: openAITextOptions(cfg).MaxBodyBytes, MaxSwitches: 1, MaxGeminiSwitches: 0}, newExecutionAvailabilityForTest(nil,
		nil, nil), gatewayServiceChoices,
	)
	apiKey := &apikey.APIKey{
		ID: 9102, UserID: 9103, GroupID: &groupID, Group: group, Status: billing.StatusActive,
		User: &identity.User{ID: 9103, Concurrency: 10, Balance: 100},
	}

	tests := []struct {
		name string
		path string
		body string
		call func(*gin.Context)
	}{
		{
			name: "responses", path: "/v1/responses", body: `{"model":"claude-test","input":"hello","stream":false}`,
			call: h.Responses,
		},
		{
			name: "chat completions", path: "/v1/chat/completions", body: `{"model":"claude-test","messages":[{"role":"user","content":"hello"}],"stream":false}`,
			call: h.ChatCompletions,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schedulerCache.snapshotCalls.Store(0)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			ctx = requeststate.WithGroup(ctx, group)
			req := httptest.NewRequest(http.MethodPost, tt.path, bytes.NewBufferString(tt.body)).WithContext(ctx)
			req.Header.Set("Content-Type", "application/json")
			c.Request = req
			c.Set(string(keyhttp.ContextKeyAPIKey), apiKey)
			c.Set(string(authctx.ContextKeyUser), authctx.AuthSubject{UserID: apiKey.UserID, Concurrency: 10})

			tt.call(c)

			require.Zero(t, schedulerCache.snapshotCalls.Load(), "a cancelled request must stop before the provider selector")
			_, selected := c.Get(gatewayhttp.OpsProviderIDKey)
			require.False(t, selected)
		})
	}
}

func TestOpenAIBodyLimitFailoverExhausted_ReturnsRedactedJSON413(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))

	newGatewayHTTPEndpoints(gatewayHTTPFixtureInput{}).openAIAttemptSupport().HandleFailoverExhausted(c, bodyLimitFailoverTestError(), false)

	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	var envelope map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	errBody, ok := envelope["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "invalid_request_error", errBody["type"])
	require.Equal(t, "Request payload is too large", errBody["message"])
	require.NotContains(t, rec.Body.String(), "must-not-leak")
}

func TestOpenAIBodyLimitFailoverExhausted_ReturnsRedactedResponsesSSE(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))

	newGatewayHTTPEndpoints(gatewayHTTPFixtureInput{}).openAIAttemptSupport().HandleFailoverExhausted(c, bodyLimitFailoverTestError(), true)

	body := rec.Body.String()
	require.True(t, strings.HasPrefix(body, "event: response.failed\n"))
	require.Contains(t, body, `"code":"invalid_request"`)
	require.Contains(t, body, `"message":"Request payload is too large"`)
	require.NotContains(t, body, "must-not-leak")
}

func TestOpenAIBodyLimitFailoverExhausted_ReturnsRedactedAnthropicError(t *testing.T) {
	t.Run("json", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(nil))

		newGatewayHTTPEndpoints(gatewayHTTPFixtureInput{}).openAIAttemptSupport().HandleAnthropicFailoverExhausted(c, bodyLimitFailoverTestError(), false)

		require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
		var envelope map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
		errBody, ok := envelope["error"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "invalid_request_error", errBody["type"])
		require.Equal(t, "Request payload is too large", errBody["message"])
		require.NotContains(t, rec.Body.String(), "must-not-leak")
	})

	t.Run("sse", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(nil))

		newGatewayHTTPEndpoints(gatewayHTTPFixtureInput{}).openAIAttemptSupport().HandleAnthropicFailoverExhausted(c, bodyLimitFailoverTestError(), true)

		body := rec.Body.String()
		require.True(t, strings.HasPrefix(body, "event: error\n"))
		require.Contains(t, body, `"type":"invalid_request_error"`)
		require.Contains(t, body, `"message":"Request payload is too large"`)
		require.NotContains(t, body, "must-not-leak")
	})
}

func bodyLimitFailoverTestError() *forwardcore.UpstreamFailoverError {
	return &forwardcore.UpstreamFailoverError{
		StatusCode:         http.StatusRequestEntityTooLarge,
		ResponseBody:       []byte(`{"error":{"message":"proxy limit secret=must-not-leak"}}`),
		Scope:              forwardcore.GatewayFailureScopeProvider,
		Reason:             forwardcore.GatewayFailureReason("openai_request_body_too_large"),
		NextProviderAction: forwardcore.NextProviderRetry,
		ClientStatusCode:   http.StatusRequestEntityTooLarge,
		ClientMessage:      "Request payload is too large",
	}
}

func TestOpenAIResponses_CompactUnauthorizedLogsFailed(t *testing.T) {
	logSink, restore := captureHandlerStructuredLog(t)
	defer restore()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", strings.NewReader(`{"model":"gpt-5.3-codex"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.125.0")

	h := newGatewayHTTPEndpoints(gatewayHTTPFixtureInput{})
	h.Responses(c)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.True(t, logSink.ContainsMessageAtLevel("codex.remote_compact.failed", "warn"))
	require.True(t, logSink.ContainsFieldValue("status_code", "401"))
	require.True(t, logSink.ContainsFieldValue("path", "/v1/responses/compact"))
}

func TestResponsesCredentialFailoverLoop(t *testing.T) {
	t.Run("revoked provider selects healthy provider", func(t *testing.T) {
		h, repo, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "revoked")
		defer cleanup()
		_ = h

		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewBufferString(`{"model":"grok","input":"hello","stream":false}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		require.Contains(t, recorder.Body.String(), "resp_healthy")
		require.Equal(t, []int64{801}, repo.errorIDs())
		require.Equal(t, []int64{802}, upstream.providerHits())
		requestURLs, authorization := upstream.requests()
		require.Equal(t, []string{xai.DefaultCLIBaseURL + "/responses"}, requestURLs)
		require.Equal(t, []string{"Bearer healthy-access"}, authorization)
	})

	t.Run("provider configuration stops before healthy provider", func(t *testing.T) {
		h, repo, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "provider")
		defer cleanup()

		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewBufferString(`{"model":"grok","input":"hello","stream":false}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
		require.Contains(t, recorder.Body.String(), forwardcore.GrokCredentialUnavailableClientMessage)
		require.Empty(t, repo.errorIDs())
		require.Empty(t, upstream.providerHits())
		require.Equal(t, 1, repo.selectorCalls())
		require.Zero(t, h.Input.Choices.SnapshotOpenAIProviderSchedulerMetrics().RuntimeStatsProviderCount,
			"provider-scoped auth failure must not penalize the selected provider")
	})

	t.Run("parent cancellation stops before healthy provider", func(t *testing.T) {
		_, repo, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "cancel")
		defer cleanup()

		ctx, cancel := context.WithCancel(context.Background())
		req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewBufferString(`{"model":"grok","input":"hello","stream":false}`)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		done := make(chan struct{})
		go func() {
			defer close(done)
			router.ServeHTTP(recorder, req)
		}()

		select {
		case <-time.After(2 * time.Second):
			t.Fatal("credential refresh did not start")
		case <-findHandlerRefresherStarted(router):
			cancel()
		}
		select {
		case <-time.After(2 * time.Second):
			t.Fatal("handler did not stop after cancellation")
		case <-done:
		}

		require.Empty(t, repo.errorIDs())
		require.Empty(t, upstream.providerHits())
	})

	t.Run("post-mapping cancellation stops before scheduler mutation or reselection", func(t *testing.T) {
		h, repo, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "postmap_cancel")
		defer cleanup()
		ctx, cancel := context.WithCancel(context.Background())
		upstream.mu.Lock()
		upstream.failProviderID = 801
		upstream.cancelRequest = cancel
		upstream.mu.Unlock()

		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewBufferString(`{"model":"grok","input":"hello","stream":false}`)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, req)

		require.Equal(t, []int64{801}, upstream.providerHits())
		require.Empty(t, repo.errorIDs())
		require.Equal(t, 1, repo.selectorCalls())
		require.Zero(t, h.Input.Choices.SnapshotOpenAIProviderSchedulerMetrics().RuntimeStatsProviderCount)
	})

	t.Run("pre-cancelled request never invokes a provider selector", func(t *testing.T) {
		tests := []struct {
			name   string
			method string
			path   string
			body   string
		}{
			{name: "responses", method: http.MethodPost, path: "/openai/v1/responses", body: `{"model":"grok","input":"hello","stream":false}`},
			{name: "messages", method: http.MethodPost, path: "/openai/v1/messages", body: `{"model":"grok","max_tokens":16,"messages":[{"role":"user","content":"hello"}]}`},
			{name: "chat completions", method: http.MethodPost, path: "/openai/v1/chat/completions", body: `{"model":"grok","messages":[{"role":"user","content":"hello"}],"stream":false}`},
			{name: "grok media", method: http.MethodGet, path: "/openai/v1/videos/request-1"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, repo, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "revoked")
				defer cleanup()
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				recorder := httptest.NewRecorder()
				req := httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body)).WithContext(ctx)
				req.Header.Set("Content-Type", "application/json")

				router.ServeHTTP(recorder, req)

				require.Zero(t, repo.selectorCalls())
				require.Empty(t, upstream.providerHits())
			})
		}
	})

	t.Run("credential state mutation failures stop before reselection", func(t *testing.T) {
		for _, mode := range []string{"mutation_set_error", "mutation_temp", "mutation_cache"} {
			t.Run(mode, func(t *testing.T) {
				_, repo, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, mode)
				defer cleanup()

				recorder := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewBufferString(`{"model":"grok","input":"hello","stream":false}`))
				req.Header.Set("Content-Type", "application/json")
				router.ServeHTTP(recorder, req)

				require.Equal(t, http.StatusServiceUnavailable, recorder.Code, recorder.Body.String())
				require.Contains(t, recorder.Body.String(), forwardcore.GrokCredentialUnavailableClientMessage)
				require.Empty(t, upstream.providerHits())
				require.Equal(t, 1, repo.selectorCalls())
			})
		}
	})

	t.Run("missing credential provider stops before upstream or reselection", func(t *testing.T) {
		_, repo, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "nil_provider")
		defer cleanup()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewBufferString(`{"model":"grok","input":"hello","stream":false}`))
		req.Header.Set("Content-Type", "application/json")

		router.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusServiceUnavailable, recorder.Code, recorder.Body.String())
		require.Contains(t, recorder.Body.String(), forwardcore.GrokCredentialUnavailableClientMessage)
		require.Equal(t, 1, repo.selectorCalls())
		require.Empty(t, upstream.providerHits())
		require.Empty(t, repo.errorIDs())
	})
}

func TestResponsesGrok429FailoverIsBounded(t *testing.T) {
	t.Run("first rate limited provider selects healthy provider", func(t *testing.T) {
		_, repo, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "first_429")
		defer cleanup()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewBufferString(`{"model":"grok","input":"hello","stream":false}`))
		req.Header.Set("Content-Type", "application/json")

		router.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		require.Contains(t, recorder.Body.String(), "resp_healthy")
		require.Equal(t, []int64{801, 802}, upstream.providerHits())
		require.Equal(t, []int64{801}, repo.rateLimitedProviderIDs())
	})

	t.Run("two rate limited providers stop without sweeping the pool", func(t *testing.T) {
		_, repo, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "all_429")
		defer cleanup()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewBufferString(`{"model":"grok","input":"hello","stream":false}`))
		req.Header.Set("Content-Type", "application/json")

		router.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusTooManyRequests, recorder.Code, recorder.Body.String())
		require.Equal(t, []int64{801, 802}, upstream.providerHits())
		require.Equal(t, []int64{801, 802}, repo.rateLimitedProviderIDs())
		require.NotContains(t, recorder.Body.String(), "expired")
		require.NotContains(t, recorder.Body.String(), "healthy-access")
		require.NotContains(t, recorder.Body.String(), "rate limited")
	})
}

func TestResponsesGrok402FailoverCooldown(t *testing.T) {
	_, repo, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "first_402")
	defer cleanup()

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewBufferString(`{"model":"grok","input":"hello","stream":false}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "resp_healthy")
	require.Equal(t, []int64{801, 802}, upstream.providerHits())
	require.Equal(t, []int64{801}, repo.setTempIDs)
	before := repo.selectorCalls()

	second := httptest.NewRecorder()
	secondReq := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewBufferString(`{"model":"grok","input":"again","stream":false}`))
	secondReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(second, secondReq)

	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	require.Equal(t, before+1, repo.selectorCalls())
	require.Equal(t, []int64{801, 802, 802}, upstream.providerHits(), "cooldown must exclude the 402 provider from later requests")
}

func TestResponsesGrok429FailoverHandlesMixedStatuses(t *testing.T) {
	t.Run("429 then 500 stops after the bounded followup", func(t *testing.T) {
		_, _, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "mixed_429_500")
		defer cleanup()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewBufferString(`{"model":"grok","input":"hello","stream":false}`))
		req.Header.Set("Content-Type", "application/json")

		router.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusBadGateway, recorder.Code, recorder.Body.String())
		require.Equal(t, []int64{801, 802}, upstream.providerHits())
		require.NotContains(t, recorder.Body.String(), "upstream unavailable")
	})

	t.Run("500 then 429 permits one healthy followup", func(t *testing.T) {
		_, _, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "mixed_500_429")
		defer cleanup()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewBufferString(`{"model":"grok","input":"hello","stream":false}`))
		req.Header.Set("Content-Type", "application/json")

		router.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		require.Equal(t, []int64{801, 802, 803}, upstream.providerHits())
	})

	t.Run("OAuth 429 then API-key failure cannot bypass the bound", func(t *testing.T) {
		_, _, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "oauth_429_apikey_500")
		defer cleanup()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewBufferString(`{"model":"grok","input":"hello","stream":false}`))
		req.Header.Set("Content-Type", "application/json")

		router.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusBadGateway, recorder.Code, recorder.Body.String())
		require.Equal(t, []int64{801, 802}, upstream.providerHits())
	})
}

func TestGrokMedia429FailoverIsBounded(t *testing.T) {
	t.Run("first 429 selects one healthy followup", func(t *testing.T) {
		_, _, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "first_429")
		defer cleanup()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/openai/v1/videos/generations", bytes.NewBufferString(`{"model":"grok-imagine-video","prompt":"waves"}`))
		req.Header.Set("Content-Type", "application/json")

		router.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		require.Equal(t, []int64{801, 802}, upstream.providerHits())
	})

	t.Run("second 429 stops without sweeping a third provider", func(t *testing.T) {
		_, _, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "all_429")
		defer cleanup()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/openai/v1/videos/generations", bytes.NewBufferString(`{"model":"grok-imagine-video","prompt":"waves"}`))
		req.Header.Set("Content-Type", "application/json")

		router.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusTooManyRequests, recorder.Code, recorder.Body.String())
		require.Equal(t, []int64{801, 802}, upstream.providerHits())
		require.NotContains(t, recorder.Body.String(), "rate limited")
	})
}

func TestGrokOAuthCredentialFailoverAcrossHTTPHandlers(t *testing.T) {
	endpoints := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "messages", method: http.MethodPost, path: "/openai/v1/messages", body: `{"model":"grok","max_tokens":16,"messages":[{"role":"user","content":"hello"}]}`},
		{name: "chat completions", method: http.MethodPost, path: "/openai/v1/chat/completions", body: `{"model":"grok","messages":[{"role":"user","content":"hello"}],"stream":false}`},
		{name: "chat completions raw fallback", method: http.MethodPost, path: "/openai/v1/chat/completions", body: `{"model":"grok","messages":[{"role":"user","content":"hello"}],"stop":["END"],"stream":false}`},
		{name: "grok media", method: http.MethodPost, path: "/openai/v1/videos/generations", body: `{"model":"grok-imagine-video","prompt":"waves"}`},
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint.name+" revoked selects healthy", func(t *testing.T) {
			_, repo, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "revoked")
			defer cleanup()
			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(endpoint.method, endpoint.path, bytes.NewBufferString(endpoint.body))
			req.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(recorder, req)

			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			require.Equal(t, []int64{801}, repo.errorIDs())
			require.Equal(t, []int64{802}, upstream.providerHits())
		})

		t.Run(endpoint.name+" all providers exhausted safely", func(t *testing.T) {
			_, repo, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "all_revoked")
			defer cleanup()
			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(endpoint.method, endpoint.path, bytes.NewBufferString(endpoint.body))
			req.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(recorder, req)

			require.Equal(t, http.StatusServiceUnavailable, recorder.Code, recorder.Body.String())
			require.Contains(t, recorder.Body.String(), forwardcore.GrokCredentialUnavailableClientMessage)
			require.NotContains(t, recorder.Body.String(), "revoked-refresh")
			require.NotContains(t, recorder.Body.String(), "healthy-refresh")
			require.Equal(t, []int64{801, 802}, repo.errorIDs())
			require.Empty(t, upstream.providerHits())
		})
	}
}

func TestGrokOAuthMissingSelectedRowRetriesHealthyProviderWithoutMutation(t *testing.T) {
	_, repo, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "missing_row")
	defer cleanup()
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewBufferString(`{"model":"grok","input":"hello","stream":false}`))
	req.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, []int64{802}, upstream.providerHits())
	require.Empty(t, repo.errorIDs())
	require.Empty(t, repo.setTempIDs)
}

func TestGatewayChatCredentialStopDoesNotSelectAnotherProviderAndReturnsSafe503(t *testing.T) {
	stopErr := &forwardcore.UpstreamFailoverError{
		Stage:              forwardcore.GatewayFailureStageProviderAuth,
		Scope:              forwardcore.GatewayFailureScopeShared,
		Reason:             forwardcore.GrokCredentialReasonProviderConfig,
		NextProviderAction: forwardcore.NextProviderStop,
		ClientStatusCode:   http.StatusTeapot,
		ClientMessage:      "invalid_client client_secret=must-not-leak",
	}
	state := failover.NewFailoverState[*forwardcore.UpstreamFailoverError](3, false, gatewaytelemetry.Failover)
	action := state.HandleFailoverError(context.Background(), &mockTempUnscheduler{}, 71, capability.PlatformGrok, 0, stopErr)

	require.Equal(t, failover.FailoverExhausted, action)
	require.Zero(t, state.SwitchCount)
	require.Empty(t, state.FailedProviderIDs)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	(gatewayhttp.MessagesErrorOutput{}).ChatExhausted(c, state.LastFailoverErr, false)

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Contains(t, recorder.Body.String(), forwardcore.GrokCredentialUnavailableClientMessage)
	require.NotContains(t, recorder.Body.String(), "invalid_client")
	require.NotContains(t, recorder.Body.String(), "client_secret")
}

func TestGatewayChatAntigravityCredentialFailureReturnsActionableMessage(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	(gatewayhttp.MessagesErrorOutput{}).ChatExhausted(c, &forwardcore.UpstreamFailoverError{
		StatusCode:         http.StatusUnauthorized,
		Stage:              forwardcore.GatewayFailureStageProviderAuth,
		Scope:              forwardcore.GatewayFailureScopeProvider,
		Reason:             forwardcore.AntigravityCredentialRejectedReason,
		NextProviderAction: forwardcore.NextProviderRetry,
		ClientStatusCode:   http.StatusBadGateway,
		ClientMessage:      forwardcore.AntigravityCredentialRejectedClientMessage,
		ResponseBody:       []byte(`{"error":{"message":"Invalid bearer token","refresh_token":"must-not-leak"}}`),
	}, false)

	require.Equal(t, http.StatusBadGateway, recorder.Code)
	require.Contains(t, recorder.Body.String(), forwardcore.AntigravityCredentialRejectedClientMessage)
	require.NotContains(t, strings.ToLower(recorder.Body.String()), "bearer")
	require.NotContains(t, strings.ToLower(recorder.Body.String()), "refresh_token")
}

func TestOpenAIAccessStateCredentialFailureUsesTypedSafeResponse(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	newGatewayHTTPEndpoints(gatewayHTTPFixtureInput{}).openAIAttemptSupport().HandleFailoverExhausted(c, &forwardcore.UpstreamFailoverError{
		StatusCode:         http.StatusForbidden,
		Stage:              forwardcore.GatewayFailureStageProviderAuth,
		Scope:              forwardcore.GatewayFailureScopeProvider,
		Reason:             forwardcore.OpenAIUpstreamAccessStateReason,
		NextProviderAction: forwardcore.NextProviderRetry,
		ClientStatusCode:   http.StatusBadGateway,
		ClientMessage:      "Upstream access is temporarily unavailable, please retry later",
		ResponseBody:       []byte(`{"error":{"message":"Your workspace is deactivated","token":"must-not-leak"}}`),
	}, false)

	require.Equal(t, http.StatusBadGateway, recorder.Code)
	require.Contains(t, recorder.Body.String(), "Upstream access is temporarily unavailable")
	require.NotContains(t, strings.ToLower(recorder.Body.String()), "deactivated")
	require.NotContains(t, recorder.Body.String(), "must-not-leak")
}

func TestOpenAICapacityFailoverExhaustionPreservesMessageAsServerError(t *testing.T) {
	message := "Our servers are currently overloaded. Please try again later."
	failoverErr := &forwardcore.UpstreamFailoverError{
		StatusCode:              http.StatusBadRequest,
		ResponseBody:            []byte(`{"error":{"code":"server_is_overloaded","message":"` + message + `"}}`),
		RetryableOnSameProvider: true,
		RequestScopedTransient:  true,
		ClientStatusCode:        http.StatusServiceUnavailable,
		ClientMessage:           message,
	}

	t.Run("native_openai", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		newGatewayHTTPEndpoints(gatewayHTTPFixtureInput{}).openAIAttemptSupport().HandleFailoverExhausted(c, failoverErr, false)
		require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
		require.Equal(t, "server_error", gjson.Get(recorder.Body.String(), "error.type").String())
		require.Equal(t, message, gjson.Get(recorder.Body.String(), "error.message").String())
		require.NotContains(t, recorder.Body.String(), "server_is_overloaded")
	})

	t.Run("responses_compat", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		(gatewayhttp.MessagesErrorOutput{}).ResponsesExhausted(c, failoverErr, false)
		require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
		require.Equal(t, "server_error", gjson.Get(recorder.Body.String(), "error.code").String())
		require.Equal(t, message, gjson.Get(recorder.Body.String(), "error.message").String())
	})

	t.Run("anthropic_compat", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		newGatewayHTTPEndpoints(gatewayHTTPFixtureInput{}).openAIAttemptSupport().HandleAnthropicFailoverExhausted(c, failoverErr, false)
		require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
		require.Equal(t, "api_error", gjson.Get(recorder.Body.String(), "error.type").String())
		require.Equal(t, message, gjson.Get(recorder.Body.String(), "error.message").String())
	})
}

func TestResponsesFailoverExhaustedAfterForwardedTerminalMarksOpsWithoutDuplicateFrame(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	official := "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":{\"code\":\"server_error\",\"message\":\"official failure\"}}}\n\n"
	_, err := c.Writer.Write([]byte(official))
	require.NoError(t, err)
	gatewayhttp.MarkOpsStreamError(c, "server_error", "official failure", http.StatusBadGateway)

	(gatewayhttp.MessagesErrorOutput{}).ResponsesExhausted(c, &forwardcore.UpstreamFailoverError{
		StatusCode:   http.StatusBadGateway,
		ResponseBody: []byte(`{"error":{"message":"fallback failure"}}`),
	}, true)

	require.Equal(t, official, recorder.Body.String())
	streamErr, ok := gatewayhttp.GetOpsStreamError(c)
	require.True(t, ok)
	require.Equal(t, "official failure", streamErr.Message)

	markerRecorder := httptest.NewRecorder()
	markerContext, _ := gin.CreateTestContext(markerRecorder)
	(gatewayhttp.MessagesErrorOutput{}).ResponsesExhausted(markerContext, &forwardcore.UpstreamFailoverError{
		StatusCode: http.StatusTooManyRequests,
	}, true)
	require.Contains(t, markerRecorder.Body.String(), "event: response.failed")
	require.Equal(t, 1, strings.Count(markerRecorder.Body.String(), "event: response.failed"))
	streamErr, ok = gatewayhttp.GetOpsStreamError(markerContext)
	require.True(t, ok)
	require.Equal(t, http.StatusTooManyRequests, streamErr.IntendedStatus)
	require.Equal(t, "rate_limit_error", streamErr.ErrType)

	heartbeatRecorder := httptest.NewRecorder()
	heartbeatContext, _ := gin.CreateTestContext(heartbeatRecorder)
	heartbeat := ": keepalive\n\n"
	written, err := heartbeatRecorder.Write([]byte(heartbeat))
	require.NoError(t, err)
	gatewayhttp.RecordStreamHeartbeat(heartbeatContext, written)
	(gatewayhttp.MessagesErrorOutput{}).ResponsesExhausted(heartbeatContext, &forwardcore.UpstreamFailoverError{
		StatusCode: http.StatusBadGateway,
	}, true)
	require.True(t, strings.HasPrefix(heartbeatRecorder.Body.String(), heartbeat))
	require.Equal(t, 1, strings.Count(heartbeatRecorder.Body.String(), "event: response.failed"))
}

func TestGatewayChatInferenceExhaustionRestoresRetryAfter(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	(gatewayhttp.MessagesErrorOutput{}).ChatExhausted(c, &forwardcore.UpstreamFailoverError{
		StatusCode:      http.StatusTooManyRequests,
		ResponseHeaders: http.Header{"Retry-After": []string{"45"}},
	}, false)

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Equal(t, "45", recorder.Header().Get("Retry-After"))
}

func TestCredentialFailoverExhaustionReturnsFixedSafe503(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	h := newGatewayHTTPEndpoints(gatewayHTTPFixtureInput{})

	h.openAIAttemptSupport().HandleFailoverExhausted(c, &forwardcore.UpstreamFailoverError{
		Stage:              forwardcore.GatewayFailureStageProviderAuth,
		Scope:              forwardcore.GatewayFailureScopeProvider,
		Reason:             forwardcore.GrokCredentialReasonRevoked,
		NextProviderAction: forwardcore.NextProviderRetry,
		ClientStatusCode:   http.StatusTeapot,
		ClientMessage:      "invalid_grant refresh_token=must-not-leak",
	}, false)

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Contains(t, recorder.Body.String(), forwardcore.GrokCredentialUnavailableClientMessage)
	require.NotContains(t, strings.ToLower(recorder.Body.String()), "invalid_grant")
	require.NotContains(t, strings.ToLower(recorder.Body.String()), "refresh_token")
	require.NotContains(t, recorder.Body.String(), "must-not-leak")
}

func TestInferenceFailoverExhaustionRestoresRetryAfter(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	h := newGatewayHTTPEndpoints(gatewayHTTPFixtureInput{})

	h.openAIAttemptSupport().HandleFailoverExhausted(c, &forwardcore.UpstreamFailoverError{
		StatusCode:      http.StatusTooManyRequests,
		ResponseHeaders: http.Header{"Retry-After": []string{"17"}},
	}, false)

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Equal(t, "17", recorder.Header().Get("Retry-After"))
}

func TestFailoverExhaustionRejectsSecretBearingRetryAfter(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	h := newGatewayHTTPEndpoints(gatewayHTTPFixtureInput{})

	h.openAIAttemptSupport().HandleFailoverExhausted(c, &forwardcore.UpstreamFailoverError{
		StatusCode:      http.StatusTooManyRequests,
		ResponseHeaders: http.Header{"Retry-After": []string{"refresh_token=must-not-leak"}},
	}, false)

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Empty(t, recorder.Header().Get("Retry-After"))
	require.NotContains(t, recorder.Body.String(), "must-not-leak")
}

func TestFailoverExhaustionRejectsFarFutureRetryAfterDate(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	h := newGatewayHTTPEndpoints(gatewayHTTPFixtureInput{})

	h.openAIAttemptSupport().HandleFailoverExhausted(c, &forwardcore.UpstreamFailoverError{
		StatusCode: http.StatusTooManyRequests,
		ResponseHeaders: http.Header{
			"Retry-After": []string{time.Now().Add(30 * 24 * time.Hour).UTC().Format(http.TimeFormat)},
		},
	}, false)

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Empty(t, recorder.Header().Get("Retry-After"))
}

func TestFailoverExhaustionAllowsBoundedRetryAfterDate(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	h := newGatewayHTTPEndpoints(gatewayHTTPFixtureInput{})
	retryAfter := time.Now().Add(time.Hour).UTC().Format(http.TimeFormat)

	h.openAIAttemptSupport().HandleFailoverExhausted(c, &forwardcore.UpstreamFailoverError{
		StatusCode:      http.StatusTooManyRequests,
		ResponseHeaders: http.Header{"Retry-After": []string{retryAfter}},
	}, false)

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Equal(t, retryAfter, recorder.Header().Get("Retry-After"))
}

func TestOpsRecoveredCredentialFailoverDoesNotCreateRequestError(t *testing.T) {
	queue := newOpsCaptureQueue(2)

	ops := opscore.NewOpsService(nil, nil, nil, nil, nil, nil, nil, opsprovider.LogControl{})
	router := gin.New()
	router.Use(gatewayhttp.OpsErrorLoggerMiddleware(ops, queue, gatewayhttp.OpsObservationAccess{}))
	router.GET("/openai/v1/responses", func(c *gin.Context) {
		c.Set(gatewayhttp.OpsUpstreamErrorsKey, []*opscore.OpsUpstreamErrorEvent{
			{Stage: string(forwardcore.GatewayFailureStageInference), UpstreamStatusCode: http.StatusForbidden, Message: "earlier inference failure"},
			{
				Stage: string(forwardcore.GatewayFailureStageProviderAuth), Scope: string(forwardcore.GatewayFailureScopeProvider),
				Reason: string(forwardcore.GrokCredentialReasonRevoked), Message: "Grok OAuth credentials require provider action",
			},
		})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/openai/v1/responses", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, int64(1), queue.health.Length)
	job := <-queue.jobs
	require.Equal(t, http.StatusOK, job.entry.StatusCode)
	require.Equal(t, string(forwardcore.GatewayFailureStageProviderAuth), job.entry.ErrorPhase)
	require.NotNil(t, job.entry.UpstreamErrorsJSON)
	events, err := opscore.ParseOpsUpstreamErrors(*job.entry.UpstreamErrorsJSON)
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, string(forwardcore.GatewayFailureStageProviderAuth), events[1].Stage)
}

// TestHandleGroupSelectionBusinessError 验证客户端策略拒绝不会被误报为服务不可用。
func TestHandleGroupSelectionBusinessError(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	var status int
	var errType string
	var message string
	handled := gatewayhttp.WriteGroupSelectionBusinessError(
		c,
		fmt.Errorf("select provider: %w", routing.ErrClaudeCodeOnly),
		false,
		keyhttp.GetAPIKeyFromContext, gatewayprovider.ModelDisplayCatalogue{},
		func(gotStatus int, gotErrType string, gotMessage string, _ bool) {
			status = gotStatus
			errType = gotErrType
			message = gotMessage
		},
	)

	require.True(t, handled)
	require.Equal(t, http.StatusForbidden, status)
	require.Equal(t, "permission_error", errType)
	require.Equal(t, routing.ErrClaudeCodeOnly.Error(), message)
}

func TestOpenAIResponsesRequiredCapability(t *testing.T) {
	tests := []struct {
		name        string
		imageIntent bool
		platform    string
		want        providercore.OpenAIEndpointCapability
	}{
		{
			name:        "OpenAI explicit image intent requires Responses",
			imageIntent: true,
			platform:    capability.PlatformOpenAI,
			want:        providercore.OpenAIEndpointCapabilityResponses,
		},
		{
			name:        "Grok explicit image intent keeps chat capability",
			imageIntent: true,
			platform:    capability.PlatformGrok,
			want:        providercore.OpenAIEndpointCapabilityTextGeneration,
		},
		{
			name:     "non-image intent keeps chat capability",
			platform: capability.PlatformOpenAI,
			want:     providercore.OpenAIEndpointCapabilityTextGeneration,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, textflow.ResponsesCapability(tt.imageIntent, tt.platform))
		})
	}
}

func TestOpenAIEnsureForwardErrorResponse_ResponsesRouteCyberWarningEmitsOriginalMessage(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, gatewayhttp.EndpointResponses, nil)
	_, _ = c.Writer.WriteString(":\n\n")

	message := "This request has been flagged for potentially high-risk cyber activity."
	err := &openAIHandlerTestWarningError{
		warning: &forwardcore.UpstreamWarning{
			StatusCode:   http.StatusForbidden,
			ResponseBody: []byte(`{"type":"response.failed","error":{"message":"` + message + `"}}`),
			Message:      message,
		},
		err: errors.New("upstream response failed"),
	}

	wrote := gatewayhttp.DefaultOpenAIErrorOutput().EnsureResponse(c, false, err)

	require.True(t, wrote)
	body := w.Body.String()
	assert.Contains(t, body, "event: response.failed\n")
	assert.Contains(t, body, `"code":"invalid_request"`)
	assert.Contains(t, body, message)
	assert.NotContains(t, body, "Upstream request failed")
}

func TestOpenAIForwardErrorAlreadyCommunicated(t *testing.T) {
	t.Run("upstream response failed after write", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, gatewayhttp.EndpointResponses, nil)
		before := c.Writer.Size()
		_, _ = c.Writer.WriteString(`event: response.failed
data: {"type":"response.failed","error":{"message":"This content was flagged"}}

`)

		reported := gatewayhttp.OpenAIForwardErrorAlreadyCommunicated(c, before, errors.New("upstream response failed: This content was flagged"))

		require.True(t, reported)
	})

	t.Run("no write still needs fallback", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, gatewayhttp.EndpointResponses, nil)

		reported := gatewayhttp.OpenAIForwardErrorAlreadyCommunicated(c, c.Writer.Size(), errors.New("upstream response failed: This content was flagged"))

		require.False(t, reported)
	})

	t.Run("generic error after write still needs fallback", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, gatewayhttp.EndpointResponses, nil)
		before := c.Writer.Size()
		_, _ = c.Writer.WriteString(":\n\n")

		reported := gatewayhttp.OpenAIForwardErrorAlreadyCommunicated(c, before, errors.New("stream read error: unexpected EOF"))

		require.False(t, reported)
	})
}

func TestOpenAIHandleFailoverExhausted_CyberWarningPassesThroughMessage(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	gatewayhttp.SetOpsRequestContext(c, "gpt-5.4", false)

	message := "This content was flagged for possible cybersecurity risk. If this seems wrong, try rephrasing your request."
	h := newGatewayHTTPEndpoints(gatewayHTTPFixtureInput{})
	h.openAIAttemptSupport().HandleFailoverExhausted(c, &forwardcore.UpstreamFailoverError{
		StatusCode:   http.StatusForbidden,
		ResponseBody: []byte(`{"error":{"message":"` + message + `"}}`),
	}, false)

	require.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), message)
	assert.NotContains(t, w.Body.String(), "All available providers exhausted")
}

func TestShouldLogOpenAIForwardFailureAsWarn(t *testing.T) {
	t.Run("fallback_written_should_not_downgrade", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		require.False(t, gatewayhttp.ShouldLogOpenAIForwardFailureAsWarn(c, true))
	})

	t.Run("context_nil_should_not_downgrade", func(t *testing.T) {
		require.False(t, gatewayhttp.ShouldLogOpenAIForwardFailureAsWarn(nil, false))
	})

	t.Run("response_not_written_should_not_downgrade", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		require.False(t, gatewayhttp.ShouldLogOpenAIForwardFailureAsWarn(c, false))
	})

	t.Run("response_already_written_should_downgrade", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c.String(http.StatusForbidden, "already written")
		require.True(t, gatewayhttp.ShouldLogOpenAIForwardFailureAsWarn(c, false))
	})
}

func TestOpenAIResponses_MissingDependencies_ReturnsServiceUnavailable(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5","stream":false}`))
	c.Request.Header.Set("Content-Type", "application/json")

	groupID := int64(2)
	c.Set(string(keyhttp.ContextKeyAPIKey), &apikey.APIKey{
		ID:      10,
		GroupID: &groupID,
	})
	c.Set(string(authctx.ContextKeyUser), authctx.AuthSubject{
		UserID:      1,
		Concurrency: 1,
	})

	// 使用未初始化依赖，检查请求返回依赖错误。
	h := newGatewayHTTPEndpoints(gatewayHTTPFixtureInput{})
	require.NotPanics(t, func() {
		h.Responses(c)
	})

	require.Equal(t, http.StatusServiceUnavailable, w.Code)

	var parsed map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &parsed)
	require.NoError(t, err)

	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "api_error", errorObj["type"])
	assert.Equal(t, "Service temporarily unavailable", errorObj["message"])
}

func TestOpenAIResponses_SetsClientTransportHTTP(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(`{"model":"gpt-5"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h := newGatewayHTTPEndpoints(gatewayHTTPFixtureInput{})
	h.Responses(c)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Equal(t, gatewayhttp.OpenAIClientTransportHTTP, gatewayhttp.GetOpenAIClientTransport(c))
}

func TestOpenAIResponses_RejectsMessageIDAsPreviousResponseID(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(
		`{"model":"gpt-5.4","stream":false,"previous_response_id":"msg_123456","input":[{"type":"input_text","text":"hello"}]}`,
	))
	c.Request.Header.Set("Content-Type", "application/json")

	groupID := int64(2)
	c.Set(string(keyhttp.ContextKeyAPIKey), &apikey.APIKey{
		ID:      101,
		GroupID: &groupID,
		User:    &identity.User{ID: 1},
	})
	c.Set(string(authctx.ContextKeyUser), authctx.AuthSubject{
		UserID:      1,
		Concurrency: 1,
	})

	h := newOpenAIHandlerForPreviousResponseIDValidation(t, nil)
	h.Responses(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "previous_response_id must be a response.id")
}

func TestOpenAIResponses_AcceptsHTTPContinuationPreviousResponseIDBeforeRouting(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(
		`{"model":"gpt-5.4","stream":false,"previous_response_id":"resp_123456","input":[{"type":"input_text","text":"hello"}]}`,
	))
	c.Request.Header.Set("Content-Type", "application/json")

	groupID := int64(2)
	c.Set(string(keyhttp.ContextKeyAPIKey), &apikey.APIKey{
		ID:      101,
		GroupID: &groupID,
		User:    &identity.User{ID: 1},
	})
	c.Set(string(authctx.ContextKeyUser), authctx.AuthSubject{
		UserID:      1,
		Concurrency: 1,
	})

	h := newOpenAIHandlerForPreviousResponseIDValidation(t, nil)
	require.NoError(t, h.Input.Source.Responses.Lineage.Store.BindHTTPResponseOwner(context.Background(), groupID, "resp_123456", 1, 101, h.Input.Source.WebSockets.OpenAIHTTPResponseStickyTTL()))
	h.Responses(c)

	require.NotEqual(t, http.StatusBadRequest, w.Code)
	require.NotContains(t, w.Body.String(), "Responses WebSocket v2")
}

func TestOpenAIResponses_RejectsHTTPContinuationOwnedByAnotherUser(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(
		`{"model":"gpt-5.4","stream":false,"previous_response_id":"resp_other_tenant","input":"hello"}`,
	))
	c.Request.Header.Set("Content-Type", "application/json")

	groupID := int64(2)
	c.Set(string(keyhttp.ContextKeyAPIKey), &apikey.APIKey{
		ID:      202,
		UserID:  2,
		GroupID: &groupID,
		User:    &identity.User{ID: 2},
	})
	c.Set(string(authctx.ContextKeyUser), authctx.AuthSubject{UserID: 2, Concurrency: 1})

	h := newOpenAIHandlerForPreviousResponseIDValidation(t, nil)
	require.NoError(t, h.Input.Source.Responses.Lineage.Store.BindHTTPResponseOwner(context.Background(), groupID, "resp_other_tenant", 1, 101, h.Input.Source.WebSockets.OpenAIHTTPResponseStickyTTL()))
	h.Responses(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "previous_response_id is not available for this user")
}

func TestOpenAIResponses_RejectsUnownedHTTPContinuation(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(
		`{"model":"gpt-5.4","stream":false,"previous_response_id":"resp_unknown","input":"hello"}`,
	))
	c.Request.Header.Set("Content-Type", "application/json")

	groupID := int64(2)
	c.Set(string(keyhttp.ContextKeyAPIKey), &apikey.APIKey{ID: 101, UserID: 1, GroupID: &groupID, User: &identity.User{ID: 1}})
	c.Set(string(authctx.ContextKeyUser), authctx.AuthSubject{UserID: 1, Concurrency: 1})

	h := newOpenAIHandlerForPreviousResponseIDValidation(t, nil)
	h.Responses(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "previous_response_id is not available for this user")
}

func TestOpenAIResponses_ReportsMissingFunctionCallOutputCallID(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(
		`{"model":"gpt-5.4","stream":false,"input":[{"type":"function_call_output","output":"{}"}]}`,
	))
	c.Request.Header.Set("Content-Type", "application/json")

	groupID := int64(2)
	c.Set(string(keyhttp.ContextKeyAPIKey), &apikey.APIKey{
		ID:      101,
		GroupID: &groupID,
		User:    &identity.User{ID: 1},
	})
	c.Set(string(authctx.ContextKeyUser), authctx.AuthSubject{
		UserID:      1,
		Concurrency: 1,
	})

	h := newOpenAIHandlerForPreviousResponseIDValidation(t, nil)
	h.Responses(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.JSONEq(t, `{"error":{"message":"input[0] (function_call_output) requires call_id","type":"invalid_request_error"}}`, w.Body.String())
}

func (s *cyberSessionBlockHandlerCacheStub) GetSessionProviderID(context.Context, int64, string) (int64, error) {
	return 0, errors.New("not found")
}

func (s *cyberSessionBlockHandlerCacheStub) SetSessionProviderID(context.Context, int64, string, int64, time.Duration) error {
	return nil
}

func (s *cyberSessionBlockHandlerCacheStub) RefreshSessionTTL(context.Context, int64, string, time.Duration) error {
	return nil
}

func (s *cyberSessionBlockHandlerCacheStub) DeleteSessionProviderID(context.Context, int64, string) error {
	return nil
}

func (s *cyberSessionBlockHandlerCacheStub) SetSessionOwnerGroupID(context.Context, int64, string, string, int64, time.Duration) (bool, error) {
	return true, nil
}

func (s *cyberSessionBlockHandlerCacheStub) GetSessionOwnerGroupID(context.Context, int64, string, string) (int64, error) {
	return 0, errors.New("not found")
}

func (s *cyberSessionBlockHandlerCacheStub) RefreshSessionOwnerTTL(context.Context, int64, string, string, time.Duration) error {
	return nil
}

func (s *cyberSessionBlockHandlerCacheStub) SetCyberSessionBlocked(context.Context, string, time.Duration) error {
	s.blocked = true
	return nil
}

func (s *cyberSessionBlockHandlerCacheStub) IsCyberSessionBlocked(context.Context, string) (bool, error) {
	s.readCalls++
	return s.blocked, nil
}

func TestOpenAIRecordCyberWarning_RecordsStructuredResponseBody(t *testing.T) {
	cfg := moderation.ContentModerationConfig{
		CyberWarningEnabled: true,
		CyberWindowHours:    720,
		AllGroups:           true,
	}
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)
	repo := &contentModerationHandlerTestRepo{}
	settingRepo := &contentModerationHandlerSettingRepo{values: map[string]string{
		moderation.SettingKeyRiskControlEnabled:      "true",
		moderation.SettingKeyContentModerationConfig: string(rawCfg),
	}}
	moderationSvc := newHTTPModeration(t, settingRepo, repo)
	moderationSvc.Start()
	h := newGatewayHTTPEndpoints(gatewayHTTPFixtureInput{Moderator: moderationSvc})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	gatewayhttp.SetOpenAICyberWarningRequestSnapshot(c, moderation.ContentModerationProtocolOpenAIResponses, []byte(`{"model":"gpt-5.4","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"bad cyber prompt"}]}]}`))
	apiKey := &apikey.APIKey{
		ID:     101,
		Name:   "test-key",
		UserID: 1001,
		User:   &identity.User{ID: 1001, Email: "user@example.com"},
	}
	provider := &gatewayprovider.ExecutionProvider{
		Record: providercore.Record{
			LoadLocation: time.LoadLocation, ID: 2001,
			Name: "openai-1",
		},
	}

	h.openAIAttemptSupport().RecordOpenAICyberWarning(
		c,
		nil,
		apiKey,
		provider,
		"gpt-5.4",
		400,
		[]byte(`{"error":{"message":"This request may pose a cybersecurity risk."}}`),
		"",
	)

	require.Len(t, repo.cyberWarnings, 1)
	warning := repo.cyberWarnings[0]
	require.Equal(t, "user@example.com", warning.UserEmail)
	require.Equal(t, int64(2001), *warning.ProviderID)
	require.Equal(t, "/v1/responses", warning.Endpoint)
	require.Equal(t, "bad cyber prompt", warning.PromptExcerpt)
	require.Contains(t, warning.WarningText, "cybersecurity risk")
}

func TestOpenAIRecordCyberWarning_UsesExplicitPromptExcerpt(t *testing.T) {
	cfg := moderation.ContentModerationConfig{
		CyberWarningEnabled: true,
		CyberWindowHours:    720,
		AllGroups:           true,
	}
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)
	repo := &contentModerationHandlerTestRepo{}
	settingRepo := &contentModerationHandlerSettingRepo{values: map[string]string{
		moderation.SettingKeyRiskControlEnabled:      "true",
		moderation.SettingKeyContentModerationConfig: string(rawCfg),
	}}
	moderationSvc := newHTTPModeration(t, settingRepo, repo)
	moderationSvc.Start()
	h := newGatewayHTTPEndpoints(gatewayHTTPFixtureInput{Moderator: moderationSvc})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	gatewayhttp.SetOpenAICyberWarningPromptExcerpt(c, "second turn prompt")

	apiKey := &apikey.APIKey{ID: 101, Name: "test-key", UserID: 1001, User: &identity.User{ID: 1001, Email: "user@example.com"}}
	provider := &gatewayprovider.ExecutionProvider{Record: providercore.Record{LoadLocation: time.LoadLocation, ID: 2001, Name: "openai-1"}}

	h.openAIAttemptSupport().RecordOpenAICyberWarningWithPromptExcerpt(
		c,
		nil,
		apiKey,
		provider,
		"gpt-5.4",
		400,
		[]byte(`{"error":{"message":"This request may pose a cybersecurity risk."}}`),
		"",
		"first turn prompt",
	)

	require.Len(t, repo.cyberWarnings, 1)
	require.Equal(t, "first turn prompt", repo.cyberWarnings[0].PromptExcerpt)
}

func TestOpenAIRecordCyberWarning_RequestSnapshotUsesCurrentToolOutput(t *testing.T) {
	cfg := moderation.ContentModerationConfig{
		CyberWarningEnabled: true,
		CyberWindowHours:    720,
		AllGroups:           true,
	}
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)
	repo := &contentModerationHandlerTestRepo{}
	settingRepo := &contentModerationHandlerSettingRepo{values: map[string]string{
		moderation.SettingKeyRiskControlEnabled:      "true",
		moderation.SettingKeyContentModerationConfig: string(rawCfg),
	}}
	moderationSvc := newHTTPModeration(t, settingRepo, repo)
	moderationSvc.Start()
	h := newGatewayHTTPEndpoints(gatewayHTTPFixtureInput{Moderator: moderationSvc})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	gatewayhttp.SetOpenAICyberWarningRequestSnapshot(c, moderation.ContentModerationProtocolOpenAIResponses, []byte(`{
		"model":"gpt-5.4",
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"latest cyber prompt"}]},
			{"type":"function_call","call_id":"call_1","name":"run_tests","arguments":"{}"},
			{"type":"function_call_output","call_id":"call_1","output":"done"}
		]
	}`))

	apiKey := &apikey.APIKey{ID: 101, Name: "test-key", UserID: 1001, User: &identity.User{ID: 1001, Email: "user@example.com"}}
	provider := &gatewayprovider.ExecutionProvider{Record: providercore.Record{LoadLocation: time.LoadLocation, ID: 2001, Name: "openai-1"}}

	h.openAIAttemptSupport().RecordOpenAICyberWarning(
		c,
		nil,
		apiKey,
		provider,
		"gpt-5.4",
		http.StatusOK,
		[]byte(`{"type":"response.failed","error":{"message":"This request has been flagged for potentially high-risk cyber activity."}}`),
		"",
	)

	require.Len(t, repo.cyberWarnings, 1)
	require.Equal(t, "done", repo.cyberWarnings[0].PromptExcerpt)
	require.Equal(t, moderation.ContentModerationSourceTool, repo.cyberWarnings[0].Source)
	require.True(t, repo.cyberWarnings[0].ContentComplete)
	require.Len(t, repo.cyberWarnings[0].InputItems, 1)
	require.Equal(t, "done", repo.cyberWarnings[0].InputItems[0].Text)
	require.Equal(t, http.StatusOK, repo.cyberWarnings[0].UpstreamStatus)
}

func TestOpenAIRecordCyberPolicyIfMarked_SkipsSideEffectsOutOfScope(t *testing.T) {
	cfg := moderation.ContentModerationConfig{
		CyberWarningEnabled: true,
		CyberWindowHours:    720,
		AllGroups:           false,
		GroupIDs:            []int64{101},
	}
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)
	repo := &contentModerationHandlerTestRepo{}
	settingRepo := &contentModerationHandlerSettingRepo{values: map[string]string{
		moderation.SettingKeyRiskControlEnabled:      "true",
		moderation.SettingKeyContentModerationConfig: string(rawCfg),
	}}
	moderationSvc := newHTTPModeration(t, settingRepo, repo)
	moderationSvc.Start()
	h := newGatewayHTTPEndpoints(gatewayHTTPFixtureInput{Moderator: moderationSvc})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	gatewayhttp.MarkOpsCyberPolicy(c, moderationflow.Mark{
		Message:        "Request blocked by upstream cyber policy",
		Body:           `{"response":{"error":{"code":"cyber_policy","message":"Request blocked by upstream cyber policy"}}}`,
		UpstreamStatus: http.StatusOK,
	})
	outOfScopeGroupID := int64(202)
	apiKey := &apikey.APIKey{
		ID:      101,
		Name:    "test-key",
		UserID:  1001,
		GroupID: &outOfScopeGroupID,
		User:    &identity.User{ID: 1001, Email: "user@example.com"},
		Group:   &routing.Group{ID: outOfScopeGroupID, Name: "out-of-scope"},
	}
	provider := &gatewayprovider.ExecutionProvider{Record: providercore.Record{LoadLocation: time.LoadLocation, ID: 2001, Name: "openai-1"}}

	handled := h.openAIAttemptSupport().RecordCyberPolicyIfMarked(c, apiKey, provider, nil, "gpt-5.4", true, "cyber-session-key", routing.PricingUsageFields{}, "payload-hash")

	require.False(t, handled)
	require.Empty(t, repo.cyberWarnings)
	require.False(t, c.GetBool(gatewayhttp.CyberPolicyRecordedKey))
}

func TestOpenAIRejectCyberSessionBlocked_OnlyChecksRiskControlGroups(t *testing.T) {
	selectedGroupID := int64(101)
	outOfScopeGroupID := int64(202)
	cfg := moderation.ContentModerationConfig{
		AllGroups: false,
		GroupIDs:  []int64{selectedGroupID},
	}
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)
	settingRepo := &contentModerationHandlerSettingRepo{values: map[string]string{
		moderation.SettingKeyRiskControlEnabled:          "true",
		moderation.SettingKeyContentModerationConfig:     string(rawCfg),
		moderation.SettingKeyCyberSessionBlockEnabled:    "true",
		moderation.SettingKeyCyberSessionBlockTTLSeconds: "3600",
	}}
	cache := &cyberSessionBlockHandlerCacheStub{blocked: true}
	gatewaySvc, gatewaySvcChoices, gatewaySvcCredentialPort := newOpenAIExecutionAndSelectionFixture(
		nil, cache, nil, nil, nil, nil,
		nil, nil, nil, newOpenAIExecutionCredentialsForTest(nil,
			nil), nil, nil, nil, gatewaytestkit.RuntimeReaders(settingRepo), nil, responseHeaderFilterForTest(nil), nil, nil, nil,
	)
	gatewaySvc.Recorder = newHTTPCompletionFixture(nil, nil, nil,
		nil, nil, nil, nil, true)

	moderationSvc := newHTTPModeration(t, settingRepo, nil)
	moderationSvc.Start()
	h := newGatewayHTTPEndpoints(gatewayHTTPFixtureInput{
		Source: gatewaySvc, Credentials: gatewaySvcCredentialPort,
		Moderator: moderationSvc, Availability: newExecutionAvailabilityForTest(nil,

			nil, nil), Choices: gatewaySvcChoices,
	})
	body := []byte(`{"prompt_cache_key":"cyber-scope-session"}`)
	tests := []struct {
		name        string
		groupID     int64
		wantBlocked bool
		wantReads   int
	}{
		{name: "未纳入风控的分组放行", groupID: outOfScopeGroupID, wantBlocked: false, wantReads: 0},
		{name: "已纳入风控的分组保持屏蔽", groupID: selectedGroupID, wantBlocked: true, wantReads: 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cache.readCalls = 0
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
			groupID := tc.groupID
			apiKey := &apikey.APIKey{ID: 1001, GroupID: &groupID}

			blocked := h.rejectIfCyberSessionBlocked(c, apiKey, body, "gpt-5.4", gatewayhttp.CyberBlockResponses)

			require.Equal(t, tc.wantBlocked, blocked)
			require.Equal(t, tc.wantReads, cache.readCalls)
			if tc.wantBlocked {
				require.Equal(t, http.StatusForbidden, w.Code)
				require.Contains(t, w.Body.String(), "session_blocked_by_cyber_policy")
			} else {
				require.Empty(t, w.Body.String())
			}
		})
	}
}

func (u *openAIHTTPPassthroughFailoverUpstream) Do(_ *http.Request, _ string, providerID int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	u.providerIDs = append(u.providerIDs, providerID)
	u.mu.Unlock()
	return &http.Response{
		StatusCode: http.StatusBadGateway,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"temporary upstream failure"}}`)),
	}, nil
}

// DoWithTLS 使故障转移测试桩兼容 fork 的 TLS 指纹上游接口。
func (u *openAIHTTPPassthroughFailoverUpstream) DoWithTLS(req *http.Request, proxyURL string, providerID int64, providerConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, providerID, providerConcurrency)
}

func (u *openAIHTTPPassthroughFailoverUpstream) calls() []int64 {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]int64(nil), u.providerIDs...)
}

func (u *openAIHTTPPassthroughAuthFailoverUpstream) Do(_ *http.Request, _ string, providerID int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	u.providerIDs = append(u.providerIDs, providerID)
	u.mu.Unlock()
	if providerID == 9911 {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_healthy","object":"response","model":"gpt-5.2","status":"completed","usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)),
		}, nil
	}
	return &http.Response{
		StatusCode: u.statusCode,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"upstream credential rejected"}}`)),
	}, nil
}

// DoWithTLS 使认证故障转移测试桩兼容 fork 的 TLS 指纹上游接口。
func (u *openAIHTTPPassthroughAuthFailoverUpstream) DoWithTLS(req *http.Request, proxyURL string, providerID int64, providerConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, providerID, providerConcurrency)
}

func (u *openAIHTTPPassthroughAuthFailoverUpstream) calls() []int64 {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]int64(nil), u.providerIDs...)
}

func (u *openAIHTTPPassthroughSSERateLimitUpstream) Do(_ *http.Request, _ string, providerID int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	u.providerIDs = append(u.providerIDs, providerID)
	u.mu.Unlock()
	body := strings.Join([]string{
		"event: response.created",
		`data: {"type":"response.created","response":{"id":"resp_rate_limited"}}`,
		"",
		"event: response.failed",
		`data: {"type":"response.failed","response":{"id":"resp_rate_limited","status":"failed","error":{"type":"invalid_request_error","code":"rate_limit_exceeded","message":"Concurrency limit exceeded for provider, please retry later"}}}`,
		"",
	}, "\n")
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
			"Retry-After":  []string{"1"},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}, nil
}

// DoWithTLS 使 SSE 限流测试桩兼容 fork 的 TLS 指纹上游接口。
func (u *openAIHTTPPassthroughSSERateLimitUpstream) DoWithTLS(req *http.Request, proxyURL string, providerID int64, providerConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, providerID, providerConcurrency)
}

func (u *openAIHTTPPassthroughSSERateLimitUpstream) calls() []int64 {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]int64(nil), u.providerIDs...)
}

func TestOpenAIResponses_APIKeyPassthroughPool5xxRetriesThenExhaustsMaxSwitches(t *testing.T) {
	groupID := int64(4203)
	providers := []gatewayprovider.ExecutionProvider{
		{
			Record: providercore.Record{
				LoadLocation: time.LoadLocation, ID: 9910, Name: "pool-api-key", Platform: capability.PlatformOpenAI,
				Type: capability.ProviderTypeAPIKey, Status: billing.StatusActive, Schedulable: true, Priority: 1,
				Credentials: map[string]any{
					"api_key":                      "sk-pool",
					"base_url":                     "https://api.example.test",
					"pool_mode":                    true,
					"pool_mode_retry_count":        float64(1),
					"pool_mode_retry_status_codes": []any{float64(http.StatusBadGateway)},
				},
				Extra: map[string]any{"openai_passthrough": true},
			},
		},
		{
			Record: providercore.Record{
				LoadLocation: time.LoadLocation, ID: 9911, Name: "fallback-api-key", Platform: capability.PlatformOpenAI,
				Type: capability.ProviderTypeAPIKey, Status: billing.StatusActive, Schedulable: true, Priority: 2,
				Credentials: map[string]any{
					"api_key":  "sk-fallback",
					"base_url": "https://api.example.test",
				},
				Extra: map[string]any{"openai_passthrough": true},
			},
		},
	}
	cfg := &config.Config{}
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Gateway.MaxProviderSwitches = 1

	providerRepo := &openAIWSFailoverHandlerProviderRepoStub{providers: providers}
	upstream := &openAIHTTPPassthroughFailoverUpstream{}
	billingCacheSvc := newBillingEligibilityFixture(cfg)
	billingCacheSvc.Start()
	t.Cleanup(billingCacheSvc.Stop)
	completionInput4 := billingtestkit.Calculator(nil, nil)
	completionInput5 := &providercore.DeferredService{}
	gatewaySvc, gatewaySvcChoices, gatewaySvcCredentialPort := newOpenAIExecutionAndSelectionFixture(
		providerRepo,
		nil,
		cfg,
		nil,
		nil, nil,

		upstream,
		nil, completionInput5, newOpenAIExecutionCredentialsForTest(providerRepo,

			nil), nil,
		nil,
		nil,

		nil,
		nil, responseHeaderFilterForTest(cfg), nil, nil, nil,
	)
	gatewaySvc.Recorder = newHTTPCompletionFixture(cfg, nil, completionInput4, billingCacheSvc, completionInput5, nil, nil, true)

	h := newGatewayHTTPEndpointsFromDeps(
		gatewaySvc, gatewaySvcCredentialPort, scheduler.NewConcurrencyService(nil, scheduler.Diagnostics{
			Logf: logging.LegacyPrintf,

			Event: logging.Event,
		},
		), newFundingAdmissionFixture(billingCacheSvc, cfg), testkit.NewService(nil, nil, nil, nil, nil, nil, cfg),
		nil,
		nil,
		nil,
		nil,
		cfg, nil, newExecutionAvailabilityForTest(providerRepo,

			nil, cfg), gatewaySvcChoices,
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(`{"model":"gpt-5.2","input":"hello","stream":false}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(keyhttp.ContextKeyAPIKey), &apikey.APIKey{
		ID: 1803, GroupID: &groupID,
		User:  &identity.User{ID: 1703, Status: billing.StatusActive},
		Group: &routing.Group{ID: groupID, Status: billing.StatusActive},
	})
	c.Set(string(authctx.ContextKeyUser), authctx.AuthSubject{UserID: 1703, Concurrency: 0})

	h.Responses(c)

	require.Equal(t, []int64{9910, 9910, 9911}, upstream.calls())
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Equal(t, "upstream_error", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
	require.Equal(t, "Upstream service temporarily unavailable", gjson.GetBytes(rec.Body.Bytes(), "error.message").String())
}

// TestOpenAIResponses_APIKeyPassthroughPoolAuthFailureRetriesThenSwitchesToHealthyProvider 验证认证错误耗尽同号预算后切换提供商。
func TestOpenAIResponses_APIKeyPassthroughPoolAuthFailureRetriesThenSwitchesToHealthyProvider(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{name: "401", statusCode: http.StatusUnauthorized},
		{name: "403", statusCode: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groupID := int64(4203)
			providers := []gatewayprovider.ExecutionProvider{
				{
					Record: providercore.Record{
						LoadLocation: time.LoadLocation, ID: 9910, Name: "pool-api-key", Platform: capability.PlatformOpenAI,
						Type: capability.ProviderTypeAPIKey, Status: billing.StatusActive, Schedulable: true, Priority: 1,
						Credentials: map[string]any{
							"api_key":                      "sk-pool",
							"base_url":                     "https://api.example.test",
							"pool_mode":                    true,
							"pool_mode_retry_count":        float64(1),
							"pool_mode_retry_status_codes": []any{float64(tt.statusCode)},
						},
						Extra: map[string]any{"openai_passthrough": true},
					},
				},
				{
					Record: providercore.Record{
						LoadLocation: time.LoadLocation, ID: 9911, Name: "fallback-api-key", Platform: capability.PlatformOpenAI,
						Type: capability.ProviderTypeAPIKey, Status: billing.StatusActive, Schedulable: true, Priority: 2,
						Credentials: map[string]any{
							"api_key":  "sk-fallback",
							"base_url": "https://api.example.test",
						},
						Extra: map[string]any{"openai_passthrough": true},
					},
				},
			}
			cfg := &config.Config{}
			cfg.Default.RateMultiplier = 1
			cfg.Security.URLAllowlist.Enabled = false
			cfg.Gateway.MaxProviderSwitches = 1

			providerRepo := &openAIWSFailoverHandlerProviderRepoStub{providers: providers}
			upstream := &openAIHTTPPassthroughAuthFailoverUpstream{statusCode: tt.statusCode}
			rateLimitSvc := newAppHealthObserverFixture(providerRepo, cfg)
			billingCacheSvc := newBillingEligibilityFixture(cfg)
			billingCacheSvc.Start()
			t.Cleanup(billingCacheSvc.Stop)
			completionInput6 := billingtestkit.Calculator(nil, nil)
			completionInput7 := &providercore.DeferredService{}
			gatewaySvc, gatewaySvcChoices, gatewaySvcCredentialPort := newOpenAIExecutionAndSelectionFixture(
				providerRepo,
				nil,
				cfg,
				nil,
				nil, rateLimitSvc,

				upstream,
				nil, completionInput7, newOpenAIExecutionCredentialsForTest(providerRepo,

					nil), nil,
				nil,
				nil,

				nil,
				nil, responseHeaderFilterForTest(cfg), nil, nil, nil,
			)
			gatewaySvc.Recorder = newHTTPCompletionFixture(cfg, nil, completionInput6, billingCacheSvc, completionInput7, nil, completionHealth{rateLimitSvc.Core}, true)

			h := newGatewayHTTPEndpointsFromDeps(
				gatewaySvc, gatewaySvcCredentialPort, scheduler.NewConcurrencyService(nil, scheduler.Diagnostics{
					Logf: logging.LegacyPrintf,

					Event: logging.Event,
				},
				), newFundingAdmissionFixture(billingCacheSvc, cfg), testkit.NewService(nil, nil, nil, nil, nil, nil, cfg),
				nil,
				nil,
				nil,
				nil,
				cfg, nil, newExecutionAvailabilityForTest(providerRepo,

					nil, cfg), gatewaySvcChoices,
			)

			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(`{"model":"gpt-5.2","input":"hello","stream":false}`))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set(string(keyhttp.ContextKeyAPIKey), &apikey.APIKey{
				ID: 1803, GroupID: &groupID,
				User:  &identity.User{ID: 1703, Status: billing.StatusActive},
				Group: &routing.Group{ID: groupID, Status: billing.StatusActive},
			})
			c.Set(string(authctx.ContextKeyUser), authctx.AuthSubject{UserID: 1703, Concurrency: 0})

			h.Responses(c)

			require.Equal(t, []int64{9910, 9910, 9911}, upstream.calls())
			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, "resp_healthy", gjson.GetBytes(rec.Body.Bytes(), "id").String())
		})
	}
}

func TestOpenAIResponses_APIKeyPassthroughSSERateLimitUsesConfiguredPoolRetry(t *testing.T) {
	groupID := int64(4204)
	providers := []gatewayprovider.ExecutionProvider{
		{
			Record: providercore.Record{
				LoadLocation: time.LoadLocation, ID: 9912, Name: "pool-sse-rate-limit", Platform: capability.PlatformOpenAI,
				Type: capability.ProviderTypeAPIKey, Status: billing.StatusActive, Schedulable: true, Priority: 1,
				Credentials: map[string]any{
					"api_key":                      "sk-pool",
					"base_url":                     "https://api.example.test",
					"pool_mode":                    true,
					"pool_mode_retry_count":        float64(1),
					"pool_mode_retry_status_codes": []any{float64(http.StatusTooManyRequests)},
				},
				Extra: map[string]any{"openai_passthrough": true},
			},
		},
	}
	cfg := &config.Config{}
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Gateway.MaxProviderSwitches = 1

	providerRepo := &openAIWSFailoverHandlerProviderRepoStub{providers: providers}
	upstream := &openAIHTTPPassthroughSSERateLimitUpstream{}
	billingCacheSvc := newBillingEligibilityFixture(cfg)
	billingCacheSvc.Start()
	t.Cleanup(billingCacheSvc.Stop)
	completionInput8 := billingtestkit.Calculator(nil, nil)
	completionInput9 := &providercore.DeferredService{}
	gatewaySvc, gatewaySvcChoices, gatewaySvcCredentialPort := newOpenAIExecutionAndSelectionFixture(
		providerRepo,
		nil,
		cfg,
		nil,
		nil, nil,

		upstream,
		nil, completionInput9, newOpenAIExecutionCredentialsForTest(providerRepo,

			nil), nil,
		nil,
		nil,

		nil,
		nil, responseHeaderFilterForTest(cfg), nil, nil, nil,
	)
	gatewaySvc.Recorder = newHTTPCompletionFixture(cfg, nil, completionInput8, billingCacheSvc, completionInput9, nil, nil, true)

	h := newGatewayHTTPEndpointsFromDeps(
		gatewaySvc, gatewaySvcCredentialPort, scheduler.NewConcurrencyService(nil, scheduler.Diagnostics{
			Logf: logging.LegacyPrintf,

			Event: logging.Event,
		},
		), newFundingAdmissionFixture(billingCacheSvc, cfg), testkit.NewService(nil, nil, nil, nil, nil, nil, cfg),
		nil,
		nil,
		nil,
		nil,
		cfg, nil, newExecutionAvailabilityForTest(providerRepo,

			nil, cfg), gatewaySvcChoices,
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(`{"model":"gpt-5.2","input":"hello","stream":true}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(keyhttp.ContextKeyAPIKey), &apikey.APIKey{
		ID: 1804, GroupID: &groupID,
		User:  &identity.User{ID: 1704, Status: billing.StatusActive},
		Group: &routing.Group{ID: groupID, Status: billing.StatusActive},
	})
	c.Set(string(authctx.ContextKeyUser), authctx.AuthSubject{UserID: 1704, Concurrency: 0})

	h.Responses(c)

	require.Equal(t, []int64{9912, 9912}, upstream.calls())
	require.Empty(t, providerRepo.rateLimitedIDs)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Equal(t, "1", rec.Header().Get("Retry-After"))
	require.Equal(t, "rate_limit_error", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
	require.Equal(t, "Upstream rate limit exceeded, please retry later", gjson.GetBytes(rec.Body.Bytes(), "error.message").String())
}

// TestOpenAIHTTPResourceBindingSharesImageCapacity 检查共享资源和 HTTP 入口使用同一个图片并发限制器。
func TestOpenAIHTTPResourceBindingSharesImageCapacity(t *testing.T) {
	resources := &gatewayhttp.OpenAIHTTPResources{
		Concurrency:  gatewayhttp.NewConcurrencyHelper(nil, gatewayhttp.SSEPingFormatNone, 0),
		Images:       &scheduler.ImageConcurrencyLimiter{},
		ImageOptions: &gatewayhttp.OpenAIImageAdmissionOptions{Enabled: true, Limit: 1},
	}
	cfg := &config.Config{Gateway: config.GatewayConfig{ImageConcurrency: config.ImageConcurrencyConfig{Enabled: true, MaxConcurrentRequests: 1}}}
	h := newGatewayHTTPEndpointsFromDeps(nil, nil, nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, resources)
	require.Same(t, resources.Images, h.Input.Images)
	require.Same(t, resources.Concurrency, h.Input.Concurrency)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	release, ok := resources.AcquireImage(c, false)
	require.True(t, ok)
	require.NotNil(t, release)
	blocked, ok := h.httpResources().AcquireImage(c, false)
	require.False(t, ok)
	require.Nil(t, blocked)
	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	release()
	release()
	acquired, ok := h.httpResources().AcquireImage(c, false)
	require.True(t, ok)
	require.NotNil(t, acquired)
	acquired()
}

func (r openAIResponsesFailoverProviderRepo) GetByID(_ context.Context, id int64) (*gatewayprovider.ExecutionProvider, error) {
	for i := range r.providers {
		if r.providers[i].Record.ID == id {
			provider := r.providers[i]
			return &provider, nil
		}
	}
	return nil, scheduler.ErrNoAvailableProviders
}

func (r openAIResponsesFailoverProviderRepo) ListSchedulableByGroupIDAndPlatform(_ context.Context, _ int64, platform string) ([]gatewayprovider.ExecutionProvider, error) {
	return r.providersForPlatform(platform), nil
}

func (r openAIResponsesFailoverProviderRepo) ListSchedulableByPlatform(_ context.Context, platform string) ([]gatewayprovider.ExecutionProvider, error) {
	return r.providersForPlatform(platform), nil
}

func (r openAIResponsesFailoverProviderRepo) ListSchedulableUngroupedByPlatform(_ context.Context, platform string) ([]gatewayprovider.ExecutionProvider, error) {
	return r.providersForPlatform(platform), nil
}

func (r openAIResponsesFailoverProviderRepo) providersForPlatform(platform string) []gatewayprovider.ExecutionProvider {
	out := make([]gatewayprovider.ExecutionProvider, 0, len(r.providers))
	for _, provider := range r.providers {
		if platform == "" || provider.Record.Platform == platform {
			out = append(out, provider)
		}
	}
	return out
}

func (u *openAIResponsesFailoverCancelUpstream) Do(_ *http.Request, _ string, providerID int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	u.providerIDs = append(u.providerIDs, providerID)
	first := len(u.providerIDs) == 1
	u.mu.Unlock()
	if first && u.onFirstDo != nil {
		u.onFirstDo()
	}
	return &http.Response{
		StatusCode: 520,
		Header:     http.Header{"Content-Type": []string{"text/html"}},
		Body:       io.NopCloser(bytes.NewBufferString("<html>520: unknown error</html>")),
	}, nil
}

func (u *openAIResponsesFailoverCancelUpstream) DoWithTLS(
	req *http.Request,
	proxyURL string,
	providerID int64,
	providerConcurrency int,
	_ *tlsfingerprint.Profile,
) (*http.Response, error) {
	return u.Do(req, proxyURL, providerID, providerConcurrency)
}

func (u *openAIResponsesFailoverCancelUpstream) calls() []int64 {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]int64(nil), u.providerIDs...)
}

func newOpenAIResponsesFailoverTestHandler(t *testing.T, upstream httpclient.UpstreamTransport, testProviders ...gatewayprovider.ExecutionProvider) *gatewayHTTPEndpointsFixture {
	t.Helper()
	proxyID := int64(11)
	providers := []gatewayprovider.ExecutionProvider{
		{
			Record: providercore.Record{
				LoadLocation: time.LoadLocation, ID: 1,
				Name:        "responses-provider-1",
				Platform:    capability.PlatformOpenAI,
				Type:        capability.ProviderTypeOAuth,
				Status:      billing.StatusActive,
				Schedulable: true,
				Concurrency: 0,
				Priority:    0,
				Credentials: map[string]any{"access_token": "token-1"},
				ProxyID:     &proxyID,
				Proxy: &egress.Proxy{
					ID:       proxyID,
					Name:     "responses-proxy",
					Protocol: "http",
					Host:     "proxy.example.com",
					Port:     8080,
					Username: "proxy-user-secret",
					Password: "proxy-password-secret",
				},
			},
		},
		{
			Record: providercore.Record{
				LoadLocation: time.LoadLocation, ID: 2,
				Name:        "responses-provider-2",
				Platform:    capability.PlatformOpenAI,
				Type:        capability.ProviderTypeOAuth,
				Status:      billing.StatusActive,
				Schedulable: true,
				Concurrency: 0,
				Priority:    1,
				Credentials: map[string]any{"access_token": "token-2"},
			},
		},
	}
	if len(testProviders) > 0 {
		providers = testProviders
	}
	providerRepo := openAIResponsesFailoverProviderRepo{providers: providers}
	cfg := &config.Config{}
	gatewayService, gatewayServiceChoices, gatewayServiceCredentialPort := newOpenAIExecutionAndSelectionFixture(
		providerRepo,
		nil,
		cfg,
		nil,
		nil,

		nil,

		upstream,
		nil,
		nil, newOpenAIExecutionCredentialsForTest(providerRepo,

			nil), nil,
		nil,
		nil,

		nil,
		nil, responseHeaderFilterForTest(cfg), nil, nil, nil,
	)
	gatewayService.Recorder = newHTTPCompletionFixture(cfg, nil,

		nil,

		nil,

		nil,

		nil, nil, true)

	billingService := newBillingEligibilityFixture(cfg)
	billingService.Start()
	t.Cleanup(billingService.Stop)
	concurrencyService := scheduler.NewConcurrencyService(nil, scheduler.Diagnostics{
		Logf:  logging.LegacyPrintf,
		Event: logging.Event,
	},
	)
	handler := newGatewayHTTPEndpointsFromDeps(
		gatewayService, gatewayServiceCredentialPort,
		concurrencyService, newFundingAdmissionFixture(billingService, cfg), testkit.NewService(nil, nil, nil, nil, nil, nil, cfg),
		nil,
		nil,
		nil,
		nil,
		cfg, nil, newExecutionAvailabilityForTest(providerRepo,

			nil, cfg), gatewayServiceChoices,
	)
	handler.Input.MaxSwitches = 10
	return handler
}

func newOpenAIResponsesFailoverTestContext(t *testing.T, ctx context.Context) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	groupID := int64(3131)
	body := []byte(`{"model":"gpt-5.4","stream":false,"input":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Set(string(keyhttp.ContextKeyAPIKey), &apikey.APIKey{
		ID:      99,
		GroupID: &groupID,
		Group: &routing.Group{
			ID: groupID,
		},
		User: &identity.User{ID: 100},
	})
	c.Set(string(authctx.ContextKeyUser), authctx.AuthSubject{UserID: 100, Concurrency: 0})
	return c, rec
}

// TestOpenAIGatewayHandlerResponses_FailoverAbortsWhenClientDisconnected 复现
// #4257：客户端在上游请求在途期间断开，上游随后返回可 failover 的 520。
// 客户端断开后结束提供商选择，将请求归类为 499，
// 提供商 2 的调用次数保持为零。
func TestOpenAIGatewayHandlerResponses_FailoverAbortsWhenClientDisconnected(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	upstream := &openAIResponsesFailoverCancelUpstream{onFirstDo: cancel}
	handler := newOpenAIResponsesFailoverTestHandler(t, upstream)
	c, rec := newOpenAIResponsesFailoverTestContext(t, ctx)

	handler.Responses(c)

	require.Equal(t, []int64{1}, upstream.calls(), "客户端断开后不应再切换到提供商 2")
	require.Equal(t, gatewayhttp.StatusClientClosedRequest, c.Writer.Status(), "应按 499 归类")
	require.Zero(t, rec.Body.Len(), "不应写入 502 错误响应体")

	_, hasFinalUpstreamErr := c.Get(gatewayhttp.OpsUpstreamStatusCodeKey)
	require.False(t, hasFinalUpstreamErr, "不应记录 failover 耗尽的上游错误终态")

	// 上游返回的 520 保留为 failover 事件，执行组件在返回错误前记录。
	rawEvents, ok := c.Get(gatewayhttp.OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := rawEvents.([]*opscore.OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.Len(t, events, 1)
	require.Equal(t, "failover", events[0].Kind)
	require.Equal(t, 520, events[0].UpstreamStatusCode)
}

// TestOpenAIGatewayHandlerResponses_FailoverContinuesForConnectedClient 检查客户端在线时，
// 提供商 1 返回 520 后切换到提供商 2，两个提供商都返回 520 时，
// 按提供商耗尽返回 502。
func TestOpenAIGatewayHandlerResponses_FailoverContinuesForConnectedClient(t *testing.T) {
	logSink, restore := captureHandlerStructuredLog(t)
	defer restore()

	upstream := &openAIResponsesFailoverCancelUpstream{}
	handler := newOpenAIResponsesFailoverTestHandler(t, upstream)
	c, rec := newOpenAIResponsesFailoverTestContext(t, nil)

	handler.Responses(c)

	require.Equal(t, []int64{1, 2}, upstream.calls(), "在线客户端应正常切换提供商")
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Equal(t, "upstream_error", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
	require.True(t, logSink.ContainsMessageAtLevel("openai.upstream_failover_switching", "warn"))
	require.True(t, logSink.ContainsFieldValue("proxy_id", "11"))
	require.True(t, logSink.ContainsFieldValue("proxy_name", "responses-proxy"))
	require.True(t, logSink.ContainsFieldValue("proxy_host", "proxy.example.com"))
	require.True(t, logSink.ContainsFieldValue("proxy_port", "8080"))
	require.False(t, logSink.ContainsFieldValue("proxy_username", "proxy-user-secret"))
	require.False(t, logSink.ContainsFieldValue("proxy_password", "proxy-password-secret"))
}

func newServiceTierHandlerTest(t *testing.T) *gatewayHTTPEndpointsFixture {
	t.Helper()
	return newGatewayHTTPEndpoints(gatewayHTTPFixtureInput{
		Source:  &gatewayExecutionFixture{},
		Funding: newFundingAdmissionFixture(newBillingEligibilityFixture(&config.Config{}), &config.Config{}),
		Keys:    &apikey.APIKeyService{},
		Concurrency: gatewayhttp.NewConcurrencyHelper(scheduler.NewConcurrencyService(
			&httptestkit.ConcurrencySequence{UserSeq: []bool{true}}, scheduler.Diagnostics{
				Logf:  logging.LegacyPrintf,
				Event: logging.Event,
			},
		), gatewayhttp.SSEPingFormatNone, 0),
		Config: &config.Config{},
		Images: &scheduler.ImageConcurrencyLimiter{}, Availability: newExecutionAvailabilityForTest(nil, nil, nil), Choices: newEmptyCompatibleSelectionFixture(),
	})
}

func runOpenAIHandlerServiceTierTest(t *testing.T, path, body string, handler func(h *gatewayHTTPEndpointsFixture, c *gin.Context)) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	groupID := int64(6401)
	userID := int64(6402)
	c.Set(string(keyhttp.ContextKeyAPIKey), &apikey.APIKey{
		ID:      6403,
		GroupID: &groupID,
		Group: &routing.Group{
			ID: groupID,
		},
		User: &identity.User{ID: userID, Status: billing.StatusActive},
	})
	c.Set(string(authctx.ContextKeyUser), authctx.AuthSubject{UserID: userID, Concurrency: 1})

	handler(newServiceTierHandlerTest(t), c)
	return rec
}

func TestOpenAIGatewayHandlerResponses_InvalidServiceTierRejected400(t *testing.T) {
	for _, body := range []string{
		`{"model":"gpt-5.5","input":"hi","service_tier":"turbo"}`,
		`{"model":"gpt-5.5","input":"hi","service_tier":"SPEED"}`,
		`{"model":"gpt-5.5","input":"hi","service_tier":""}`,
		`{"model":"gpt-5.5","input":"hi","service_tier":123}`,
		`{"model":"gpt-5.5","input":"hi","service_tier":{}}`,
	} {
		rec := runOpenAIHandlerServiceTierTest(t, "/v1/responses", body, func(h *gatewayHTTPEndpointsFixture, c *gin.Context) {
			h.Responses(c)
		})
		require.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", body)
		require.Contains(t, rec.Body.String(), "invalid_request_error", "body=%s", body)
		require.Contains(t, rec.Body.String(), "invalid service_tier", "body=%s", body)
	}
}

func TestOpenAIGatewayHandlerChatCompletions_InvalidServiceTierRejected400(t *testing.T) {
	for _, body := range []string{
		`{"model":"gpt-5.5","messages":[{"role":"user","content":"hi"}],"service_tier":"turbo"}`,
		`{"model":"gpt-5.5","messages":[{"role":"user","content":"hi"}],"service_tier":"ultra"}`,
		`{"model":"gpt-5.5","messages":[{"role":"user","content":"hi"}],"service_tier":""}`,
		`{"model":"gpt-5.5","messages":[{"role":"user","content":"hi"}],"service_tier":["priority"]}`,
	} {
		rec := runOpenAIHandlerServiceTierTest(t, "/v1/chat/completions", body, func(h *gatewayHTTPEndpointsFixture, c *gin.Context) {
			h.ChatCompletions(c)
		})
		require.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", body)
		require.Contains(t, rec.Body.String(), "invalid_request_error", "body=%s", body)
		require.Contains(t, rec.Body.String(), "invalid service_tier", "body=%s", body)
	}
}

func TestOpenAICompatibleHandlersRejectInvalidStreamFieldType(t *testing.T) {
	tests := []struct {
		name string
		path string
		body string
		run  func(*gin.Context)
	}{
		{
			name: "gateway_responses_string_stream",
			path: "/v1/responses",
			body: `{"model":"gpt-5","stream":"true","input":"hello"}`,
			run: func(c *gin.Context) {
				newMessageEndpointsFixture(nil, nil, nil, nil, gatewayhttp.MessagesHTTPOptions{MaxBodyBytes: openAITextOptions(nil).MaxBodyBytes, MaxSwitches: 0, MaxGeminiSwitches: 0}, nil, nil).Responses(c)
			},
		},
		{
			name: "gateway_responses_number_stream",
			path: "/v1/responses",
			body: `{"model":"gpt-5","stream":1,"input":"hello"}`,
			run: func(c *gin.Context) {
				newMessageEndpointsFixture(nil, nil, nil, nil, gatewayhttp.MessagesHTTPOptions{MaxBodyBytes: openAITextOptions(nil).MaxBodyBytes, MaxSwitches: 0, MaxGeminiSwitches: 0}, nil, nil).Responses(c)
			},
		},
		{
			name: "gateway_chat_completions_string_stream",
			path: "/v1/chat/completions",
			body: `{"model":"gpt-5","stream":"true","messages":[{"role":"user","content":"hello"}]}`,
			run: func(c *gin.Context) {
				newMessageEndpointsFixture(nil, nil, nil, nil, gatewayhttp.MessagesHTTPOptions{MaxBodyBytes: openAITextOptions(nil).MaxBodyBytes, MaxSwitches: 0, MaxGeminiSwitches: 0}, nil, nil).ChatCompletions(c)
			},
		},
		{
			name: "gateway_chat_completions_number_stream",
			path: "/v1/chat/completions",
			body: `{"model":"gpt-5","stream":1,"messages":[{"role":"user","content":"hello"}]}`,
			run: func(c *gin.Context) {
				newMessageEndpointsFixture(nil, nil, nil, nil, gatewayhttp.MessagesHTTPOptions{MaxBodyBytes: openAITextOptions(nil).MaxBodyBytes, MaxSwitches: 0, MaxGeminiSwitches: 0}, nil, nil).ChatCompletions(c)
			},
		},
		{
			name: "openai_responses_string_stream",
			path: "/openai/v1/responses",
			body: `{"model":"gpt-5","stream":"true","input":"hello"}`,
			run: func(c *gin.Context) {
				newOpenAIHandlerForPreviousResponseIDValidation(t, nil).Responses(c)
			},
		},
		{
			name: "openai_responses_number_stream",
			path: "/openai/v1/responses",
			body: `{"model":"gpt-5","stream":1,"input":"hello"}`,
			run: func(c *gin.Context) {
				newOpenAIHandlerForPreviousResponseIDValidation(t, nil).Responses(c)
			},
		},
		{
			name: "openai_chat_completions_string_stream",
			path: "/openai/v1/chat/completions",
			body: `{"model":"gpt-5","stream":"true","messages":[{"role":"user","content":"hello"}]}`,
			run: func(c *gin.Context) {
				newOpenAIHandlerForPreviousResponseIDValidation(t, nil).ChatCompletions(c)
			},
		},
		{
			name: "openai_chat_completions_number_stream",
			path: "/openai/v1/chat/completions",
			body: `{"model":"gpt-5","stream":1,"messages":[{"role":"user","content":"hello"}]}`,
			run: func(c *gin.Context) {
				newOpenAIHandlerForPreviousResponseIDValidation(t, nil).ChatCompletions(c)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, rec := newOpenAICompatibleStreamValidationContext(tt.path, tt.body, false)

			tt.run(c)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Equal(t, gatewayhttp.InvalidStreamFieldTypeMessage, gjson.GetBytes(rec.Body.Bytes(), "error.message").String())
			require.Contains(t, rec.Body.String(), "invalid_request_error")
		})
	}
}

func TestGatewayOpenAICompatibleHandlersAllowBooleanStreamToContinue(t *testing.T) {
	tests := []struct {
		name string
		path string
		body string
		run  func(*gin.Context)
	}{
		{
			name: "responses_false",
			path: "/v1/responses",
			body: `{"model":"gpt-5","stream":false,"input":"hello"}`,
			run: func(c *gin.Context) {
				newMessageEndpointsFixture(&messageExecutionFixture{Routes: gatewayprovider.NewRoutePlanner(nil)}, nil, nil, nil, gatewayhttp.MessagesHTTPOptions{MaxBodyBytes: openAITextOptions(nil).MaxBodyBytes, MaxSwitches: 0, MaxGeminiSwitches: 0}, newExecutionAvailabilityForTest(nil, nil, nil), newEmptyGenericSelectionFixture()).Responses(c)
			},
		},
		{
			name: "chat_completions_true",
			path: "/v1/chat/completions",
			body: `{"model":"gpt-5","stream":true,"messages":[{"role":"user","content":"hello"}]}`,
			run: func(c *gin.Context) {
				newMessageEndpointsFixture(&messageExecutionFixture{Routes: gatewayprovider.NewRoutePlanner(nil)}, nil, nil, nil, gatewayhttp.MessagesHTTPOptions{MaxBodyBytes: openAITextOptions(nil).MaxBodyBytes, MaxSwitches: 0, MaxGeminiSwitches: 0}, newExecutionAvailabilityForTest(nil, nil, nil), newEmptyGenericSelectionFixture()).ChatCompletions(c)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, rec := newOpenAICompatibleStreamValidationContext(tt.path, tt.body, true)

			tt.run(c)

			require.Equal(t, http.StatusForbidden, rec.Code)
			require.Contains(t, rec.Body.String(), routing.ErrClaudeCodeOnly.Error())
		})
	}
}

func newOpenAICompatibleStreamValidationContext(path, body string, claudeCodeOnly bool) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	groupID := int64(7)
	c.Set(string(keyhttp.ContextKeyAPIKey), &apikey.APIKey{
		ID:      11,
		GroupID: &groupID,
		Group:   &routing.Group{ID: groupID, ClaudeCodeOnly: claudeCodeOnly},
		User:    &identity.User{ID: 13},
	})
	c.Set(string(authctx.ContextKeyUser), authctx.AuthSubject{UserID: 13, Concurrency: 1})

	return c, rec
}

// TestOpenAITextAssemblyReadAndStopBoundaries 检查三种文本入口在读取报文前拒绝缺失依赖或已关闭的请求。
func TestOpenAITextAssemblyReadAndStopBoundaries(t *testing.T) {
	activity := &gatewayRequestActivity{Operations: lifecycle.NewOperations("openai-text-contract")}
	h := provideOpenAITextHTTP(nil, nil, nil, nil, nil, nil, nil, nil, nil, provideOpenAITextAttemptRuntime(provideOpenAIAttemptBindings(nil, nil, nil, nil, nil, nil, GatewayCompletionRecorders{}, nil, nil, nil, nil, nil, nil, nil, nil, nil)), activity, nil, nil, nil, nil)
	for _, stopped := range []bool{false, true} {
		if stopped {
			require.NoError(t, activity.StopContext(context.Background()))
		}
		for name, entry := range map[string]gin.HandlerFunc{"responses": h.Responses, "chat": h.ChatCompletions, "messages": h.Messages} {
			t.Run(name+map[bool]string{false: "-running", true: "-stopped"}[stopped], func(t *testing.T) {
				writer := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(writer)
				body := &protocolGateTrackingReader{}
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/"+name, body)
				c.Set(string(keyhttp.ContextKeyAPIKey), &apikey.APIKey{ID: 9, UserID: 7})
				c.Set(authctx.ContextKeyUser, authctx.AuthSubject{UserID: 7})
				entry(c)
				require.Equal(t, http.StatusServiceUnavailable, writer.Code)
				require.False(t, body.read)
				if stopped {
					require.Contains(t, writer.Body.String(), "Service is shutting down")
				} else {
					require.Contains(t, writer.Body.String(), "Service temporarily unavailable")
				}
			})
		}
	}
}

func (s *mixedHTTPTransport) Do(req *http.Request, _ string, id int64, _ int) (*http.Response, error) {
	s.mu.Lock()
	s.providers = append(s.providers, id)
	s.mu.Unlock()
	return http.DefaultClient.Do(req)
}

func (s *mixedHTTPTransport) DoWithTLS(req *http.Request, p string, id int64, n int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, p, id, n)
}

func (s *mixedHTTPTransport) calls() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.providers)
}

func (mixedHTTPNoSearch) Current() searchtools.Searcher { return nil }

func mixedUpstreamResponse(w http.ResponseWriter, r *http.Request, failAnthropic bool) {
	body, _ := io.ReadAll(r.Body)
	if failAnthropic && strings.Contains(r.URL.Path, "/messages") {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, `{"error":{"type":"overloaded_error","message":"retry another provider"}}`)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if strings.Contains(r.URL.Path, "/messages") {
		if gjson.GetBytes(body, "stream").Bool() {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg-mixed\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-sonnet-4-5-20250929\",\"content\":[],\"usage\":{\"input_tokens\":10,\"output_tokens\":0}}}\n\nevent: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"ok-mixed\"}}\n\nevent: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":3}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
		} else {
			_, _ = io.WriteString(w, `{"id":"msg-mixed","type":"message","role":"assistant","model":"claude-sonnet-4-5-20250929","content":[{"type":"text","text":"ok-mixed"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":3}}`)
		}
	} else if strings.Contains(r.URL.Path, "models/") {
		response := `{"candidates":[{"content":{"role":"model","parts":[{"text":"ok-mixed"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":3,"totalTokenCount":13}}`
		if strings.Contains(r.URL.Path, "streamGenerateContent") {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprintf(w, "data: %s\n\n", response)
		} else {
			_, _ = io.WriteString(w, response)
		}
	} else if strings.Contains(r.URL.Path, "chat/completions") {
		_, _ = io.WriteString(w, `{"id":"chat-mixed","object":"chat.completion","model":"gpt-5.4","choices":[{"index":0,"message":{"role":"assistant","content":"ok-mixed"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13}}`)
	} else {
		response := `{"id":"resp-mixed","object":"response","status":"completed","model":"gpt-5.4","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok-mixed"}]}],"usage":{"input_tokens":10,"output_tokens":3,"total_tokens":13}}`
		if gjson.GetBytes(body, "stream").Bool() {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprintf(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp-mixed\",\"model\":\"gpt-5.4\"}}\n\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"ok-mixed\"}\n\ndata: {\"type\":\"response.completed\",\"response\":%s}\n\n", response)
		} else {
			_, _ = io.WriteString(w, response)
		}
	}
}

// TestMixedGroupTextHTTP 用实际选择、协议转换及完成器覆盖三种入口与跨平台故障转移。
func TestMixedGroupTextHTTP(t *testing.T) {
	for _, path := range []string{"/v1/messages", "/v1/responses", "/v1/chat/completions"} {
		for _, platform := range []string{"anthropic", "openai", "gemini", "failover"} {
			t.Run(path+"/"+platform, func(t *testing.T) {
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { mixedUpstreamResponse(w, r, platform == "failover") }))
				defer upstream.Close()
				groupID := int64(42)
				group := &routing.Group{ID: groupID, Hydrated: true, Name: "mixed", Status: "active", RateMultiplier: 1, AllowedProtocols: []protocol.ProtocolID{protocol.ProtocolAnthropicMessages, protocol.ProtocolOpenAIResponses, protocol.ProtocolOpenAIChatCompletions}}
				cfg := &config.Config{}
				cfg.Default.RateMultiplier = 1
				cfg.Security.URLAllowlist.Enabled = false
				cfg.Security.URLAllowlist.AllowInsecureHTTP = true
				cfg.Gateway.MaxProviderSwitches = 2
				providers := &mixedHTTPProviders{}
				for index, p := range []string{"anthropic", "openai", "gemini"} {
					record := &providercore.Record{ID: int64(index + 1), Name: p, Platform: p, Type: "apikey", Status: "active", Schedulable: true, Priority: index + 1, Concurrency: 1, GroupIDs: []int64{groupID}, Credentials: map[string]any{"api_key": "test-key", "base_url": upstream.URL}}
					// 混合分组的测试提供商明确声明各自可承接的型号。
					record.Credentials["model_whitelist"] = []string{map[string]string{"anthropic": "claude-sonnet-4-5-20250929", "openai": "gpt-5.4", "gemini": "gemini-2.5-flash"}[p]}
					if p == "openai" && platform == "failover" {
						record.Credentials["model_mapping"] = map[string]any{"claude-sonnet-4-5-20250929": "gpt-5.4"}
					}
					providers.values = append(providers.values, *gatewayprovider.NewExecutionProvider(record))
				}
				transport := &mixedHTTPTransport{}
				source, choices, credentials := newOpenAIExecutionAndSelectionFixture(providers, nil, cfg, nil, nil, nil, transport, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
				logs := &gatewaytestkit.UsageLogStore{Inserted: true}
				funds := &gatewaytestkit.SettlementStore{}
				recording := gatewaytestkit.NewRecording(logs, funds, nil, false)
				recording.Options.DefaultMultiplier = 1
				source.Recorder = recording.Core(nil, true)
				native := &gatewayhttp.UnifiedTextExecutor{OpenAI: source.Responses, Anthropic: gatewayhttp.NewMessagesExecutor(messageforward.NewRuntime(messageforward.Dependencies{Credentials: &providercore.MessageCredentialSource{}, Transport: transport, Deferred: &providercore.DeferredService{}, Search: searchtools.NewEmulator(mixedHTTPNoSearch{}, nil, nil, nil, nil, nil)}, messageforward.Options{Configured: true, AllowInsecureHTTP: true, ResponseReadLimit: 1 << 20}), nil), Gemini: &gatewayhttp.GeminiExecutor{Runtime: &googleforward.Gemini{Transport: transport, Options: googleforward.Options{Configured: true, AllowInsecureHTTP: true, ResponseReadLimit: 1 << 20}}}}
				eligibility := newBillingEligibilityFixture(cfg)
				t.Cleanup(eligibility.Stop)
				resources := provideOpenAIHTTPResources(scheduler.NewConcurrencyService(nil), cfg)
				endpoints := newGatewayHTTPEndpoints(gatewayHTTPFixtureInput{Source: source, Native: native, Credentials: credentials, Choices: choices, Config: cfg, Funding: newFundingAdmissionFixture(eligibility, cfg), Keys: testkit.NewService(nil, nil, nil, nil, nil, nil, cfg), Concurrency: resources.Concurrency, MaxSwitches: 2})
				user := &identity.User{ID: 7, Balance: 100, Status: "active"}
				key := &apikey.APIKey{ID: 8, UserID: 7, User: user, GroupID: &groupID, Group: group}
				router := gin.New()
				router.Use(func(c *gin.Context) {
					c.Set(string(keyhttp.ContextKeyAPIKey), key)
					c.Set("gateway_effective_key", key)
					authctx.SetPrincipal(c, identity.Principal{UserID: 7}, 0, "")
					sourceProtocol := protocol.ProtocolAnthropicMessages
					if path == "/v1/responses" {
						sourceProtocol = protocol.ProtocolOpenAIResponses
					}
					if path == "/v1/chat/completions" {
						sourceProtocol = protocol.ProtocolOpenAIChatCompletions
					}
					c.Request = c.Request.WithContext(requeststate.WithClientProtocol(requeststate.WithGroup(c.Request.Context(), group), sourceProtocol))
				})
				router.POST("/v1/messages", endpoints.Messages)
				router.POST("/v1/responses", endpoints.Responses)
				router.POST("/v1/chat/completions", endpoints.ChatCompletions)
				model := map[string]string{"anthropic": "claude-sonnet-4-5-20250929", "openai": "gpt-5.4", "gemini": "gemini-2.5-flash", "failover": "claude-sonnet-4-5-20250929"}[platform]
				body := fmt.Sprintf(`{"model":%q,"max_tokens":20,"messages":[{"role":"user","content":"hello"}],"stream":false}`, model)
				if path == "/v1/responses" {
					body = fmt.Sprintf(`{"model":%q,"input":"hello","stream":false}`, model)
				}
				response := httptest.NewRecorder()
				request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
				request.Header.Set("Content-Type", "application/json")
				router.ServeHTTP(response, request)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				require.Contains(t, response.Body.String(), "ok-mixed")
				expected := map[string]int64{"anthropic": 1, "openai": 2, "gemini": 3, "failover": 2}[platform]
				calls := transport.calls()
				require.NotEmpty(t, calls)
				require.Equal(t, expected, calls[len(calls)-1])
				if platform == "failover" {
					require.Equal(t, []int64{1, 2}, calls)
				} else {
					require.Len(t, calls, 1)
				}
				require.Equal(t, 1, funds.Calls, "一次成功只能提交一次资金扣费")
				require.Equal(t, 1, logs.Calls)
				require.Equal(t, expected, logs.LastLog.ProviderID)
				require.Equal(t, map[int64]string{1: "anthropic", 2: "openai", 3: "gemini"}[expected], logs.LastLog.Platform)
			})
		}
	}
}

func (f responsesWireTransport) Do(req *http.Request, _ string, id int64, _ int) (*http.Response, error) {
	return f(req, id)
}

func (f responsesWireTransport) DoWithTLS(req *http.Request, _ string, id int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return f(req, id)
}

// TestResponsesDesktopContextForwarding 核对经过真实入口和选路之后送到假上游的两轮报文。
func TestResponsesDesktopContextForwarding(t *testing.T) {
	for _, tc := range []struct {
		name, contextName, encoding string
		native, secondRoute         bool
	}{
		{"native", "browser_tabs", "identity", true, false},
		{"codex_oauth", "terminal_info", "identity", true, false},
		{"standard", "terminal_info", "gzip", false, false},
		{"next_compatible", "another_context", "zstd", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := gatewayprovider.ExecutionProvider{Record: providercore.Record{
				ID: 11, LoadLocation: time.LoadLocation,
				Name: "test-relay", Platform: capability.PlatformOpenAI, Type: capability.ProviderTypeAPIKey, Status: billing.StatusActive,
				Schedulable: true, Credentials: map[string]any{"api_key": "test", "base_url": "https://provider.example.test"},
				Extra: map[string]any{"openai_passthrough": true, providercore.ExtraKeyResponsesAgentContext: tc.native && !tc.secondRoute},
			}}
			if tc.name == "codex_oauth" {
				provider.Record.Type = capability.ProviderTypeOAuth
				provider.Record.Credentials = map[string]any{"access_token": "test"}
				provider.Record.Extra = nil
			}
			providers := []gatewayprovider.ExecutionProvider{provider}
			expectedID := int64(11)
			if tc.secondRoute {
				second := provider
				second.Record.ID = 12
				second.Record.Priority = 2
				second.Record.Extra = map[string]any{"openai_passthrough": true, providercore.ExtraKeyResponsesAgentContext: true}
				providers = append(providers, second)
				expectedID = 12
			}
			var hits int
			upstream := responsesWireTransport(func(req *http.Request, id int64) (*http.Response, error) {
				hits++
				require.Equal(t, expectedID, id)
				wire, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				input := gjson.GetBytes(wire, "input").Array()
				require.Len(t, input, 16+2*(hits-1))
				if tc.native {
					require.Equal(t, "agent", input[15].Get("namespace").Str)
					require.Equal(t, tc.contextName, input[15].Get("name").Str)
					require.False(t, input[15].Get("call_id").Exists())
				} else {
					require.Equal(t, "user", input[15].Get("role").Str)
					require.Contains(t, input[15].Get("content.0.text").Str, "&lt;note&gt;")
				}
				stream := `data: {"type":"response.output_item.added","output_index":0,"item":{"id":"fc_item42","type":"function_call","call_id":"call_read","name":"read","arguments":""}}` + "\n\n" +
					`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"fc_item42","type":"function_call","name":"read","call_id":"call_read","arguments":"{}"}}` + "\n\n" +
					`data: {"type":"response.completed","response":{"id":"resp_tool","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n"
				if hits == 2 {
					upstreamID := "call_read"
					if tc.name == "codex_oauth" {
						upstreamID = "fc_read"
					}
					require.Equal(t, upstreamID, input[16].Get("call_id").Str)
					require.Equal(t, upstreamID, input[17].Get("call_id").Str)
					stream = `data: {"type":"response.output_text.delta","delta":"ok"}` + "\n\n" + `data: {"type":"response.completed","response":{"id":"resp_answer","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n"
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream))}, nil
			})
			h := newOpenAIResponsesFailoverTestHandler(t, upstream, providers...)
			input := make([]any, 0, 18)
			for range 15 {
				input = append(input, map[string]string{"role": "user", "content": "hi"})
			}
			input = append(input, map[string]string{"type": "function_call_output", "id": "fco_ui", "namespace": "agent", "name": tc.contextName, "output": "<note>"})
			for turn := range 2 {
				body, err := json.Marshal(map[string]any{"input": input, "model": "gpt-5.4", "stream": true})
				require.NoError(t, err)
				encoding := tc.encoding
				if encoding == "zstd" && turn > 0 {
					encoding = "identity"
				}
				switch encoding {
				case "gzip":
					var buf bytes.Buffer
					z := gzip.NewWriter(&buf)
					_, err = z.Write(body)
					require.NoError(t, err)
					require.NoError(t, z.Close())
					body = buf.Bytes()
				case "zstd":
					// 第一轮使用固定的 Zstandard 线格式，第二轮把实际收到的工具编号写入请求。
					body, err = os.ReadFile("testdata/desktop-context-turn.zst")
					require.NoError(t, err)
				}
				c, w := newOpenAIResponsesFailoverTestContext(t, context.Background())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
				c.Request.Header.Set("Content-Type", "application/json")
				c.Request.Header.Set("Content-Encoding", encoding)
				h.Responses(c)
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				require.Equal(t, 1, strings.Count(w.Body.String(), `"type":"response.completed"`))
				if turn == 0 {
					var call json.RawMessage
					protocolopenai.ForEachSSEDataPayload(w.Body.String(), func(event []byte) {
						if gjson.GetBytes(event, "type").Str == "response.output_item.done" {
							call = json.RawMessage(gjson.GetBytes(event, "item").Raw)
						}
					})
					require.Equal(t, "call_read", gjson.GetBytes(call, "call_id").Str)
					input = append(input, call, map[string]string{"type": "function_call_output", "call_id": gjson.GetBytes(call, "call_id").Str, "output": "ok"})
				}
			}
			require.Equal(t, 2, hits)
		})
	}
}

// TestResponsesDelegationReplay 核对第八项 Codex 客户端条目经过 HTTP 入口后的位置、正文和相邻工具调用的编号。
// 用例包括跨任务消息、工具历史后的自动化心跳、正文含未转义特殊字符的消息和未知工具名。
func TestResponsesDelegationReplay(t *testing.T) {
	const envelope = "<codex_delegation><source_thread_id>thread-A</source_thread_id><input>inspect &lt;main&gt;</input></codex_delegation>"
	const heartbeat = "<heartbeat><automation_id>daily-review</automation_id></heartbeat>"
	const rawDelegation = "<codex_delegation><source_thread_id>thread-A</source_thread_id><input>run a && b</input></codex_delegation>"
	const history = `{"type":"message","role":"user","content":"hi"},
		{"type":"function_call_output","namespace":"agent","name":"terminal_info","output":"cwd"},
		{"type":"function_call","call_id":"call_read","name":"read","arguments":"{}"},
		{"type":"function_call_output","call_id":"call_read","output":"ok"},
		{"type":"tool_search_call","execution":"server","call_id":null,"arguments":{}},
		{"type":"tool_search_output","execution":"server","call_id":null,"tools":[]},
		{"type":"web_search_call","id":"ws_1","status":"completed"}`
	rawJSON, err := json.Marshal(rawDelegation)
	require.NoError(t, err)
	for _, tc := range []struct {
		name, tool, output string
		status             int
		// text 是期望原样转发的正文。text 为空时期望来源标记，wrapped 是标记内的正文。
		text, wrapped string
	}{
		{name: "text", tool: "send_message_to_thread", output: `"` + envelope + `"`, status: http.StatusOK, text: envelope},
		{name: "text items", tool: "send_message_to_thread", output: `[{"type":"input_text","text":"` + envelope + `"}]`, status: http.StatusOK, text: envelope},
		{name: "automation heartbeat after history", tool: "automation_update", output: `"` + heartbeat + `"`, status: http.StatusOK, text: heartbeat},
		{name: "raw special characters", tool: "send_message_to_thread", output: string(rawJSON), status: http.StatusOK, wrapped: "run a &amp;&amp; b"},
		{name: "ordinary text", tool: "send_message_to_thread", output: `"done"`, status: http.StatusOK, wrapped: "done"},
		{name: "unknown tool", tool: "automation_create", output: `"payload"`, status: http.StatusOK, wrapped: "payload"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			upstream := responsesWireTransport(func(req *http.Request, _ int64) (*http.Response, error) {
				calls++
				wire, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				items := gjson.GetBytes(wire, "input").Array()
				require.Len(t, items, 8)
				require.Equal(t, "call_read", items[2].Get("call_id").Str)
				require.Equal(t, "call_read", items[3].Get("call_id").Str)
				require.Equal(t, "message", items[7].Get("type").Str)
				require.Equal(t, "user", items[7].Get("role").Str)
				require.False(t, items[7].Get("call_id").Exists())
				text := items[7].Get("content.0.text").Str
				if tc.text != "" {
					require.Equal(t, tc.text, text)
				} else {
					require.Contains(t, text, `<codex_client_item namespace="codex_app" name="`+tc.tool+`">`)
					require.Contains(t, text, tc.wrapped)
				}
				stream := `data: {"type":"response.completed","response":{"id":"resp_delegation","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n"
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream))}, nil
			})
			provider := gatewayprovider.ExecutionProvider{Record: providercore.Record{
				ID: 11, Name: "relay", Platform: capability.PlatformOpenAI, Type: capability.ProviderTypeAPIKey,
				Status: billing.StatusActive, Schedulable: true, LoadLocation: time.LoadLocation,
				Credentials: map[string]any{"api_key": "test", "base_url": "https://provider.example.test"},
				Extra:       map[string]any{"openai_passthrough": true, providercore.ExtraKeyResponsesAgentContext: true},
			}}
			h := newOpenAIResponsesFailoverTestHandler(t, upstream, provider)
			body := `{"model":"gpt-5.4","stream":true,"input":[` + history + `,
				{"type":"function_call_output","id":"fco_dispatch","namespace":"codex_app","name":"` + tc.tool + `","output":` + tc.output + `}]}`
			c, w := newOpenAIResponsesFailoverTestContext(t, context.Background())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			h.Responses(c)
			require.Equal(t, tc.status, w.Code, w.Body.String())
			require.Equal(t, 1, calls)
			require.Contains(t, w.Body.String(), "response.completed")
		})
	}
}

// TestResponsesMissingCallIDStillRejected 核对不带 namespace 的工具结果缺少编号时，入口返回 400，上游收不到请求。
func TestResponsesMissingCallIDStillRejected(t *testing.T) {
	calls := 0
	upstream := responsesWireTransport(func(req *http.Request, _ int64) (*http.Response, error) {
		calls++
		return nil, io.EOF
	})
	provider := gatewayprovider.ExecutionProvider{Record: providercore.Record{
		ID: 11, Name: "relay", Platform: capability.PlatformOpenAI, Type: capability.ProviderTypeAPIKey,
		Status: billing.StatusActive, Schedulable: true, LoadLocation: time.LoadLocation,
		Credentials: map[string]any{"api_key": "test", "base_url": "https://provider.example.test"},
		Extra:       map[string]any{"openai_passthrough": true},
	}}
	h := newOpenAIResponsesFailoverTestHandler(t, upstream, provider)
	body := `{"model":"gpt-5.4","stream":true,"input":[{"type":"message","role":"user","content":"hi"},
		{"type":"function_call_output","namespace":"codex_app","name":"send_message_to_thread","output":"ok"},
		{"type":"function_call_output","output":"done"}]}`
	c, w := newOpenAIResponsesFailoverTestContext(t, context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Responses(c)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	require.Zero(t, calls)
	require.Contains(t, w.Body.String(), "input[2] (function_call_output) requires call_id")
}

// TestResponsesDisconnectReplay 检查首段前可以换号，工具调用已经交付后只返回一个失败终态。
func TestResponsesDisconnectReplay(t *testing.T) {
	for _, tc := range []struct {
		name, partial string
		calls         int
		terminal      string
	}{
		{"before_output", "", 2, "response.completed"},
		{"after_tool", `data: {"type":"response.output_item.done","output_index":0,"item":{"id":"fc_job","type":"function_call","call_id":"call_job","name":"job","arguments":"{}"}}` + "\n\n", 1, "response.failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var ids []int64
			upstream := responsesWireTransport(func(_ *http.Request, id int64) (*http.Response, error) {
				ids = append(ids, id)
				body := `data: {"type":"response.created","response":{"id":"resp_partial"}}` + "\n\n" + tc.partial + "data: [DONE]\n\n"
				if len(ids) > 1 {
					body = `data: {"type":"response.output_text.delta","delta":"ok"}` + "\n\n" + `data: {"type":"response.completed","response":{"id":"resp_ok","status":"completed","usage":{"input_tokens":1,"output_tokens":1},"output":[]}}` + "\n\n"
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			h := newOpenAIResponsesFailoverTestHandler(t, upstream)
			c, w := newOpenAIResponsesFailoverTestContext(t, context.Background())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"input":"test","model":"gpt-5.4","stream":true}`))
			c.Request.Header.Set("Content-Type", "application/json")
			h.Responses(c)
			require.Len(t, ids, tc.calls, w.Body.String())
			require.Equal(t, 1, strings.Count(w.Body.String(), `"type":"`+tc.terminal+`"`))
			if tc.calls == 1 {
				require.Equal(t, 1, strings.Count(w.Body.String(), `"call_id":"call_job"`))
				require.NotContains(t, w.Body.String(), `"type":"response.completed"`)
				require.Equal(t, "written", c.GetString("responses_failure_write"))
			} else {
				require.NotEqual(t, ids[0], ids[1])
				require.NotContains(t, w.Body.String(), "resp_partial")
				require.NotContains(t, w.Body.String(), `"type":"response.failed"`)
			}
		})
	}
}
