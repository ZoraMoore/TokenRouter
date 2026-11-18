package bridge

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// 混合目录与纯 custom 目录均需保留执行器，并支持工具调用完整往返。
func TestResponsesNamespacedCustomRoundTrip(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		name := "custom_only"
		if mixed {
			name = "mixed"
		}
		t.Run(name, func(t *testing.T) {
			children := []any{map[string]any{"type": "custom", "name": "exec", "format": map[string]any{"type": "grammar"}}}
			if mixed {
				children = append(children, map[string]any{"type": "function", "name": "wait", "parameters": map[string]any{"type": "object"}})
			}
			req := map[string]any{
				"tools":       []any{map[string]any{"type": "namespace", "name": "functions", "tools": children}},
				"tool_choice": map[string]any{"type": "custom", "namespace": "functions", "name": "exec"},
				"input": []any{
					map[string]any{"type": "custom_tool_call", "id": "ctc_1", "call_id": "c1", "namespace": "functions", "name": "exec", "input": "await tools.exec_command({cmd: 'pwd'})"},
					map[string]any{"type": "custom_tool_call_output", "call_id": "c1", "output": "/workspace"},
				},
			}
			mapping, changed, err := AdaptResponsesClientTools(req)
			require.NoError(t, err)
			require.True(t, changed)
			require.True(t, mapping.CustomTools["functions__exec"])
			require.Equal(t, ResponsesNamespaceName{Namespace: "functions", Name: "exec"}, mapping.NamespaceTools["functions__exec"])
			encoded, err := json.Marshal(req)
			require.NoError(t, err)
			require.Equal(t, "function", gjson.GetBytes(encoded, "tools.0.type").String())
			require.Equal(t, "functions__exec", gjson.GetBytes(encoded, "tools.0.name").String())
			require.Equal(t, "function", gjson.GetBytes(encoded, "tool_choice.type").String())
			require.Equal(t, "functions__exec", gjson.GetBytes(encoded, "tool_choice.name").String())
			require.Equal(t, "function_call", gjson.GetBytes(encoded, "input.0.type").String())
			require.Equal(t, "functions__exec", gjson.GetBytes(encoded, "input.0.name").String())
			require.Equal(t, "fc_1", gjson.GetBytes(encoded, "input.0.id").String())
			require.Equal(t, "await tools.exec_command({cmd: 'pwd'})", gjson.Parse(gjson.GetBytes(encoded, "input.0.arguments").String()).Get("input").String())
			require.Equal(t, "function_call_output", gjson.GetBytes(encoded, "input.1.type").String())
			require.False(t, gjson.GetBytes(encoded, "input.0.namespace").Exists())

			payload := []byte(`{"output":[{"type":"function_call","id":"fc_2","call_id":"c2","name":"functions__exec","arguments":"{\"input\":\"return 42\"}"}]}`)
			restored, changed, err := RestoreResponsesClientToolPayload(payload, mapping)
			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, "custom_tool_call", gjson.GetBytes(restored, "output.0.type").String())
			require.Equal(t, "exec", gjson.GetBytes(restored, "output.0.name").String())
			require.Equal(t, "functions", gjson.GetBytes(restored, "output.0.namespace").String())
			require.Equal(t, "ctc_2", gjson.GetBytes(restored, "output.0.id").String())
			require.Equal(t, "c2", gjson.GetBytes(restored, "output.0.call_id").String())
			require.Equal(t, "return 42", gjson.GetBytes(restored, "output.0.input").String())

			for _, withDeclarations := range []bool{false, true} {
				followup := map[string]any{"input": []any{map[string]any{"type": "custom_tool_call", "namespace": "functions", "name": "exec", "call_id": "c2", "input": "return 42"}}}
				var inheritedTools [][]any
				if withDeclarations {
					tools, ok := req["tools"].([]any)
					require.True(t, ok)
					inheritedTools = [][]any{tools}
				}
				again, changed, err := AdaptResponsesClientToolsWithInheritedMapping(followup, mapping, inheritedTools...)
				require.NoError(t, err)
				require.True(t, changed)
				require.True(t, again.CustomTools["functions__exec"])
				require.Equal(t, mapping.NamespaceTools["functions__exec"], again.NamespaceTools["functions__exec"])
				inputs, ok := followup["input"].([]any)
				require.True(t, ok)
				item, ok := inputs[0].(map[string]any)
				require.True(t, ok)
				require.Equal(t, "function_call", item["type"])
				require.Equal(t, "functions__exec", item["name"])
			}
		})
	}
}

// SSE 的调用项、自由文本输入事件和最终快照必须携带一致身份。
func TestResponsesNamespacedCustomStream(t *testing.T) {
	mapping := ResponsesClientToolMapping{CustomTools: map[string]bool{"functions__exec": true}, NamespaceTools: map[string]ResponsesNamespaceName{"functions__exec": {Namespace: "functions", Name: "exec"}}}
	restorer := NewResponsesClientToolStreamRestorer(mapping)
	frames := []string{
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"c1","name":"functions__exec","arguments":""}}`,
		`{"type":"response.function_call_arguments.done","output_index":0,"item_id":"fc_1","arguments":"{\"input\":\"return 42\"}"}`,
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"c1","name":"functions__exec","arguments":"{\"input\":\"return 42\"}"}}`,
		`{"type":"response.completed","response":{"output":[{"type":"function_call","id":"fc_1","call_id":"c1","name":"functions__exec","arguments":"{\"input\":\"return 42\"}"}]}}`,
	}
	for _, frame := range frames {
		events, _, err := restorer.RestoreEvent([]byte(frame))
		require.NoError(t, err)
		require.NotEmpty(t, events)
		for _, event := range events {
			typ := gjson.GetBytes(event, "type").String()
			switch typ {
			case "response.output_item.added", "response.output_item.done":
				require.Equal(t, "custom_tool_call", gjson.GetBytes(event, "item.type").String())
				require.Equal(t, "exec", gjson.GetBytes(event, "item.name").String())
				require.Equal(t, "functions", gjson.GetBytes(event, "item.namespace").String())
				require.Equal(t, "ctc_1", gjson.GetBytes(event, "item.id").String())
			case "response.custom_tool_call_input.done":
				require.Equal(t, "exec", gjson.GetBytes(event, "name").String())
				require.Equal(t, "return 42", gjson.GetBytes(event, "input").String())
				require.Equal(t, "ctc_1", gjson.GetBytes(event, "item_id").String())
			case "response.completed":
				require.Equal(t, "exec", gjson.GetBytes(event, "response.output.0.name").String())
				require.Equal(t, "functions", gjson.GetBytes(event, "response.output.0.namespace").String())
			}
		}
	}
}

// 相同平名或同一 namespace 中的不同工具类型不能静默覆盖。
func TestResponsesNamespacedCustomCollision(t *testing.T) {
	for _, extra := range []map[string]any{
		{"type": "function", "name": "functions__exec"},
		{"type": "namespace", "name": "functions", "tools": []any{map[string]any{"type": "function", "name": "exec"}}},
	} {
		req := map[string]any{"tools": []any{map[string]any{"type": "namespace", "name": "functions", "tools": []any{map[string]any{"type": "custom", "name": "exec"}}}, extra}}
		_, _, err := AdaptResponsesClientTools(req)
		require.Error(t, err)
	}
}
