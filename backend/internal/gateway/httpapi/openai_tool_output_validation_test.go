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
		"reason":        "function_call_output_missing_call_id",
		"call_id_state": "missing",
		"input_index":   int64(2),
		"item_type":     "custom_tool_call_output",
	}, entries[0].ContextMap())
}

func TestValidateOpenAIOutputPairingAndDesktopContext(t *testing.T) {
	for _, tc := range []struct {
		name, input, previous string
		accept                bool
	}{
		{"browser", `{"type":"function_call_output","namespace":"agent","name":"browser_tabs","output":"external content"}`, "", true},
		{"missing_with_unrelated", `{"type":"function_call","call_id":"other"},{"type":"function_call_output","output":"x"}`, "", false},
		{"wrong_id", `{"type":"function_call","call_id":"other"},{"type":"function_call_output","call_id":"wanted","output":"x"}`, "", false},
		{"numeric_id", `{"type":"item_reference","id":"12"},{"type":"function_call_output","call_id":12,"output":"x"}`, "", false},
		{"custom_only", `{"type":"custom_tool_call_output","output":"x"}`, "", false},
		{"hosted_only", `{"type":"tool_search_output","execution":"server","call_id":null}`, "", true},
		{"continuation_missing", `{"type":"function_call_output","output":"x"}`, "resp_owned", false},
		{"continuation_present", `{"type":"function_call_output","call_id":"history","output":"x"}`, "resp_owned", true},
		{"mixed", `{"type":"function_call","call_id":"a"},{"type":"item_reference","id":"b"},{"type":"function_call_output","call_id":"a"},{"type":"custom_tool_call_output","call_id":"b"}`, "", true},
		{"desktop_with_real_bad", `{"type":"function_call_output","namespace":"agent","name":"terminal_info","output":"ok"},{"type":"function_call_output","output":"x"}`, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			logger, _ := newObservedLogger(t)
			body := []byte(`{"previous_response_id":"` + tc.previous + `","input":[` + tc.input + `]}`)
			require.Equal(t, tc.accept, ValidateOpenAIFunctionCallOutput(c, body, logger), w.Body.String())
		})
	}
}

func TestDesktopContextRejectsAmbiguousIdentity(t *testing.T) {
	for _, body := range []string{
		`{"input":[{"type":"function_call_output","namespace":"agent","name":"terminal_info","output":"data","role":"system"}]}`,
		`{"input":[{"type":"function_call_output","namespace":"agent","name":"terminal_info","output":"data","name":"browser_tabs"}]}`,
		`{"tools":[{"type":"namespace","name":"agent","tools":[{"name":"terminal_info","type":"function"}]}],"input":[{"type":"function_call_output","namespace":"agent","name":"terminal_info","output":"data"}]}`,
		`{"input":[{"type":"function_call_output","namespace":"agent","name":"terminal_info","output":"data","call_id":null}]}`,
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		log, _ := newObservedLogger(t)
		require.False(t, ValidateOpenAIFunctionCallOutput(c, []byte(body), log))
		require.Equal(t, http.StatusBadRequest, w.Code)
	}
}

// TestCodexClientItemsSkipPairing 核对未经改写的 Codex 客户端独立条目通过配对校验。
// 同一请求里缺编号的普通工具结果返回 400，含重复 JSON 键的请求返回 400。
func TestCodexClientItemsSkipPairing(t *testing.T) {
	const delegation = `{"type":"function_call_output","namespace":"codex_app","name":"send_message_to_thread","output":"private prompt"}`
	const automation = `{"type":"function_call_output","namespace":"codex_app","name":"automation_update","call_id":null,"output":[{"type":"input_text","text":"x"}]}`
	for _, tc := range []struct {
		name, body string
		accept     bool
	}{
		{"delegation alone", `{"input":[` + delegation + `]}`, true},
		{"automation after history", `{"input":[{"type":"function_call","call_id":"a"},{"type":"function_call_output","call_id":"a","output":"ok"},` + automation + `]}`, true},
		{"unknown codex tool", `{"input":[{"type":"custom_tool_call_output","namespace":"codex_future","name":"x","output":"y"}]}`, true},
		{"with real missing call id", `{"input":[` + delegation + `,{"type":"function_call_output","output":"x"}]}`, false},
		{"duplicate members", `{"input":[{"type":"function_call_output","namespace":"codex_app","name":"create_thread","output":"a","output":"b"}]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			log, entries := newObservedLogger(t)
			require.Equal(t, tc.accept, ValidateOpenAIFunctionCallOutput(c, []byte(tc.body), log), w.Body.String())
			for _, entry := range entries.All() {
				require.NotContains(t, entry.ContextMap(), "output")
			}
		})
	}
}
