package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestEnsureWebSearchToolForHistory(t *testing.T) {
	const history = `{"type":"web_search_call","id":"ws_history","action":{"type":"search","query":"audit","opaque":9007199254740993}}`
	tests := []struct {
		name    string
		body    string
		lite    bool
		changed bool
		choice  string
		err     bool
	}{
		{name: "no history", body: `{"input":[{"type":"message","content":"web_search_call"}],"tools":[]}`},
		{name: "string input", body: `{"input":"web_search_call","tools":[]}`},
		{name: "missing tools", body: `{"input":[` + history + `]}`, changed: true, choice: "none"},
		{name: "null tools", body: `{"input":[` + history + `],"tools":null}`, changed: true, choice: "none"},
		{name: "empty tools auto", body: `{"input":[` + history + `],"tools":[],"tool_choice":"auto"}`, changed: true, choice: "none"},
		{name: "empty tools none", body: `{"input":[` + history + `],"tools":[],"tool_choice":"none"}`, changed: true, choice: "none"},
		{name: "null choice", body: `{"input":[` + history + `],"tool_choice":null}`, changed: true, choice: "none"},
		{name: "required without tools", body: `{"input":[` + history + `],"tool_choice":"required"}`},
		{name: "named choice without tools", body: `{"input":[` + history + `],"tool_choice":{"type":"function","name":"missing"}}`},
		{name: "function tools", body: `{"input":[` + history + `],"tools":[{"type":"function","name":"run","parameters":{"type":"object"}}],"tool_choice":"auto"}`, changed: true, choice: "auto"},
		{name: "required with caller tools", body: `{"input":[` + history + `],"tools":[{"type":"function","name":"run"}],"tool_choice":"required"}`, changed: true, choice: "required"},
		{name: "existing search", body: `{"input":[` + history + `],"tools":[{"type":"web_search","external_web_access":true}]}`},
		{name: "existing preview", body: `{"input":[` + history + `],"tools":[{"type":"web_search_preview_2025_03_11"}]}`},
		{name: "existing additional search", body: `{"input":[` + history + `,{"type":"additional_tools","tools":[{"type":"web_search"}]}]}`, lite: true},
		{name: "lite missing additional tools", body: `{"input":[` + history + `]}`, lite: true, changed: true, choice: "none"},
		{name: "lite empty additional tools", body: `{"input":[{"type":"additional_tools","role":"developer","tools":[]},` + history + `]}`, lite: true, changed: true, choice: "none"},
		{name: "lite null additional tools", body: `{"input":[{"type":"additional_tools","role":"developer","tools":null},` + history + `]}`, lite: true, changed: true, choice: "none"},
		{name: "lite caller namespaces", body: `{"input":[{"type":"additional_tools","role":"developer","tools":[{"type":"namespace","name":"functions","tools":[]}]},` + history + `],"tool_choice":"auto"}`, lite: true, changed: true, choice: "auto"},
		{name: "lite trigger remains last", body: `{"input":[` + history + `,{"type":"compaction_trigger"}],"tools":[]}`, lite: true, changed: true, choice: "none"},
		{name: "invalid top tools", body: `{"input":[` + history + `],"tools":{}}`, err: true},
		{name: "invalid additional tools", body: `{"input":[` + history + `,{"type":"additional_tools","tools":{}}]}`, lite: true, err: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			original := []byte(tc.body)
			result, changed, err := EnsureWebSearchToolForHistory(original, tc.lite)
			if tc.err {
				require.Error(t, err)
				require.False(t, changed)
				require.Equal(t, original, result)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.changed, changed)
			if !changed {
				require.Equal(t, original, result)
				return
			}
			require.True(t, json.Valid(result))
			require.Equal(t, history, gjson.GetBytes(result, `input.#(type=="web_search_call")`).Raw)
			require.Equal(t, tc.choice, gjson.GetBytes(result, "tool_choice").String())
			declarations := gjson.GetBytes(result, "tools").Array()
			if tc.lite {
				require.Equal(t, gjson.GetBytes(original, "tools").Raw, gjson.GetBytes(result, "tools").Raw)
				declarations = gjson.GetBytes(result, `input.#(type=="additional_tools").tools`).Array()
			}
			found := 0
			for _, tool := range declarations {
				if tool.Get("type").String() == "web_search" {
					found++
					require.Equal(t, gjson.False, tool.Get("external_web_access").Type)
				}
			}
			require.Equal(t, 1, found)
			items := gjson.GetBytes(result, "input").Array()
			if strings.Contains(tc.body, `"compaction_trigger"`) {
				require.Equal(t, "compaction_trigger", items[len(items)-1].Get("type").String())
			}
			again, changedAgain, err := EnsureWebSearchToolForHistory(result, tc.lite)
			require.NoError(t, err)
			require.False(t, changedAgain)
			require.Equal(t, result, again)
		})
	}
}

// 原始历史、推理密文和未知扩展字段用于后续回放，补声明时需要逐字保留。
func TestEnsureWebSearchToolForHistoryPreservesPayload(t *testing.T) {
	body := []byte(`{"input":[{"type":"reasoning","encrypted_content":"opaque-cipher","summary":[]},{"type":"web_search_call","action":{"type":"open_page","url":"https://example.test","sources":[{"url":"https://example.test/doc"}]}},{"type":"message","role":"assistant","phase":"commentary","content":"中文 <tag>"}],"opaque":{"large":9007199254740993},"tools":[{"type":"function","name":"lookup","parameters":{"type":"object","properties":{"value":{"const":9007199254740993}}}}]}`)
	updated, changed, err := EnsureWebSearchToolForHistory(body, false)
	require.NoError(t, err)
	require.True(t, changed)
	for _, path := range []string{"input", "opaque", "tools.0"} {
		require.Equal(t, gjson.GetBytes(body, path).Raw, gjson.GetBytes(updated, path).Raw, path)
	}
	require.False(t, gjson.GetBytes(updated, "tool_choice").Exists())
}
