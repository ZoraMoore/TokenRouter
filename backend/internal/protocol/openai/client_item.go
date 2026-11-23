package openai

import (
	"regexp"
	"strings"

	"github.com/tidwall/gjson"
)

// clientItemIdent 是独立条目 namespace 和 name 的合法格式，符合格式的值可以直接写进日志和来源标记。
var clientItemIdent = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)

// IsCodexClientItem 判断条目是否是 Codex 客户端借用工具输出格式注入的独立条目，
// 例如 codex_app 的跨任务消息和自动化提示。
//
// 条件是：类型为 function_call_output、custom_tool_call_output 或 mcp_tool_call_output，
// namespace 和 name 是合法标识符且 namespace 为 agent 以外的值，call_id 缺省、为空白字符串或 null，
// 没有 role 和 content 字段，带 output 字段。
// 工具结果靠 call_id 与调用配对，缺少 call_id 又带 namespace 的条目只能来自客户端注入，因此跳过配对。
// agent 命名空间的条目由 IsDesktopContext 判断。
func IsCodexClientItem(item gjson.Result) bool {
	if !item.IsObject() || !isClientItemOutputType(item.Get("type").String()) {
		return false
	}
	namespace, name := item.Get("namespace"), item.Get("name")
	if namespace.Type != gjson.String || name.Type != gjson.String || namespace.Str == "agent" ||
		!clientItemIdent.MatchString(namespace.Str) || !clientItemIdent.MatchString(name.Str) {
		return false
	}
	if item.Get("role").Exists() || item.Get("content").Exists() || !item.Get("output").Exists() {
		return false
	}
	id := item.Get("call_id")
	return !id.Exists() || id.Type == gjson.Null || (id.Type == gjson.String && strings.TrimSpace(id.Str) == "")
}

// IsCodexClientItemMap 对解码后的条目做 IsCodexClientItem 的判断。
func IsCodexClientItemMap(item map[string]any) bool {
	typ, _ := item["type"].(string)
	if !isClientItemOutputType(typ) {
		return false
	}
	namespace, okNamespace := item["namespace"].(string)
	name, okName := item["name"].(string)
	if !okNamespace || !okName || namespace == "agent" ||
		!clientItemIdent.MatchString(namespace) || !clientItemIdent.MatchString(name) {
		return false
	}
	_, hasRole := item["role"]
	_, hasContent := item["content"]
	_, hasOutput := item["output"]
	if hasRole || hasContent || !hasOutput {
		return false
	}
	id, exists := item["call_id"]
	if !exists || id == nil {
		return true
	}
	text, ok := id.(string)
	return ok && strings.TrimSpace(text) == ""
}

// IsStandaloneClientItem 判断条目是否是跳过工具配对的客户端条目，包括 agent 桌面上下文和 Codex 客户端独立条目。
func IsStandaloneClientItem(item gjson.Result) bool {
	return IsDesktopContext(item) || IsCodexClientItem(item)
}

// IsStandaloneClientItemMap 对解码后的条目做 IsStandaloneClientItem 的判断。
func IsStandaloneClientItemMap(item map[string]any) bool {
	return IsDesktopContextMap(item) || IsCodexClientItemMap(item)
}

// isClientItemOutputType 列出 Codex 投递独立条目时使用的输出类型。tool_search_output 由托管搜索的规则判断。
func isClientItemOutputType(typ string) bool {
	switch typ {
	case "function_call_output", "custom_tool_call_output", "mcp_tool_call_output":
		return true
	default:
		return false
	}
}
