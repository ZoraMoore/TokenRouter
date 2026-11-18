package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	gatewayprovider "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	providercore "github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
	"github.com/TokenFlux/TokenRouter/internal/upstream/openai"
)

// TestCodexIdentityProjectionHeaderBodyParity 检查头、正文和嵌套字段使用同一尝试身份。
func TestCodexIdentityProjectionHeaderBodyParity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, raw := range []bool{false, true} {
		t.Run(map[bool]string{false: "map", true: "raw"}[raw], func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Request.Header.Set("session-id", "header-session")
			c.Request.Header.Set("session_id", "header-session-alias")
			c.Request.Header.Set("thread-id", "header-thread")
			c.Request.Header.Set("thread_id", "header-thread-alias")
			p := &gatewayprovider.ExecutionProvider{Record: providercore.Record{ID: 77, Platform: capability.PlatformOpenAI, Type: capability.ProviderTypeOAuth, Credentials: map[string]any{"chatgpt_account_id": "fixture-account"}}}
			embedded := `{"session_id":"embedded-session","thread_id":"embedded-thread"}`
			body := map[string]any{"model": "gpt-6.1-sol", "input": json.RawMessage(`[ { "role": "user", "content": "verbatim" } ]`), "client_metadata": map[string]any{"session_id": "body-session", "session-id": "body-session-alias", "thread_id": "body-thread", openai.WSTurnMetadataHeader: embedded}, "prompt_cache_key": "body-session"}
			var payload []byte
			var err error
			if raw {
				payload, err = json.Marshal(body)
				require.NoError(t, err)
				input := gjson.GetBytes(payload, "input").Raw
				payload, _, err = ApplyCodexIdentityRaw(c, p, 42, payload)
				require.NoError(t, err)
				require.Equal(t, input, gjson.GetBytes(payload, "input").Raw)
			} else {
				require.True(t, ApplyCodexIdentityMap(c, p, 42, body))
				payload, err = json.Marshal(body)
				require.NoError(t, err)
			}
			headers := c.Request.Header.Clone()
			ApplyStagedCodexIdentityHeaders(c, p, 42, headers)
			session := gjson.GetBytes(payload, "client_metadata.session_id").String()
			require.NotEqual(t, "header-session", session)
			require.Equal(t, session, headers.Get("session-id"))
			require.Equal(t, session, headers.Get("session_id"))
			require.Equal(t, session, gjson.GetBytes(payload, "client_metadata.session-id").String())
			require.Equal(t, session, gjson.Get(gjson.GetBytes(payload, "client_metadata."+openai.WSTurnMetadataHeader).String(), "session_id").String())
			require.Equal(t, session, gjson.GetBytes(payload, "prompt_cache_key").String())
			require.Equal(t, headers.Get("thread-id"), gjson.GetBytes(payload, "client_metadata.thread_id").String())
			previous := headers.Get("session_id")
			body2 := map[string]any{"client_metadata": map[string]any{"session_id": "body-session"}}
			require.True(t, ApplyCodexIdentityMap(c, p, 53, body2))
			next := c.Request.Header.Clone()
			ApplyStagedCodexIdentityHeaders(c, p, 53, next)
			require.NotEqual(t, previous, next.Get("session_id"))
		})
	}
}
