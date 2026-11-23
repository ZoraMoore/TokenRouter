package requeststate

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const (
	automationBootstrapPrompt = "Review the project and report any important changes."
	delegationEnvelope        = `<codex_delegation><source_thread_id>thread-1</source_thread_id><input>do the work</input></codex_delegation>`
)

func TestNormalizeCodexAutomationBootstrapSupportedLastRunValues(t *testing.T) {
	tests := []struct {
		name    string
		lastRun string
		crlf    bool
	}{
		{name: "never", lastRun: "never"},
		{name: "timestamp", lastRun: "2026-09-01T02:06:34.536Z (1788228394536)"},
		{name: "crlf", lastRun: "never", crlf: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := codexAutomationBootstrap("wiki-maintenance", tt.lastRun, automationBootstrapPrompt)
			if tt.crlf {
				output = strings.ReplaceAll(output, "\n", "\r\n")
			}
			got, changed := NormalizeCodexAutomationBootstrap(codexAutomationBootstrapBody(t, output, ""))
			require.True(t, changed)
			require.Equal(t, "message", gjson.GetBytes(got, "input.0.type").String())
			require.Equal(t, "user", gjson.GetBytes(got, "input.0.role").String())
			require.Equal(t, output, gjson.GetBytes(got, "input.0.content.0.text").String())
			require.False(t, gjson.GetBytes(got, "input.0.call_id").Exists())
		})
	}
}

func TestNormalizeCodexAutomationBootstrapHeartbeat(t *testing.T) {
	output := `<heartbeat><automation_id>wiki</automation_id></heartbeat>`
	got, changed := NormalizeCodexAutomationBootstrap(codexAutomationBootstrapBody(t, output, ""))
	require.True(t, changed)
	require.Equal(t, "message", gjson.GetBytes(got, "input.0.type").String())
	require.Equal(t, "user", gjson.GetBytes(got, "input.0.role").String())
	require.Equal(t, output, gjson.GetBytes(got, "input.0.content.0.text").String())
	require.False(t, gjson.GetBytes(got, "input.0.call_id").Exists())

	again, changedAgain := NormalizeCodexAutomationBootstrap(got)
	require.False(t, changedAgain)
	require.Equal(t, got, again)
}

func TestNormalizeCodexAutomationBootstrapRejectsUnsafeShapes(t *testing.T) {
	validOutput := codexAutomationBootstrap("wiki", "never", automationBootstrapPrompt)
	tests := []struct {
		name string
		body []byte
	}{
		{
			name: "ordinary missing call id",
			body: []byte(`{"model":"gpt-5","input":[{"type":"function_call_output","namespace":"codex_app","name":"other","output":` + mustJSON(t, validOutput) + `}]}`),
		},
		{
			name: "tui namespace",
			body: []byte(`{"model":"gpt-5","input":[{"type":"function_call_output","namespace":"codex_tui","name":"automation_update","output":` + mustJSON(t, validOutput) + `}]}`),
		},
		{
			name: "valid call id",
			body: codexAutomationBootstrapBody(t, validOutput, `,"call_id":"call-1"`),
		},
		{
			name: "previous response",
			body: []byte(`{"model":"gpt-5","previous_response_id":"resp-1","input":[{"type":"function_call_output","namespace":"codex_app","name":"automation_update","output":` + mustJSON(t, validOutput) + `}]}`),
		},
		{
			name: "real call context",
			body: []byte(`{"model":"gpt-5","input":[{"type":"function_call_output","namespace":"codex_app","name":"automation_update","output":` + mustJSON(t, validOutput) + `},{"type":"function_call","call_id":"call-1"}]}`),
		},
		{
			name: "mismatched memory id",
			body: codexAutomationBootstrapBody(t, strings.Replace(validOutput, "/wiki/memory.md", "/other/memory.md", 1), ""),
		},
		{
			name: "unsafe automation id",
			body: codexAutomationBootstrapBody(t, codexAutomationBootstrap("../wiki", "never", automationBootstrapPrompt), ""),
		},
		{
			name: "invalid timestamp",
			body: codexAutomationBootstrapBody(t, codexAutomationBootstrap("wiki", "yesterday", automationBootstrapPrompt), ""),
		},
		{
			name: "mismatched timestamp epoch",
			body: codexAutomationBootstrapBody(t, codexAutomationBootstrap("wiki", "2026-09-01T02:06:34.536Z (1)", automationBootstrapPrompt), ""),
		},
		{
			name: "missing separator",
			body: codexAutomationBootstrapBody(t, strings.Replace(validOutput, "\n\n"+automationBootstrapPrompt, "\n"+automationBootstrapPrompt, 1), ""),
		},
		{
			name: "empty prompt",
			body: codexAutomationBootstrapBody(t, codexAutomationBootstrap("wiki", "never", " \n"), ""),
		},
		{
			name: "mixed missing call id output",
			body: []byte(`{"model":"gpt-5","input":[{"type":"function_call_output","namespace":"codex_app","name":"automation_update","output":` + mustJSON(t, validOutput) + `},{"type":"computer_call_output","output":"done"}]}`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := NormalizeCodexAutomationBootstrap(tt.body)
			require.False(t, changed)
			require.Equal(t, tt.body, got)
		})
	}
}

func TestNormalizeCodexAutomationBootstrapRejectsUnsafeHeartbeatShapes(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{name: "arbitrary tool output", output: `<result><automation_id>wiki</automation_id></result>`},
		{name: "root attribute", output: `<heartbeat status="ok"><automation_id>wiki</automation_id></heartbeat>`},
		{name: "namespaced root", output: `<heartbeat xmlns="urn:codex"><automation_id>wiki</automation_id></heartbeat>`},
		{name: "extra child", output: `<heartbeat><automation_id>wiki</automation_id><status>ok</status></heartbeat>`},
		{name: "nested id content", output: `<heartbeat><automation_id><value>wiki</value></automation_id></heartbeat>`},
		{name: "padded id", output: `<heartbeat><automation_id> wiki </automation_id></heartbeat>`},
		{name: "unsafe id", output: `<heartbeat><automation_id>../wiki</automation_id></heartbeat>`},
		{name: "comment", output: `<heartbeat><!-- ok --><automation_id>wiki</automation_id></heartbeat>`},
		{name: "trailing content", output: `<heartbeat><automation_id>wiki</automation_id></heartbeat>extra`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := codexAutomationBootstrapBody(t, tt.output, "")
			got, changed := NormalizeCodexAutomationBootstrap(body)
			require.False(t, changed)
			require.Equal(t, body, got)
		})
	}
}

func TestNormalizeCodexAutomationBootstrapPreservesOrderAndIsIdempotent(t *testing.T) {
	output := codexAutomationBootstrap("wiki", "never", automationBootstrapPrompt)
	body := []byte(`{"model":"gpt-5","input":[{"type":"message","role":"user","content":"before"},{"type":"function_call_output","namespace":"codex_app","name":"automation_update","output":` + mustJSON(t, output) + `},{"type":"message","role":"user","content":"after"}]}`)

	got, changed := NormalizeCodexAutomationBootstrap(body)
	require.True(t, changed)
	require.Equal(t, "before", gjson.GetBytes(got, "input.0.content").String())
	require.Equal(t, output, gjson.GetBytes(got, "input.1.content.0.text").String())
	require.Equal(t, "after", gjson.GetBytes(got, "input.2.content").String())

	again, changedAgain := NormalizeCodexAutomationBootstrap(got)
	require.False(t, changedAgain)
	require.Equal(t, got, again)
}

func TestNormalizeCodexDelegationBootstrapSupportedTools(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		tool      string
		callID    string
	}{
		{name: "app create missing call id", namespace: "codex_app", tool: "create_thread"},
		{name: "app send empty call id", namespace: "codex_app", tool: "send_message_to_thread", callID: `,"call_id":""`},
		{name: "tui create whitespace call id", namespace: "codex_tui", tool: "create_thread", callID: `,"call_id":"  "`},
		{name: "tui send missing call id", namespace: "codex_tui", tool: "send_message_to_thread"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5","input":[{"type":"function_call_output","namespace":"` + tt.namespace + `","name":"` + tt.tool + `","output":"` + delegationEnvelope + `"` + tt.callID + `}]}`)
			got, changed := NormalizeCodexDelegationBootstrap(body)
			require.True(t, changed)
			require.Equal(t, "message", gjson.GetBytes(got, "input.0.type").String())
			require.Equal(t, "user", gjson.GetBytes(got, "input.0.role").String())
			require.Equal(t, delegationEnvelope, gjson.GetBytes(got, "input.0.content.0.text").String())
			require.False(t, gjson.GetBytes(got, "input.0.call_id").Exists())
		})
	}
}

func TestNormalizeCodexDelegationBootstrapRejectsUnsafeShapes(t *testing.T) {
	tests := []struct {
		name  string
		item  string
		extra string
	}{
		{name: "ordinary missing call id", item: `{"type":"function_call_output","name":"other","namespace":"codex_app","output":"` + delegationEnvelope + `"}`},
		{name: "valid call id", item: `{"type":"function_call_output","call_id":"call-1","name":"create_thread","namespace":"codex_app","output":"` + delegationEnvelope + `"}`},
		{name: "ambiguous call context", item: `{"type":"function_call_output","name":"create_thread","namespace":"codex_app","output":"` + delegationEnvelope + `"},{"type":"function_call"}`},
		{name: "ambiguous built-in call context", item: `{"type":"function_call_output","name":"create_thread","namespace":"codex_app","output":"` + delegationEnvelope + `"},{"type":"computer_call"}`},
		{name: "mixed missing call id output", item: `{"type":"function_call_output","name":"create_thread","namespace":"codex_app","output":"` + delegationEnvelope + `"},{"type":"computer_call_output","output":"done"}`},
		{name: "mixed tool search output", item: `{"type":"function_call_output","name":"create_thread","namespace":"codex_app","output":"` + delegationEnvelope + `"},{"type":"tool_search_output","output":"done"}`},
		{name: "mixed invalid delegation output", item: `{"type":"function_call_output","name":"create_thread","namespace":"codex_app","output":"` + delegationEnvelope + `"},{"type":"function_call_output","name":"create_thread","namespace":"codex_app","output":"not an envelope"}`},
		{name: "non string call id", item: `{"type":"function_call_output","call_id":42,"name":"create_thread","namespace":"codex_app","output":"` + delegationEnvelope + `"}`},
		{name: "padded type", item: `{"type":" function_call_output ","name":"create_thread","namespace":"codex_app","output":"` + delegationEnvelope + `"}`},
		{name: "padded name", item: `{"type":"function_call_output","name":" create_thread ","namespace":"codex_app","output":"` + delegationEnvelope + `"}`},
		{name: "padded namespace", item: `{"type":"function_call_output","name":"create_thread","namespace":" codex_app ","output":"` + delegationEnvelope + `"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5","input":[` + tt.item + `]` + tt.extra + `}`)
			got, changed := NormalizeCodexDelegationBootstrap(body)
			require.False(t, changed)
			require.Equal(t, body, got)
		})
	}
}

func TestNormalizeCodexDelegationBootstrapWithHistoricalContext(t *testing.T) {
	body := []byte(`{"model":"gpt-5","previous_response_id":"resp-1","input":[{"type":"function_call_output","namespace":"codex_app","name":"send_message_to_thread","output":"` + delegationEnvelope + `"},{"type":"function_call","call_id":"call-1"},{"type":"function_call_output","call_id":"call-1","output":"done"},{"type":"item_reference","id":"item-1"},{"type":"computer_call","call_id":"call-2"}]}`)

	got, changed := NormalizeCodexDelegationBootstrap(body)
	require.True(t, changed)
	require.Equal(t, "message", gjson.GetBytes(got, "input.0.type").String())
	require.Equal(t, "user", gjson.GetBytes(got, "input.0.role").String())
	require.Equal(t, "resp-1", gjson.GetBytes(got, "previous_response_id").String())
	require.Equal(t, "call-1", gjson.GetBytes(got, "input.2.call_id").String())
	require.Equal(t, "item-1", gjson.GetBytes(got, "input.3.id").String())
}

// TestNormalizeCodexDelegationDesktopHistory 核对同一轮历史里各类记录的身份和原始内容。
func TestNormalizeCodexDelegationDesktopHistory(t *testing.T) {
	const prefix = `{"type":"function_call_output","namespace":"codex_app","name":"send_message_to_thread",`
	for name, history := range map[string]string{
		"terminal":        `{"type":"function_call_output","namespace":"agent","name":"terminal_info","output":"cwd"}`,
		"hosted search":   `{"type":"tool_search_call","call_id":null,"execution":"server","arguments":{}}`,
		"hosted result":   `{"type":"tool_search_output","call_id":null,"execution":"server","tools":[]}`,
		"web search":      `{"type":"web_search_call","id":"ws_1","status":"completed"}`,
		"numbered result": prefix + `"call_id":"call_send","output":"` + delegationEnvelope + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			body := []byte(`{"input":[` + history + `,` + prefix + `"output":"` + delegationEnvelope + `"}]}`)
			got, changed := NormalizeCodexDelegationBootstrap(body)
			require.True(t, changed)
			require.JSONEq(t, history, gjson.GetBytes(got, "input.0").Raw)
			require.Equal(t, delegationEnvelope, gjson.GetBytes(got, "input.1.content.0.text").Str)
		})
	}
}

// TestNormalizeCodexBootstrapMixedInputs 核对自动化首轮里的桌面上下文和跨任务消息。
func TestNormalizeCodexBootstrapMixedInputs(t *testing.T) {
	const heartbeat = "<heartbeat><automation_id>wiki</automation_id></heartbeat>"
	body := []byte(`{"input":[
		{"type":"function_call_output","namespace":"agent","name":"browser_tabs","output":"tab"},
		{"type":"function_call_output","namespace":"codex_tui","name":"create_thread","output":"` + delegationEnvelope + `"},
		{"type":"function_call_output","namespace":"codex_app","name":"automation_update","output":[{"type":"input_text","text":"` + heartbeat + `"}]}]}`)
	normalized, changed := NormalizeCodexAutomationBootstrap(body)
	require.True(t, changed)
	normalized, changed = NormalizeCodexDelegationBootstrap(normalized)
	require.True(t, changed)
	require.Equal(t, "agent", gjson.GetBytes(normalized, "input.0.namespace").Str)
	require.Equal(t, delegationEnvelope, gjson.GetBytes(normalized, "input.1.content.0.text").Str)
	require.Equal(t, heartbeat, gjson.GetBytes(normalized, "input.2.content.0.text").Str)
}

// TestNormalizeCodexDelegationContentItems 保留完整委派文本，含图片、未知字段或角色冲突的条目留给入站校验。
func TestNormalizeCodexDelegationContentItems(t *testing.T) {
	const text = `{"type":"input_text","text":"` + delegationEnvelope + `"}`
	for _, tc := range []struct {
		name, output, fields, want string
	}{
		{"single", `[` + text + `]`, "", delegationEnvelope},
		{"multiple", `[{"type":"input_text","text":"<codex_delegation>"},{"type":"input_text","text":"<source_thread_id>t</source_thread_id><input>ok</input></codex_delegation>"}]`, "", "<codex_delegation>\n<source_thread_id>t</source_thread_id><input>ok</input></codex_delegation>"},
		{"image", `[` + text + `,{"type":"input_image","image_url":"redacted"}]`, "", ""},
		{"extra field", `[{"type":"input_text","text":"` + delegationEnvelope + `","other":true}]`, "", ""},
		{"empty", `[]`, "", ""},
		{"plain text", `[{"type":"input_text","text":"done"}]`, "", ""},
		{"explicit null id", `[` + text + `]`, `,"call_id":null`, ""},
		{"conflicting role", `"` + delegationEnvelope + `"`, `,"role":"system"`, ""},
		{"conflicting content", `"` + delegationEnvelope + `"`, `,"content":[]`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"input":[{"type":"function_call_output","namespace":"codex_app","name":"send_message_to_thread","output":` + tc.output + tc.fields + `}]}`)
			got, changed := NormalizeCodexDelegationBootstrap(body)
			require.Equal(t, tc.want != "", changed)
			if !changed {
				require.Equal(t, body, got)
				return
			}
			require.Equal(t, tc.want, gjson.GetBytes(got, "input.0.content.0.text").Str)
		})
	}
}

func TestNormalizeCodexDelegationBootstrapPreviousResponseID(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		changed bool
	}{
		{name: "absent", changed: true},
		{name: "blank string", value: `,"previous_response_id":"  "`, changed: true},
		{name: "non-empty string", value: `,"previous_response_id":"resp-1"`, changed: true},
		{name: "null", value: `,"previous_response_id":null`},
		{name: "number", value: `,"previous_response_id":42`},
		{name: "boolean", value: `,"previous_response_id":false`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5","input":[{"type":"function_call_output","namespace":"codex_app","name":"create_thread","output":"` + delegationEnvelope + `"}]` + tt.value + `}`)
			got, changed := NormalizeCodexDelegationBootstrap(body)
			require.Equal(t, tt.changed, changed)
			if !tt.changed {
				require.Equal(t, body, got)
			}
		})
	}
}

func TestNormalizeCodexDelegationBootstrapRejectsDuplicateMembers(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "top-level previous response id",
			body: `{"model":"gpt-5","previous_response_id":"","previous_response_id":"resp-1","input":[{"type":"function_call_output","namespace":"codex_app","name":"create_thread","output":"` + delegationEnvelope + `"}]}`,
		},
		{
			name: "candidate discriminator",
			body: `{"model":"gpt-5","input":[{"type":"message","type":"function_call_output","namespace":"codex_app","name":"create_thread","output":"` + delegationEnvelope + `"}]}`,
		},
		{
			name: "candidate call id",
			body: `{"model":"gpt-5","input":[{"type":"function_call_output","namespace":"codex_app","name":"create_thread","call_id":"call-1","call_id":"","output":"` + delegationEnvelope + `"}]}`,
		},
		{
			name: "candidate output",
			body: `{"model":"gpt-5","input":[{"type":"function_call_output","namespace":"codex_app","name":"create_thread","output":"not a delegation","output":"` + delegationEnvelope + `"}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(tt.body)
			got, changed := NormalizeCodexDelegationBootstrap(body)
			require.False(t, changed)
			require.Equal(t, body, got)
		})
	}
}

func TestNormalizeCodexDelegationBootstrapPreservesExactNumbers(t *testing.T) {
	body := []byte(`{"model":"gpt-5","metadata":{"integer":9007199254740993},"input":[{"type":"function_call_output","namespace":"codex_app","name":"create_thread","output":"` + delegationEnvelope + `"}]}`)
	got, changed := NormalizeCodexDelegationBootstrap(body)
	require.True(t, changed)
	require.Equal(t, "9007199254740993", gjson.GetBytes(got, "metadata.integer").Raw)
}

func TestNormalizeCodexDelegationBootstrapRequiresCompleteEnvelope(t *testing.T) {
	malformed := []string{
		`<codex_delegation><source_thread_id>thread-1</source_thread_id></codex_delegation>`,
		`<codex_delegation><source_thread_id></source_thread_id><input>work</input></codex_delegation>`,
		`<codex_delegation><source_thread_id>thread-1</source_thread_id><input> </input></codex_delegation>`,
		`prefix<codex_delegation><source_thread_id>thread-1</source_thread_id><input>work</input></codex_delegation>`,
		`<codex_delegation><source_thread_id>thread-1</source_thread_id><input>work</input></codex_delegation>suffix`,
		`<codex_delegation><source_thread_id>thread-1</source_thread_id><input>work</input><extra>x</extra></codex_delegation>`,
		`<codex_delegation><source_thread_id>thread-1</source_thread_id><input><nested>work</nested></input></codex_delegation>`,
		`<x:codex_delegation xmlns:x="urn:test"><source_thread_id>thread-1</source_thread_id><input>work</input></x:codex_delegation>`,
		`<x:codex_delegation><source_thread_id>thread-1</source_thread_id><input>work</input></x:codex_delegation>`,
		`<codex_delegation xmlns="urn:test"><source_thread_id>thread-1</source_thread_id><input>work</input></codex_delegation>`,
		`<codex_delegation xmlns:x="urn:test"><x:source_thread_id>thread-1</x:source_thread_id><input>work</input></codex_delegation>`,
		`<codex_delegation><x:source_thread_id>thread-1</x:source_thread_id><input>work</input></codex_delegation>`,
		`<codex_delegation xmlns=""><source_thread_id>thread-1</source_thread_id><input>work</input></codex_delegation>`,
	}
	for _, envelope := range malformed {
		body := []byte(`{"model":"gpt-5","input":[{"type":"function_call_output","namespace":"codex_app","name":"create_thread","output":` + mustJSON(t, envelope) + `}]}`)
		got, changed := NormalizeCodexDelegationBootstrap(body)
		require.False(t, changed, envelope)
		require.Equal(t, body, got)
	}
}

func TestNormalizeCodexDelegationBootstrapPreservesOrderAndIsIdempotent(t *testing.T) {
	body := []byte(`{"model":"gpt-5","input":[{"type":"message","role":"user","content":"before"},{"type":"function_call_output","namespace":"codex_tui","name":"send_message_to_thread","output":"` + delegationEnvelope + `"},{"type":"message","role":"user","content":"after"}]}`)
	got, changed := NormalizeCodexDelegationBootstrap(body)
	require.True(t, changed)
	require.Equal(t, "before", gjson.GetBytes(got, "input.0.content").String())
	require.Equal(t, delegationEnvelope, gjson.GetBytes(got, "input.1.content.0.text").String())
	require.Equal(t, "after", gjson.GetBytes(got, "input.2.content").String())

	again, changedAgain := NormalizeCodexDelegationBootstrap(got)
	require.False(t, changedAgain)
	require.Equal(t, got, again)
}

func codexAutomationBootstrap(automationID, lastRun, prompt string) string {
	return "Automation: Scheduled project review\n" +
		"Automation ID: " + automationID + "\n" +
		"Automation memory: $CODEX_HOME/automations/" + automationID + "/memory.md\n" +
		"Last run: " + lastRun + "\n\n" + prompt
}

func codexAutomationBootstrapBody(t *testing.T, output, callID string) []byte {
	t.Helper()
	return []byte(`{"model":"gpt-5","input":[{"type":"function_call_output","namespace":"codex_app","name":"automation_update","output":` +
		mustJSON(t, output) + callID + `}]}`)
}

func mustJSON(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return string(encoded)
}
