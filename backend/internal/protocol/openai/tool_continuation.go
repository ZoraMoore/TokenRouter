package openai

import (
	"strings"

	"github.com/tidwall/gjson"

	"github.com/TokenFlux/TokenRouter/internal/protocol/wirejson"
)

// ToolContinuationSignals 在一次遍历中收集工具续接所需的 input 信号。
type ToolContinuationSignals struct {
	HasFunctionCallOutput              bool
	HasFunctionCallOutputMissingCallID bool
	HasToolCallContext                 bool
	HasItemReference                   bool
	HasItemReferenceForAllCallIDs      bool
	FunctionCallOutputCallIDs          []string
}

// FunctionCallOutputValidation 汇总 function_call_output 关联性校验结果。
type FunctionCallOutputValidation struct {
	HasDesktopContext   bool
	UnmatchedInputIndex int64

	HasFunctionCallOutput              bool
	HasToolCallContext                 bool
	HasFunctionCallOutputMissingCallID bool
	HasItemReferenceForAllCallIDs      bool
	// 缺编号时记录第一个需要客户端配对的输出，InputIndex 从零开始。
	MissingCallIDItemType   string
	MissingCallIDInputIndex int64
}

// ToolCallOutputContextCoverage 描述 input 中工具输出与可重建上下文的覆盖关系，
// 用于判断剥离 previous_response_id 后上游能否仅凭 input 重建工具续链。
type ToolCallOutputContextCoverage struct {
	HasFunctionCallOutput bool
	// ContextCoversAllCallIDs 表示每个工具输出的 call_id 都能在 input 内找到
	// 同 call_id 的工具调用上下文项或同 id 的 item_reference，且不存在缺失 call_id 的输出。
	// 任一输出无法由 input 自身重建时为 false，此时剥离 previous_response_id 会导致
	// 上游以 "No tool call found for function call output" 拒绝请求。
	ContextCoversAllCallIDs bool
}

// IsCodexToolCallContextItemType 判断 input item 是否是带 call_id 的工具调用上下文。
func IsCodexToolCallContextItemType(typ string) bool {
	switch strings.TrimSpace(typ) {
	case "tool_call",
		"function_call",
		"local_shell_call",
		"tool_search_call",
		"custom_tool_call",
		"mcp_tool_call":
		return true
	default:
		return false
	}
}

// IsCodexToolCallOutputItemType 判断 input item 是否是 Codex 工具输出。
func IsCodexToolCallOutputItemType(typ string) bool {
	switch strings.TrimSpace(typ) {
	case "function_call_output",
		"tool_search_output",
		"custom_tool_call_output",
		"mcp_tool_call_output":
		return true
	default:
		return false
	}
}

// NeedsToolContinuation 判定请求是否需要工具调用续链处理。
// 满足以下任一信号即视为续链：previous_response_id、input 内包含工具输出/item_reference、
// 或声明 tools/tool_choice。
func NeedsToolContinuation(reqBody map[string]any) bool {
	if reqBody == nil {
		return false
	}
	if HasNonEmptyString(reqBody["previous_response_id"]) {
		return true
	}
	if HasToolsSignal(reqBody) {
		return true
	}
	if HasToolChoiceSignal(reqBody) {
		return true
	}
	input, ok := reqBody["input"].([]any)
	if !ok {
		return false
	}
	for _, item := range input {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		itemType, _ := itemMap["type"].(string)
		if !IsDesktopContextMap(itemMap) && (IsCodexToolCallItemType(itemType) || itemType == "item_reference") {
			return true
		}
	}
	return false
}

// AnalyzeToolContinuationSignals 单次遍历 input，提取工具输出/工具调用上下文/item_reference 相关信号。
// FunctionCallOutput 字段覆盖 Codex 的所有工具输出
// （function_call_output/tool_search_output/custom_tool_call_output/mcp_tool_call_output）。
func AnalyzeToolContinuationSignals(reqBody map[string]any) ToolContinuationSignals {
	signals := ToolContinuationSignals{}
	if reqBody == nil {
		return signals
	}
	input, ok := reqBody["input"].([]any)
	if !ok {
		return signals
	}

	var callIDs map[string]struct{}
	var referenceIDs map[string]struct{}

	for _, item := range input {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		itemType, _ := itemMap["type"].(string)
		switch {
		case IsCodexToolCallContextItemType(itemType):
			callID, _ := itemMap["call_id"].(string)
			if strings.TrimSpace(callID) != "" {
				signals.HasToolCallContext = true
			}
		case IsCodexToolCallOutputItemType(itemType):
			if IsDesktopContextMap(itemMap) || IsHostedToolSearchOutputMap(itemMap) {
				continue
			}
			signals.HasFunctionCallOutput = true
			callID, _ := itemMap["call_id"].(string)
			callID = strings.TrimSpace(callID)
			if callID == "" {
				signals.HasFunctionCallOutputMissingCallID = true
				continue
			}
			if callIDs == nil {
				callIDs = make(map[string]struct{})
			}
			callIDs[callID] = struct{}{}
		case itemType == "item_reference":
			signals.HasItemReference = true
			idValue, _ := itemMap["id"].(string)
			idValue = strings.TrimSpace(idValue)
			if idValue == "" {
				continue
			}
			if referenceIDs == nil {
				referenceIDs = make(map[string]struct{})
			}
			referenceIDs[idValue] = struct{}{}
		}
	}

	if len(callIDs) == 0 {
		return signals
	}
	signals.FunctionCallOutputCallIDs = make([]string, 0, len(callIDs))
	allReferenced := len(referenceIDs) > 0
	for callID := range callIDs {
		signals.FunctionCallOutputCallIDs = append(signals.FunctionCallOutputCallIDs, callID)
		if allReferenced {
			if _, ok := referenceIDs[callID]; !ok {
				allReferenced = false
			}
		}
	}
	signals.HasItemReferenceForAllCallIDs = allReferenced
	return signals
}

// ValidateFunctionCallOutputContextBytes 检查每个客户端工具结果的编号和历史引用。
func ValidateFunctionCallOutputContextBytes(body []byte) FunctionCallOutputValidation {
	input := wirejson.ParseView(body).Get("input")
	if !input.IsArray() {
		return FunctionCallOutputValidation{}
	}
	return validateToolOutputItems(body, input)
}

func validateToolOutputItems(body []byte, input gjson.Result) FunctionCallOutputValidation {
	result := FunctionCallOutputValidation{}
	// 延迟分配以减少普通对话的开销。
	var outputs map[string]int64
	var contextIDs map[string]struct{}
	var names map[string]bool
	check := func(index, item gjson.Result) bool {
		kind := item.Get("type").String()
		switch {
		case IsCodexToolCallOutputItemType(kind):
			id := item.Get("call_id")
			callID := strings.TrimSpace(id.Str)
			if IsDesktopContext(item) {
				result.HasDesktopContext = true
				if names == nil {
					names = desktopToolNames(body)
				}
				if !names[item.Get("name").Str] {
					return true
				}
			}
			if kind == "tool_search_output" && item.Get("execution").Str == "server" && id.Type == gjson.Null && id.Exists() {
				return true
			}
			result.HasFunctionCallOutput = true
			if callID == "" {
				result.noteMissingHTTPCallID(index.Int(), kind)
			} else {
				if outputs == nil {
					outputs = map[string]int64{}
				}
				if _, exists := outputs[callID]; !exists {
					outputs[callID] = index.Int()
				}
			}
		case IsCodexToolCallContextItemType(kind), kind == "item_reference":
			callID := strings.TrimSpace(item.Get("call_id").Str)
			if kind == "item_reference" {
				callID = strings.TrimSpace(item.Get("id").Str)
			} else if callID != "" {
				result.HasToolCallContext = true
			}
			if callID != "" {
				if contextIDs == nil {
					contextIDs = map[string]struct{}{}
				}
				contextIDs[callID] = struct{}{}
			}
		}
		return true
	}
	if input.IsArray() {
		input.ForEach(check)
	} else if input.IsObject() {
		check(gjson.Result{}, input)
	}
	result.HasItemReferenceForAllCallIDs = len(outputs) > 0
	var unmatched bool
	for id, index := range outputs {
		if _, ok := contextIDs[id]; !ok {
			result.HasItemReferenceForAllCallIDs = false
			if !unmatched || index < result.UnmatchedInputIndex {
				result.UnmatchedInputIndex = index
			}
			unmatched = true
		}
	}
	return result
}

// AnalyzeToolCallOutputContextCoverageBytes 检查移除 previous_response_id 后能否恢复全部工具配对。
func AnalyzeToolCallOutputContextCoverageBytes(body []byte) ToolCallOutputContextCoverage {
	v := validateToolOutputItems(body, wirejson.ParseView(body).Get("input"))
	return ToolCallOutputContextCoverage{HasFunctionCallOutput: v.HasFunctionCallOutput, ContextCoversAllCallIDs: v.HasItemReferenceForAllCallIDs && !v.HasFunctionCallOutputMissingCallID}
}

// ValidateFunctionCallOutputContext 与 HTTP 入站使用相同的配对规则。
func ValidateFunctionCallOutputContext(reqBody map[string]any) FunctionCallOutputValidation {
	body, err := wirejson.Marshal(reqBody)
	if err != nil {
		return FunctionCallOutputValidation{}
	}
	return ValidateFunctionCallOutputContextBytes(body)
}

// noteMissingHTTPCallID 保存第一个无编号工具结果的原始位置。
func (v *FunctionCallOutputValidation) noteMissingHTTPCallID(index int64, itemType string) {
	if !v.HasFunctionCallOutputMissingCallID {
		v.HasFunctionCallOutputMissingCallID = true
		v.MissingCallIDItemType = strings.TrimSpace(itemType)
		v.MissingCallIDInputIndex = index
	}
}

// HasFunctionCallOutput 判断 input 是否包含任意 Codex 工具输出，用于触发续链校验。
// 此处同时处理 function_call_output 和其他 Codex 工具输出。
func HasFunctionCallOutput(reqBody map[string]any) bool {
	return AnalyzeToolContinuationSignals(reqBody).HasFunctionCallOutput
}

// HasItemReferenceForCallIDs 判断 item_reference.id 是否覆盖所有 call_id。
// 用于仅依赖引用项完成续链场景的校验。
func HasItemReferenceForCallIDs(reqBody map[string]any, callIDs []string) bool {
	if reqBody == nil || len(callIDs) == 0 {
		return false
	}
	input, ok := reqBody["input"].([]any)
	if !ok {
		return false
	}
	referenceIDs := make(map[string]struct{})
	for _, item := range input {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		itemType, _ := itemMap["type"].(string)
		if itemType != "item_reference" {
			continue
		}
		idValue, _ := itemMap["id"].(string)
		idValue = strings.TrimSpace(idValue)
		if idValue == "" {
			continue
		}
		referenceIDs[idValue] = struct{}{}
	}
	if len(referenceIDs) == 0 {
		return false
	}
	for _, callID := range callIDs {
		if _, ok := referenceIDs[strings.TrimSpace(callID)]; !ok {
			return false
		}
	}
	return true
}

// HasNonEmptyString 判断字段是否为非空字符串。
func HasNonEmptyString(value any) bool {
	stringValue, ok := value.(string)
	return ok && strings.TrimSpace(stringValue) != ""
}

// HasToolsSignal 判断 tools 字段是否存在且非空。
func HasToolsSignal(reqBody map[string]any) bool {
	raw, exists := reqBody["tools"]
	if !exists || raw == nil {
		return false
	}
	if tools, ok := raw.([]any); ok {
		return len(tools) > 0
	}
	return false
}

// HasToolChoiceSignal 判断 tool_choice 字段是否非空且非 nil。
func HasToolChoiceSignal(reqBody map[string]any) bool {
	raw, exists := reqBody["tool_choice"]
	if !exists || raw == nil {
		return false
	}
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value) != ""
	case map[string]any:
		return len(value) > 0
	default:
		return false
	}
}
