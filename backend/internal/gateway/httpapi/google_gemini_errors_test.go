package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	gemininative "github.com/TokenFlux/TokenRouter/internal/upstream/gemini"
)

// TestGeminiErrorsPreserveStreamFraming 错误输出在已有 SSE 中保持格式，并按请求去重。
func TestGeminiErrorsPreserveStreamFraming(t *testing.T) {
	for _, kind := range []string{"gemini", "messages", "chat", "responses"} {
		t.Run(kind, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			path := "/v1/chat/completions"
			if kind == "responses" {
				path = "/v1/responses"
			}
			c.Request = httptest.NewRequest(http.MethodPost, path, nil)
			c.Header("Content-Type", "text/event-stream")
			_, _ = c.Writer.Write([]byte("data: {}\n\n"))
			output := &GeminiOutput{GoogleOutput: GoogleOutput{Context: c}}
			emit := func() {
				switch kind {
				case "gemini":
					_ = output.GoogleError(429, "quota")
				case "messages":
					_ = output.ClaudeError(429, "rate_limit_error", "quota")
				case "chat":
					_ = output.ChatError(429, "rate_limit_error", "quota")
				case "responses":
					_ = output.GeminiOpenAICompatError(gemininative.OpenAICompatResponses, 429, "rate_limit_error", "quota")
				}
			}
			emit()
			emit()
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Contains(t, recorder.Body.String(), "data: ")
			require.Equal(t, 1, strings.Count(recorder.Body.String(), `"error":{`))
			if kind == "responses" {
				require.Contains(t, recorder.Body.String(), "event: response.failed")
			}
		})
	}
}

// TestGeminiErrorSkipsCanceledRequest 请求取消后的错误只返回给调用方。
func TestGeminiErrorSkipsCanceledRequest(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/test:streamGenerateContent", nil).WithContext(ctx)
	output := &GeminiOutput{GoogleOutput: GoogleOutput{Context: c}}
	require.Error(t, output.GoogleError(502, "failed"))
	require.Empty(t, recorder.Body.String())
}
