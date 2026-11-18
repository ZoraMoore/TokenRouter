package httpapi

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	forwardcore "github.com/TokenFlux/TokenRouter/internal/gateway/forward"
	gatewayprovider "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	providercore "github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
	upstreamcore "github.com/TokenFlux/TokenRouter/internal/upstream"
	"github.com/TokenFlux/TokenRouter/internal/upstream/openai"
)

// 两种上游压缩项必须具有相同的首输出语义，空占位项不能提交当前尝试。
func TestOpenAICompactionProtectionFailureBeforeOutputRemainsRetryable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, itemType := range []string{"compaction", "compaction_summary"} {
		for _, passthrough := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/passthrough=%t", itemType, passthrough), func(t *testing.T) {
				svc := newResponsesFixture(responsesFixtureInputs{options: &responsesFixtureOptions{Response: OpenAIResponseOptions{MaxLineSize: OpenAIResponseDefaultMaxLineSize}}}).Output
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/responses", nil)
				MarkOpenAINativeCompactionV2(c)
				account := &gatewayprovider.ExecutionProvider{Record: providercore.Record{ID: 7414, Platform: capability.PlatformOpenAI, Type: capability.ProviderTypeOAuth}}
				body := fmt.Sprintf("data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_private\"}}\n\n"+
					"data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":%q,\"id\":\"cmp_private\",\"status\":\"in_progress\"}}\n\n"+
					"data: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_private\",\"status\":\"failed\",\"error\":{\"code\":\"upstream_error\",\"type\":\"internal_error\",\"message\":\"response protection is unavailable\"}}}\n\n", itemType)
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"X-Request-Id": []string{"private-attempt"}}, Body: io.NopCloser(strings.NewReader(body))}
				var err error
				if passthrough {
					_, err = openai.ReadPassthroughStreaming(c.Request.Context(), resp, upstreamcore.NewOutputContext(ResponseSink{Writer: c.Writer}), svc.PassthroughOptions(c.Request.Context(), c, account), time.Now(), "gpt-6.1-sol", "gpt-6.1-sol")
				} else {
					_, err = svc.ReadStreamObservation(c.Request.Context(), resp, c, account, time.Now(), "gpt-6.1-sol", "gpt-6.1-sol", "")
				}
				var failover *forwardcore.UpstreamFailoverError
				require.ErrorAs(t, err, &failover)
				require.Empty(t, rec.Body.String(), "失败尝试的压缩项和响应 ID 不得泄露到客户端")
				require.False(t, c.Writer.Written())
				require.Empty(t, rec.Header().Get("X-Request-Id"))
			})
		}
	}
}

// 原生 V2 在成功终态前暂存密文，普通响应在下发内容后停止换号。
func TestOpenAICompactionProtectionFailureAfterEncryptedOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, native := range []bool{false, true} {
		for _, itemType := range []string{"compaction", "compaction_summary"} {
			t.Run(fmt.Sprintf("%s/native=%t", itemType, native), func(t *testing.T) {
				svc := newResponsesFixture(responsesFixtureInputs{options: &responsesFixtureOptions{Response: OpenAIResponseOptions{MaxLineSize: OpenAIResponseDefaultMaxLineSize}}}).Output
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/responses", nil)
				if native {
					MarkOpenAINativeCompactionV2(c)
				}
				body := fmt.Sprintf("data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":%q,\"encrypted_content\":\"opaque-summary\"}}\n\n"+
					"data: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":{\"code\":\"upstream_error\",\"type\":\"internal_error\",\"message\":\"response protection is unavailable\"}}}\n\n", itemType)
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
				_, err := svc.ReadStreamObservation(c.Request.Context(), resp, c, &gatewayprovider.ExecutionProvider{Record: providercore.Record{ID: 7414, Platform: capability.PlatformOpenAI, Type: capability.ProviderTypeOAuth}}, time.Now(), "gpt-6.1-sol", "gpt-6.1-sol", "")
				require.Error(t, err)
				var failover *forwardcore.UpstreamFailoverError
				if native {
					require.ErrorAs(t, err, &failover)
					require.False(t, c.Writer.Written())
					require.Empty(t, rec.Body.String())
				} else {
					require.NotErrorAs(t, err, &failover)
					require.Contains(t, rec.Body.String(), "opaque-summary")
					require.Equal(t, 1, strings.Count(rec.Body.String(), `"type":"response.failed"`))
				}
			})
		}
	}
}
