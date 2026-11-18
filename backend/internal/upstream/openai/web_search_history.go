package openai

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const webSearchHistoryTool = `{"type":"web_search","external_web_access":false}`

// EnsureWebSearchToolForHistory 为 hosted 搜索历史补齐上游要求的工具声明。
// 调用方限定 OpenAI OAuth Responses 请求，Compact 独立端点使用自己的格式。
// @project-doc docs/interfaces/openai_upstream.md#openai_web_search_history
func EnsureWebSearchToolForHistory(body []byte, responsesLite bool) ([]byte, bool, error) {
	if !bytes.Contains(body, []byte(`"web_search_call"`)) {
		return body, false, nil
	}
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return body, false, nil
	}
	items := input.Array()
	tools := gjson.GetBytes(body, "tools")
	hasHistory := false
	hasDeclaredTools := len(tools.Array()) > 0
	additionalIndex := -1
	invalidAdditionalTools := false
	for index, item := range items {
		switch strings.TrimSpace(item.Get("type").String()) {
		case "web_search_call":
			hasHistory = true
		case "additional_tools":
			additional := item.Get("tools")
			if webSearchToolsDeclared(additional) {
				return body, false, nil
			}
			hasDeclaredTools = hasDeclaredTools || len(additional.Array()) > 0
			invalidAdditionalTools = invalidAdditionalTools || (additional.Exists() && additional.Type != gjson.Null && !additional.IsArray())
			if additionalIndex < 0 {
				additionalIndex = index
			}
		}
	}
	if !hasHistory || webSearchToolsDeclared(tools) {
		return body, false, nil
	}
	if tools.Exists() && tools.Type != gjson.Null && !tools.IsArray() {
		return body, false, fmt.Errorf("web search history requires tools to be an array")
	}
	if invalidAdditionalTools {
		return body, false, fmt.Errorf("web search history requires additional_tools.tools to be an array")
	}
	// 无工具请求只有在允许固定为 none 时补声明，防止 required 触发新工具调用。
	if !hasDeclaredTools && !webSearchHistoryAllowsNoTools(gjson.GetBytes(body, "tool_choice")) {
		return body, false, nil
	}

	path := "tools"
	var replacement []byte
	switch {
	case !responsesLite:
		replacement = insertWebSearchHistoryJSON(tools.Array(), len(tools.Array()), webSearchHistoryTool)
	case additionalIndex >= 0:
		path = "input." + strconv.Itoa(additionalIndex) + ".tools"
		additional := items[additionalIndex].Get("tools").Array()
		replacement = insertWebSearchHistoryJSON(additional, len(additional), webSearchHistoryTool)
	default:
		path = "input"
		index := len(items)
		if index > 0 && strings.TrimSpace(items[index-1].Get("type").String()) == "compaction_trigger" {
			index--
		}
		declaration := `{"type":"additional_tools","role":"developer","tools":[` + webSearchHistoryTool + `]}`
		replacement = insertWebSearchHistoryJSON(items, index, declaration)
	}
	updated, err := sjson.SetRawBytes(body, path, replacement)
	if err != nil {
		return body, false, fmt.Errorf("declare web search history tool: %w", err)
	}
	if !hasDeclaredTools {
		updated, err = sjson.SetBytes(updated, "tool_choice", "none")
		if err != nil {
			return body, false, fmt.Errorf("disable tools for search history replay: %w", err)
		}
	}
	return updated, true, nil
}

// webSearchToolsDeclared 同时识别当前搜索工具和已有的 preview 声明。
func webSearchToolsDeclared(tools gjson.Result) bool {
	for _, tool := range tools.Array() {
		if strings.HasPrefix(strings.TrimSpace(tool.Get("type").String()), "web_search") {
			return true
		}
	}
	return false
}

func webSearchHistoryAllowsNoTools(choice gjson.Result) bool {
	if !choice.Exists() || choice.Type == gjson.Null {
		return true
	}
	if choice.Type != gjson.String {
		return false
	}
	switch strings.TrimSpace(choice.String()) {
	case "", "auto", "none":
		return true
	default:
		return false
	}
}

// insertWebSearchHistoryJSON 直接复制 JSON 项，历史中的大整数和密文按原字节写回。
func insertWebSearchHistoryJSON(items []gjson.Result, index int, inserted string) []byte {
	out := []byte{'['}
	for position := 0; position <= len(items); position++ {
		if position > 0 {
			out = append(out, ',')
		}
		switch {
		case position < index:
			out = append(out, items[position].Raw...)
		case position == index:
			out = append(out, inserted...)
		default:
			out = append(out, items[position-1].Raw...)
		}
	}
	return append(out, ']')
}
