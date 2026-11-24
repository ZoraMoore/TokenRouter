package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestIsCodexClientItem(t *testing.T) {
	for _, tc := range []struct {
		name, item string
		want       bool
	}{
		{"delegation missing id", `{"type":"function_call_output","namespace":"codex_app","name":"send_message_to_thread","output":"x"}`, true},
		{"empty id", `{"type":"function_call_output","namespace":"codex_app","name":"automation_update","call_id":"","output":"x"}`, true},
		{"blank id", `{"type":"function_call_output","namespace":"codex_tui","name":"create_thread","call_id":"  ","output":"x"}`, true},
		{"null id", `{"type":"function_call_output","namespace":"codex_app","name":"automation_update","call_id":null,"output":[]}`, true},
		{"custom output", `{"type":"custom_tool_call_output","namespace":"codex_future","name":"feature.v2","output":"x"}`, true},
		{"mcp output", `{"type":"mcp_tool_call_output","namespace":"mcp:server","name":"tool-1","output":{"a":1}}`, true},
		{"item id allowed", `{"type":"function_call_output","id":"fco_1","status":"completed","namespace":"codex_app","name":"x","output":"x"}`, true},
		{"numbered", `{"type":"function_call_output","namespace":"codex_app","name":"x","call_id":"call_1","output":"x"}`, false},
		{"numeric id", `{"type":"function_call_output","namespace":"codex_app","name":"x","call_id":1,"output":"x"}`, false},
		{"agent namespace", `{"type":"function_call_output","namespace":"agent","name":"terminal_info","output":"x"}`, false},
		{"no namespace", `{"type":"function_call_output","name":"x","output":"x"}`, false},
		{"empty namespace", `{"type":"function_call_output","namespace":"","name":"x","output":"x"}`, false},
		{"no name", `{"type":"function_call_output","namespace":"codex_app","output":"x"}`, false},
		{"quoted name", `{"type":"function_call_output","namespace":"codex_app","name":"a\"b","output":"x"}`, false},
		{"role", `{"type":"function_call_output","namespace":"codex_app","name":"x","role":"user","output":"x"}`, false},
		{"content", `{"type":"function_call_output","namespace":"codex_app","name":"x","content":"c","output":"x"}`, false},
		{"no output", `{"type":"function_call_output","namespace":"codex_app","name":"x"}`, false},
		{"tool search output", `{"type":"tool_search_output","namespace":"codex_app","name":"x","output":"x"}`, false},
		{"call item", `{"type":"function_call","namespace":"codex_app","name":"x","arguments":"{}"}`, false},
		{"message", `{"type":"message","role":"user","content":"x"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, IsCodexClientItem(gjson.Parse(tc.item)))
			var decoded map[string]any
			require.NoError(t, json.Unmarshal([]byte(tc.item), &decoded))
			require.Equal(t, tc.want, IsCodexClientItemMap(decoded), "map variant must agree")
			if tc.want {
				require.True(t, IsStandaloneClientItem(gjson.Parse(tc.item)))
				require.True(t, IsStandaloneClientItemMap(decoded))
				require.False(t, RequiresToolPairing(gjson.Parse(tc.item)))
			}
		})
	}
}

func TestCodexClientItemsDoNotDriveToolContinuation(t *testing.T) {
	const item = `{"type":"function_call_output","namespace":"codex_app","name":"send_message_to_thread","output":"x"}`
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{"input":[`+item+`]}`), &body))
	require.False(t, NeedsToolContinuation(body))
	signals := AnalyzeToolContinuationSignals(body)
	require.False(t, signals.HasFunctionCallOutput)
	require.False(t, signals.HasFunctionCallOutputMissingCallID)

	validation := ValidateFunctionCallOutputContextBytes([]byte(`{"input":[` + item + `,{"type":"function_call","call_id":"a"},{"type":"function_call_output","call_id":"a","output":"ok"}]}`))
	require.True(t, validation.HasDesktopContext)
	require.False(t, validation.HasFunctionCallOutputMissingCallID)
	require.True(t, validation.HasItemReferenceForAllCallIDs)

	coverage := AnalyzeToolCallOutputContextCoverageBytes([]byte(`{"input":[` + item + `]}`))
	require.False(t, coverage.HasFunctionCallOutput)
}
