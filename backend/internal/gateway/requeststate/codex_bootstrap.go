package requeststate

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/protocol/openai"
)

// NormalizeCodexDelegationBootstrap 把来自其他 Codex 任务的委派信封放回用户输入。
func NormalizeCodexDelegationBootstrap(body []byte) ([]byte, bool) {
	return normalizeCodexCallOutputBootstrap(body, true)
}

// NormalizeCodexAutomationBootstrap 读取自动化首轮的启动提示或心跳。
func NormalizeCodexAutomationBootstrap(body []byte) ([]byte, bool) {
	return normalizeCodexCallOutputBootstrap(body, false)
}

// normalizeCodexCallOutputBootstrap 原位替换信封，历史有歧义时交回入口校验。
func normalizeCodexCallOutputBootstrap(body []byte, delegation bool) ([]byte, bool) {
	if !HasUniqueJSONMembers(body) {
		return body, false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var request map[string]any
	if err := decoder.Decode(&request); err != nil {
		return body, false
	}
	if previousResponseID, exists := request["previous_response_id"]; exists {
		value, ok := previousResponseID.(string)
		if !ok || (!delegation && strings.TrimSpace(value) != "") {
			return body, false
		}
	}
	input, ok := request["input"].([]any)
	if !ok {
		return body, false
	}

	changed := false
	for i, raw := range input {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		typ := StringField(item, "type")
		if typ == "item_reference" {
			if !delegation || strings.TrimSpace(StringField(item, "id")) == "" {
				return body, false
			}
			continue
		}
		if !strings.HasSuffix(typ, "_call") && !IsResponsesCallOutputType(typ) {
			continue
		}
		// 带编号的结果归属历史调用，同名委派仍按自己的位置处理。
		if delegation && strings.TrimSpace(StringField(item, "call_id")) != "" {
			continue
		}
		if isCodexIndependentHistory(item, typ) {
			continue
		}
		output, valid := CodexBootstrapEnvelope(item)
		if !valid || !codexBootstrapHasEmptyID(item) {
			return body, false
		}
		// 同轮收到两种信封时，各自的处理阶段读取对应的条目。
		if delegation != IsCodexDelegationTool(StringField(item, "namespace"), StringField(item, "name")) {
			continue
		}
		input[i] = map[string]any{
			"type": "message",
			"role": "user",
			"content": []any{map[string]any{
				"type": "input_text",
				"text": output,
			}},
		}
		changed = true
	}
	if !changed {
		return body, false
	}
	normalized, err := json.Marshal(request)
	if err != nil {
		return body, false
	}
	return normalized, true
}

// codexBootstrapHasEmptyID 区分可省略的编号与显式的 null 或错误类型。
func codexBootstrapHasEmptyID(item map[string]any) bool {
	id, exists := item["call_id"]
	text, ok := id.(string)
	return !exists || (ok && strings.TrimSpace(text) == "")
}

// isCodexIndependentHistory 识别由桌面端注入或由上游独立执行的历史记录。
func isCodexIndependentHistory(item map[string]any, typ string) bool {
	switch typ {
	case "function_call_output":
		return openai.IsDesktopContextMap(item)
	case "tool_search_output":
		return openai.IsHostedToolSearchOutputMap(item)
	case "tool_search_call":
		id, exists := item["call_id"]
		return item["execution"] == "server" && exists && id == nil
	case "web_search_call", "file_search_call", "code_interpreter_call", "image_generation_call":
		_, hasCallID := item["call_id"]
		return !hasCallID
	default:
		return false
	}
}

func HasUniqueJSONMembers(body []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(body))
	if !ConsumeUniqueJSONValue(decoder) {
		return false
	}
	_, err := decoder.Token()
	return err == io.EOF
}

func ConsumeUniqueJSONValue(decoder *json.Decoder) bool {
	token, err := decoder.Token()
	if err != nil {
		return false
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return true
	}

	switch delim {
	case '{':
		members := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return false
			}
			key, ok := keyToken.(string)
			if !ok {
				return false
			}
			if _, duplicate := members[key]; duplicate {
				return false
			}
			members[key] = struct{}{}
			if !ConsumeUniqueJSONValue(decoder) {
				return false
			}
		}
		end, err := decoder.Token()
		return err == nil && end == json.Delim('}')
	case '[':
		for decoder.More() {
			if !ConsumeUniqueJSONValue(decoder) {
				return false
			}
		}
		end, err := decoder.Token()
		return err == nil && end == json.Delim(']')
	default:
		return false
	}
}

func IsResponsesCallOutputType(typ string) bool {
	return strings.HasSuffix(typ, "_call_output") || typ == "tool_search_output"
}

// CodexBootstrapEnvelope 校验桌面端的工具名、输出字段和信封，返回可以转发的文本。
func CodexBootstrapEnvelope(item map[string]any) (string, bool) {
	namespace := StringField(item, "namespace")
	name := StringField(item, "name")
	_, hasRole := item["role"]
	_, hasContent := item["content"]
	if item["type"] != "function_call_output" || hasRole || hasContent {
		return "", false
	}
	delegation := IsCodexDelegationTool(namespace, name)
	if !delegation && !IsCodexAutomationTool(namespace, name) {
		return "", false
	}
	output, ok := codexBootstrapText(item["output"])
	if !ok {
		return "", false
	}
	if delegation {
		return output, ValidCodexDelegationEnvelope(output)
	}
	return output, ValidCodexAutomationBootstrap(output) || ValidCodexAutomationHeartbeat(output)
}

// codexBootstrapText 按 Codex 的换行约定拼接文本块。其他内容留在原条目中。
func codexBootstrapText(output any) (string, bool) {
	switch value := output.(type) {
	case string:
		return value, true
	case []any:
		parts := make([]string, 0, len(value))
		for _, raw := range value {
			part, ok := raw.(map[string]any)
			if !ok || len(part) != 2 || part["type"] != "input_text" {
				return "", false
			}
			text, ok := part["text"].(string)
			if !ok {
				return "", false
			}
			parts = append(parts, text)
		}
		return strings.Join(parts, "\n"), len(parts) > 0
	default:
		return "", false
	}
}

func StringField(item map[string]any, key string) string {
	value, _ := item[key].(string)
	return value
}

func IsCodexDelegationTool(namespace, name string) bool {
	return (namespace == "codex_app" || namespace == "codex_tui") &&
		(name == "create_thread" || name == "send_message_to_thread")
}

// IsCodexAutomationTool 匹配桌面端投递自动化提示使用的工具名。
func IsCodexAutomationTool(namespace, name string) bool {
	return namespace == "codex_app" && name == "automation_update"
}

func ValidCodexAutomationBootstrap(value string) bool {
	normalized := strings.ReplaceAll(value, "\r\n", "\n")
	if strings.ContainsRune(normalized, '\r') {
		return false
	}
	lines := strings.Split(normalized, "\n")
	if len(lines) < 6 {
		return false
	}
	if _, ok := CodexAutomationHeaderValue(lines[0], "Automation: "); !ok {
		return false
	}
	automationID, ok := CodexAutomationHeaderValue(lines[1], "Automation ID: ")
	if !ok || !ValidCodexAutomationID(automationID) {
		return false
	}
	expectedMemory := "Automation memory: $CODEX_HOME/automations/" + automationID + "/memory.md"
	if lines[2] != expectedMemory {
		return false
	}
	lastRun, ok := CodexAutomationHeaderValue(lines[3], "Last run: ")
	if !ok || !ValidCodexAutomationLastRun(lastRun) || lines[4] != "" {
		return false
	}
	return strings.TrimSpace(strings.Join(lines[5:], "\n")) != ""
}

func CodexAutomationHeaderValue(line, prefix string) (string, bool) {
	if !strings.HasPrefix(line, prefix) {
		return "", false
	}
	value := strings.TrimPrefix(line, prefix)
	return value, value != "" && strings.TrimSpace(value) == value
}

func ValidCodexAutomationID(value string) bool {
	if value == "" || len(value) > 128 || value == "." || value == ".." {
		return false
	}
	for i := range len(value) {
		c := value[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' {
			continue
		}
		return false
	}
	return true
}

func ValidCodexAutomationLastRun(value string) bool {
	if value == "never" {
		return true
	}
	separator := strings.LastIndex(value, " (")
	if separator <= 0 || !strings.HasSuffix(value, ")") {
		return false
	}
	runAt, err := time.Parse(time.RFC3339Nano, value[:separator])
	if err != nil {
		return false
	}
	epochMillis, err := strconv.ParseInt(value[separator+2:len(value)-1], 10, 64)
	return err == nil && runAt.UnixMilli() == epochMillis
}

func ValidCodexAutomationHeartbeat(value string) bool {
	decoder := xml.NewDecoder(strings.NewReader(value))
	var rootSeen, automationIDSeen bool
	var automationID bytes.Buffer
	depth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			id := automationID.String()
			return rootSeen && automationIDSeen && depth == 0 &&
				strings.TrimSpace(id) == id && ValidCodexAutomationID(id)
		}
		if err != nil {
			return false
		}
		switch current := token.(type) {
		case xml.StartElement:
			depth++
			if current.Name.Space != "" || len(current.Attr) != 0 || depth > 2 {
				return false
			}
			if depth == 1 {
				if rootSeen || current.Name.Local != "heartbeat" {
					return false
				}
				rootSeen = true
			} else if automationIDSeen || current.Name.Local != "automation_id" {
				return false
			}
			automationIDSeen = depth == 2
		case xml.EndElement:
			if current.Name.Space != "" {
				return false
			}
			depth--
			if depth < 0 {
				return false
			}
		case xml.CharData:
			if depth == 2 {
				_, _ = automationID.Write(current)
			} else if len(bytes.TrimSpace(current)) != 0 {
				return false
			}
		case xml.Comment, xml.ProcInst, xml.Directive:
			return false
		}
	}
}

func ValidCodexDelegationEnvelope(value string) bool {
	decoder := xml.NewDecoder(strings.NewReader(value))
	var rootSeen, sourceSeen, inputSeen bool
	var childName string
	var childText bytes.Buffer
	depth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return rootSeen && depth == 0 && sourceSeen && inputSeen
		}
		if err != nil {
			return false
		}
		switch current := token.(type) {
		case xml.StartElement:
			depth++
			if current.Name.Space != "" || len(current.Attr) != 0 || (depth == 1 && current.Name.Local != "codex_delegation") || depth > 2 {
				return false
			}
			if depth == 1 {
				if rootSeen {
					return false
				}
				rootSeen = true
				continue
			}
			if current.Name.Local != "source_thread_id" && current.Name.Local != "input" {
				return false
			}
			childName = current.Name.Local
			childText.Reset()
		case xml.EndElement:
			if current.Name.Space != "" {
				return false
			}
			if depth == 2 {
				if current.Name.Local != childName || strings.TrimSpace(childText.String()) == "" {
					return false
				}
				if childName == "source_thread_id" {
					if sourceSeen {
						return false
					}
					sourceSeen = true
				} else {
					if inputSeen {
						return false
					}
					inputSeen = true
				}
				childName = ""
			}
			depth--
			if depth < 0 {
				return false
			}
		case xml.CharData:
			if depth == 2 {
				_, _ = childText.Write(current)
			} else if len(bytes.TrimSpace(current)) != 0 {
				return false
			}
		case xml.Comment:
			return false
		case xml.ProcInst, xml.Directive:
			return false
		}
	}
}
