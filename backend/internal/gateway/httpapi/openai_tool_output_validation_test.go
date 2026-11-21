package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestValidateOpenAIFunctionCallOutputMissingItem(t *testing.T) {
	// 托管搜索为空是合法历史；记录后续第一个缺编号的客户端输出。
	body := []byte(`{"input":[{"type":"tool_search_output","execution":"server","call_id":null},{"type":"function_call_output","call_id":"call_A","output":"ok"},{"type":"custom_tool_call_output","output":"private result"},{"type":"function_call_output"}]}`)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	logger, logs := newObservedLogger(t)
	require.False(t, ValidateOpenAIFunctionCallOutput(c, body, logger))
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.JSONEq(t, `{"error":{"type":"invalid_request_error","message":"input[2] (custom_tool_call_output) requires call_id"}}`, w.Body.String())
	entries := logs.FilterMessage("openai.request_validation_failed").All()
	require.Len(t, entries, 1)
	require.Equal(t, map[string]any{
		"reason":      "function_call_output_missing_call_id",
		"input_index": int64(2),
		"item_type":   "custom_tool_call_output",
	}, entries[0].ContextMap())
}
