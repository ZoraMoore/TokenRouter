package bridge

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ResponsesToAnthropicRequest converts a Responses API request into an
// Anthropic Messages request. This is the reverse of AnthropicToResponses and
// enables Anthropic platform groups to accept OpenAI Responses API requests
// by converting them to the native /v1/messages format before forwarding upstream.
func ResponsesToAnthropicRequest(req *ResponsesRequest) (*AnthropicRequest, error) {
	system, messages, err := convertResponsesInputToAnthropic(req.Instructions, req.Input)
	if err != nil {
		return nil, err
	}

	out := &AnthropicRequest{
		Model:       req.Model,
		Messages:    messages,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      req.Stream,
	}
	// OpenAI Priority/Fast 转为 Claude Fast 候选，beta header 由网关统一补齐。
	switch strings.ToLower(strings.TrimSpace(req.ServiceTier)) {
	case "priority", "fast":
		out.Speed = "fast"
	}

	if len(system) > 0 {
		out.System = system
	}

	// max_output_tokens → max_tokens
	if req.MaxOutputTokens != nil && *req.MaxOutputTokens > 0 {
		out.MaxTokens = *req.MaxOutputTokens
	}
	if out.MaxTokens == 0 {
		// Anthropic requires max_tokens; default to a sensible value.
		out.MaxTokens = 8192
	}

	// Convert tools
	if len(req.Tools) > 0 {
		out.Tools = convertResponsesToAnthropicTools(req.Tools)
	}

	// Convert tool_choice (reverse of convertAnthropicToolChoiceToResponses)
	if len(req.ToolChoice) > 0 {
		tc, err := convertResponsesToAnthropicToolChoice(req.ToolChoice)
		if err != nil {
			return nil, fmt.Errorf("convert tool_choice: %w", err)
		}
		out.ToolChoice = tc
	}

	// reasoning.effort → output_config.effort + thinking
	if req.Reasoning != nil && req.Reasoning.Effort != "" {
		if isUltraReasoningEffort(req.Reasoning.Effort) {
			return nil, fmt.Errorf("reasoning effort %q is not supported", strings.TrimSpace(req.Reasoning.Effort))
		}
		effort := mapResponsesEffortToAnthropic(req.Reasoning.Effort)
		out.OutputConfig = &AnthropicOutputConfig{Effort: effort}
		// Enable thinking for non-low efforts
		if effort != "low" {
			out.Thinking = &AnthropicThinking{
				Type:         "enabled",
				BudgetTokens: defaultThinkingBudget(effort),
			}
		}
	}

	return out, nil
}

// defaultThinkingBudget returns a sensible thinking budget based on effort level.
func defaultThinkingBudget(effort string) int {
	switch effort {
	case "low":
		return 1024
	case "medium":
		return 4096
	case "high":
		return 10240
	case "xhigh", "max":
		return 32768
	default:
		return 10240
	}
}

// mapResponsesEffortToAnthropic 分别映射 xhigh 和 max，max 会触发独立的计费倍率。
func mapResponsesEffortToAnthropic(effort string) string {
	switch value := strings.ToLower(strings.TrimSpace(effort)); value {
	case "low", "medium", "high", "xhigh", "max":
		return value
	default:
		return effort
	}
}

// convertResponsesInputToAnthropic 从 Responses API 的 instructions 与 input 数组中
// 提取系统提示和消息，并以原始 JSON 返回 Anthropic 多态 system 字段。
func convertResponsesInputToAnthropic(instructions string, inputRaw json.RawMessage) (json.RawMessage, []AnthropicMessage, error) {
	var systemParts []string
	if strings.TrimSpace(instructions) != "" {
		systemParts = append(systemParts, strings.TrimSpace(instructions))
	}

	// Try as plain string input.
	var inputStr string
	if err := json.Unmarshal(inputRaw, &inputStr); err == nil {
		content, _ := json.Marshal(inputStr)
		var system json.RawMessage
		if len(systemParts) > 0 {
			system, _ = json.Marshal(strings.Join(systemParts, "\n\n"))
		}
		return system, []AnthropicMessage{{Role: "user", Content: content}}, nil
	}

	var items []ResponsesInputItem
	if err := json.Unmarshal(inputRaw, &items); err != nil {
		return nil, nil, fmt.Errorf("parse responses input: %w", err)
	}

	var messages []AnthropicMessage

	for _, item := range items {
		switch {
		case item.Role == "system" || item.Role == "developer":
			text := extractTextFromContent(item.Content)
			if text != "" {
				systemParts = append(systemParts, text)
			}

		case item.Type == "function_call":
			// function_call → assistant message with tool_use block
			input := json.RawMessage("{}")
			if item.Arguments != "" {
				input = json.RawMessage(item.Arguments)
			}
			block := AnthropicContentBlock{
				Type:  "tool_use",
				ID:    fromResponsesCallIDToAnthropic(item.CallID),
				Name:  item.Name,
				Input: input,
			}
			blockJSON, _ := json.Marshal([]AnthropicContentBlock{block})
			messages = append(messages, AnthropicMessage{
				Role:    "assistant",
				Content: blockJSON,
			})

		case item.Type == "function_call_output":
			// function_call_output → user message with tool_result block
			contentJSON := responsesFunctionOutputToAnthropicContent(item)
			block := AnthropicContentBlock{
				Type:      "tool_result",
				ToolUseID: fromResponsesCallIDToAnthropic(item.CallID),
				Content:   contentJSON,
			}
			blockJSON, _ := json.Marshal([]AnthropicContentBlock{block})
			messages = append(messages, AnthropicMessage{
				Role:    "user",
				Content: blockJSON,
			})

		case item.Type == "reasoning":
			// Anthropic thinking 重放需要自己签发的 signature，OpenAI reasoning 的 encrypted_content 无法用于重放。
			// 因此丢弃 reasoning item，包括 summary、encrypted_content 和 content 数组。
			// 将 reasoning_text 原样放入 Anthropic 请求会返回 400。

		case item.Role == "user":
			content, err := convertResponsesUserToAnthropicContent(item.Content)
			if err != nil {
				return nil, nil, err
			}
			// 内容里只有网关不认识的分片时，sanitize 会得到空串。Anthropic 拒收
			// 空内容消息（"all messages must have non-empty content"），整条丢掉
			// 比发一条必然 400 的消息更可用。
			if anthropicContentIsEmpty(content) || anthropicContentIsOnlyBlankText(content) {
				continue
			}
			messages = append(messages, AnthropicMessage{
				Role:    "user",
				Content: content,
			})

		case item.Role == "assistant":
			content, err := convertResponsesAssistantToAnthropicContent(item.Content)
			if err != nil {
				return nil, nil, err
			}
			// 同上：分片全不认识时会退化成单个空 text 块，而 Anthropic 拒收
			// 空文本块（"text content blocks must contain non-whitespace text"）。
			if anthropicContentIsEmpty(content) || anthropicContentIsOnlyBlankText(content) {
				continue
			}
			messages = append(messages, AnthropicMessage{
				Role:    "assistant",
				Content: content,
			})

		default:
			// 未知 role/type 按 user 消息的白名单转换其中的文本和图片。
			// Responses 专用分片（如 reasoning_text、web_search_call 载荷）会使 Anthropic 返回 400。
			if item.Content == nil {
				continue
			}
			content, err := convertResponsesUserToAnthropicContent(item.Content)
			if err != nil {
				return nil, nil, err
			}
			if anthropicContentIsEmpty(content) || anthropicContentIsOnlyBlankText(content) {
				continue
			}
			messages = append(messages, AnthropicMessage{
				Role:    "user",
				Content: content,
			})
		}
	}

	// 先修复 tool_use/tool_result 配对，再合并连续同角色消息（Anthropic 要求角色交替）。
	// 第一次合并会把并行调用及其结果聚在一起，便于配对修复统一处理；配对修复可能再次拆分
	// user 轮次（例如调用和输出之间夹了注入消息），因此最后再合并一次恢复角色交替。
	messages = mergeConsecutiveMessages(messages)
	messages = normalizeAnthropicToolPairing(messages)
	messages = mergeConsecutiveMessages(messages)

	var system json.RawMessage
	if len(systemParts) > 0 {
		system, _ = json.Marshal(strings.Join(systemParts, "\n\n"))
	}

	return system, messages, nil
}

// responsesFunctionOutputToAnthropicContent 将工具输出转换为 Anthropic 的文本或多模态内容。
func responsesFunctionOutputToAnthropicContent(item ResponsesInputItem) json.RawMessage {
	if len(item.RawOutput()) == 0 {
		output := item.Output
		if output == "" {
			output = "(empty)"
		}
		content, _ := json.Marshal(output)
		return content
	}

	var parts []ResponsesContentPart
	if err := json.Unmarshal(item.RawOutput(), &parts); err == nil {
		blocks := make([]AnthropicContentBlock, 0, len(parts))
		for _, part := range parts {
			switch part.Type {
			case "input_text", "output_text", "text":
				if part.Text != "" {
					blocks = append(blocks, AnthropicContentBlock{Type: "text", Text: part.Text})
				}
			case "input_image":
				if source := dataURIToAnthropicImageSource(part.ImageURL); source != nil {
					blocks = append(blocks, AnthropicContentBlock{Type: "image", Source: source})
				}
			}
		}
		if len(blocks) > 0 {
			content, _ := json.Marshal(blocks)
			return content
		}
		if len(parts) == 0 {
			content, _ := json.Marshal("(empty)")
			return content
		}
	}

	content, _ := json.Marshal(item.Output)
	return content
}

// normalizeAnthropicToolPairing 重建消息序列，使 tool_use 和 tool_result 相邻配对，user 与 assistant 交替。
// 每个 tool_result 对应前一条 assistant 中的 tool_use，每个 tool_use 对应后一条 user 中的 tool_result。
// Codex 的 store:false 请求会重发完整历史，其中调用与输出之间可能有开发者或审批提示，
// 也可能有尚未返回输出的并行调用。逐项转换会产生分离的工具消息并触发 Anthropic 400。
// 此处先按 tool_use ID 索引结果，再按调用顺序把有结果的调用和结果放入相邻消息。
// 缺少结果的调用、孤立结果和由此产生的空 assistant 消息会被丢弃，非工具内容保持原位。
// Responses 转 Chat 的 normalizeChatMessages 使用相同的配对规则。
func normalizeAnthropicToolPairing(messages []AnthropicMessage) []AnthropicMessage {
	// 按 tool_use id 索引所有 tool_result；重复 id 时保留最后一个。
	results := make(map[string]AnthropicContentBlock)
	for _, m := range messages {
		if m.Role != "user" {
			continue
		}
		for _, b := range parseContentBlocks(m.Content) {
			if b.Type == "tool_result" && b.ToolUseID != "" {
				results[b.ToolUseID] = b
			}
		}
	}

	out := make([]AnthropicMessage, 0, len(messages))
	for _, m := range messages {
		blocks := parseContentBlocks(m.Content)
		switch m.Role {
		case "assistant":
			var toolUses, others []AnthropicContentBlock
			for _, b := range blocks {
				if b.Type == "tool_use" {
					toolUses = append(toolUses, b)
				} else {
					others = append(others, b)
				}
			}
			if len(toolUses) == 0 {
				out = append(out, m)
				continue
			}
			kept := make([]AnthropicContentBlock, 0, len(toolUses))
			for _, tu := range toolUses {
				if _, ok := results[tu.ID]; ok {
					kept = append(kept, tu)
				}
			}
			if len(kept) == 0 {
				// 没有已回答调用时，只保留非工具内容；没有非工具内容则整条丢弃。
				if len(others) > 0 {
					out = append(out, anthropicMessageFromBlocks("assistant", others))
				}
				continue
			}
			asstBlocks := make([]AnthropicContentBlock, 0, len(others)+len(kept))
			asstBlocks = append(asstBlocks, others...)
			asstBlocks = append(asstBlocks, kept...)
			out = append(out, anthropicMessageFromBlocks("assistant", asstBlocks))

			resBlocks := make([]AnthropicContentBlock, 0, len(kept))
			for _, tu := range kept {
				resBlocks = append(resBlocks, results[tu.ID])
			}
			out = append(out, anthropicMessageFromBlocks("user", resBlocks))

		case "user":
			var nonResult []AnthropicContentBlock
			hasResult := false
			for _, b := range blocks {
				if b.Type == "tool_result" {
					hasResult = true
					continue
				}
				nonResult = append(nonResult, b)
			}
			if !hasResult {
				out = append(out, m)
				continue
			}
			// tool_result 会被重新发到对应调用旁边；本轮其他 user 内容保留在原位。
			if len(nonResult) > 0 {
				out = append(out, anthropicMessageFromBlocks("user", nonResult))
			}

		default:
			out = append(out, m)
		}
	}
	return out
}

// anthropicMessageFromBlocks 用 block 数组构造 AnthropicMessage。
func anthropicMessageFromBlocks(role string, blocks []AnthropicContentBlock) AnthropicMessage {
	content, _ := json.Marshal(blocks)
	return AnthropicMessage{Role: role, Content: content}
}

// extractTextFromContent extracts text from a content field that may be a
// plain string or an array of content parts.
func extractTextFromContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var parts []ResponsesContentPart
	if err := json.Unmarshal(raw, &parts); err == nil {
		var texts []string
		for _, p := range parts {
			if (p.Type == "input_text" || p.Type == "output_text" || p.Type == "text") && p.Text != "" {
				texts = append(texts, p.Text)
			}
		}
		return strings.Join(texts, "\n\n")
	}
	return ""
}

// anthropicContentIsEmpty 判断转换结果是否为"空内容"。
// convertResponsesUserToAnthropicContent 在没有任何可识别分片时返回 JSON 空串，
// 而 Anthropic 拒收空内容消息。
func anthropicContentIsEmpty(content json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(content))
	switch trimmed {
	case "", "null", `""`, "[]":
		return true
	}
	// 字符串形式的消息也不能只包含空白字符。
	var text string
	if err := json.Unmarshal(content, &text); err == nil {
		return strings.TrimSpace(text) == ""
	}
	return false
}

// anthropicContentIsOnlyBlankText 判断内容是否只由空白 text 块组成。
func anthropicContentIsOnlyBlankText(content json.RawMessage) bool {
	blocks := parseContentBlocks(content)
	if len(blocks) == 0 {
		return false
	}
	for _, b := range blocks {
		if b.Type != "text" || strings.TrimSpace(b.Text) != "" {
			return false
		}
	}
	return true
}

// convertResponsesUserToAnthropicContent 把 Responses 用户消息转换为 Anthropic
// content block JSON，并过滤无法映射或只有空白字符的文本分片。
func convertResponsesUserToAnthropicContent(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.Marshal("") // empty string content
	}

	// Try plain string.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return json.Marshal(s)
	}

	// Array of content parts → Anthropic content blocks.
	var parts []ResponsesContentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		// Pass through as-is if we can't parse
		return raw, nil
	}

	var blocks []AnthropicContentBlock
	for _, p := range parts {
		switch p.Type {
		case "input_text", "text":
			if strings.TrimSpace(p.Text) != "" {
				blocks = append(blocks, AnthropicContentBlock{
					Type: "text",
					Text: p.Text,
				})
			}
		case "input_image":
			src := dataURIToAnthropicImageSource(p.ImageURL)
			if src != nil {
				blocks = append(blocks, AnthropicContentBlock{
					Type:   "image",
					Source: src,
				})
			}
		}
	}

	if len(blocks) == 0 {
		return json.Marshal("")
	}
	return json.Marshal(blocks)
}

// convertResponsesAssistantToAnthropicContent 把 Responses assistant 消息转换为
// Anthropic content block JSON，并过滤只有空白字符的文本分片。
func convertResponsesAssistantToAnthropicContent(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.Marshal([]AnthropicContentBlock{{Type: "text", Text: ""}})
	}

	// Try plain string.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return json.Marshal([]AnthropicContentBlock{{Type: "text", Text: s}})
	}

	// Array of content parts → Anthropic content blocks.
	var parts []ResponsesContentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return raw, nil
	}

	var blocks []AnthropicContentBlock
	for _, p := range parts {
		switch p.Type {
		case "output_text", "text":
			if strings.TrimSpace(p.Text) != "" {
				blocks = append(blocks, AnthropicContentBlock{
					Type: "text",
					Text: p.Text,
				})
			}
		}
	}

	if len(blocks) == 0 {
		blocks = append(blocks, AnthropicContentBlock{Type: "text", Text: ""})
	}
	return json.Marshal(blocks)
}

// fromResponsesCallIDToAnthropic converts an OpenAI function call ID back to
// Anthropic format. Reverses toResponsesCallID.
func fromResponsesCallIDToAnthropic(id string) string {
	// If it has our "fc_" prefix wrapping a known Anthropic prefix, strip it
	if after, ok := strings.CutPrefix(id, "fc_"); ok {
		if strings.HasPrefix(after, "toolu_") || strings.HasPrefix(after, "call_") {
			return after
		}
	}
	// Generate a synthetic Anthropic tool ID
	if !strings.HasPrefix(id, "toolu_") && !strings.HasPrefix(id, "call_") {
		return "toolu_" + id
	}
	return id
}

// dataURIToAnthropicImageSource parses a data URI into an AnthropicImageSource.
func dataURIToAnthropicImageSource(dataURI string) *AnthropicImageSource {
	if !strings.HasPrefix(dataURI, "data:") {
		return nil
	}
	// Format: data:<media_type>;base64,<data>
	rest := strings.TrimPrefix(dataURI, "data:")
	semicolonIdx := strings.Index(rest, ";")
	if semicolonIdx < 0 {
		return nil
	}
	mediaType := rest[:semicolonIdx]
	rest = rest[semicolonIdx+1:]
	if !strings.HasPrefix(rest, "base64,") {
		return nil
	}
	data := strings.TrimPrefix(rest, "base64,")
	return &AnthropicImageSource{
		Type:      "base64",
		MediaType: mediaType,
		Data:      data,
	}
}

// mergeConsecutiveMessages merges consecutive messages with the same role
// because Anthropic requires alternating user/assistant turns.
func mergeConsecutiveMessages(messages []AnthropicMessage) []AnthropicMessage {
	if len(messages) <= 1 {
		return messages
	}

	var merged []AnthropicMessage
	for _, msg := range messages {
		if len(merged) == 0 || merged[len(merged)-1].Role != msg.Role {
			merged = append(merged, msg)
			continue
		}

		// 相邻消息角色相同时合并内容数组。
		last := &merged[len(merged)-1]
		lastBlocks := parseContentBlocks(last.Content)
		newBlocks := parseContentBlocks(msg.Content)
		combined := append(lastBlocks, newBlocks...)
		last.Content, _ = json.Marshal(combined)
	}
	return merged
}

// parseContentBlocks attempts to parse content as []AnthropicContentBlock.
// If it's a string, wraps it in a text block.
func parseContentBlocks(raw json.RawMessage) []AnthropicContentBlock {
	var blocks []AnthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		return blocks
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []AnthropicContentBlock{{Type: "text", Text: s}}
	}
	return nil
}

// convertResponsesToAnthropicTools maps Responses API tools to Anthropic format.
// Reverse of convertAnthropicToolsToResponses.
func convertResponsesToAnthropicTools(tools []ResponsesTool) []AnthropicTool {
	var out []AnthropicTool
	for _, t := range tools {
		switch t.Type {
		case "web_search", "google_search", "web_search_20250305":
			out = append(out, AnthropicTool{
				Type: "web_search_20250305",
				Name: "web_search",
			})
		case "function":
			out = append(out, AnthropicTool{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: normalizeAnthropicInputSchema(t.Parameters),
			})
		case "custom":
			out = append(out, AnthropicTool{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: normalizeAnthropicInputSchema(t.Parameters),
			})
		default:
			// 未知工具类型保留 type，但仍要保证 input_schema 合法。
			out = append(out, AnthropicTool{
				Type:        t.Type,
				Name:        t.Name,
				Description: t.Description,
				InputSchema: normalizeAnthropicInputSchema(t.Parameters),
			})
		}
	}
	return out
}

// normalizeAnthropicInputSchema 将 input_schema 规范化为 object schema。
func normalizeAnthropicInputSchema(schema json.RawMessage) json.RawMessage {
	const emptyObjectSchema = `{"type":"object","properties":{}}`

	trimmed := strings.TrimSpace(string(schema))
	if trimmed == "" || trimmed == "null" {
		return json.RawMessage(emptyObjectSchema)
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(schema, &m); err != nil {
		return json.RawMessage(emptyObjectSchema)
	}

	typeRaw, ok := m["type"]
	if !ok || strings.TrimSpace(string(typeRaw)) == "" || string(typeRaw) == "null" {
		m["type"] = json.RawMessage(`"object"`)
	} else {
		var typ string
		if err := json.Unmarshal(typeRaw, &typ); err != nil || typ != "object" {
			return json.RawMessage(emptyObjectSchema)
		}
	}

	if _, ok := m["properties"]; !ok {
		m["properties"] = json.RawMessage(`{}`)
	}

	out, err := json.Marshal(m)
	if err != nil {
		return json.RawMessage(emptyObjectSchema)
	}
	return out
}

// convertResponsesToAnthropicToolChoice maps Responses tool_choice to Anthropic format.
// Reverse of convertAnthropicToolChoiceToResponses.
//
//	"auto"                                     → {"type":"auto"}
//	"required"                                 → {"type":"any"}
//	"none"                                     → {"type":"none"}
//	{"type":"function","name":"X"}                 → {"type":"tool","name":"X"}
//	{"type":"function","function":{"name":"X"}}     → {"type":"tool","name":"X"} // legacy
func convertResponsesToAnthropicToolChoice(raw json.RawMessage) (json.RawMessage, error) {
	// Try as string first
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		switch s {
		case "auto":
			return json.Marshal(map[string]string{"type": "auto"})
		case "required":
			return json.Marshal(map[string]string{"type": "any"})
		case "none":
			return json.Marshal(map[string]string{"type": "none"})
		default:
			return raw, nil
		}
	}

	// Try as object with type=function
	var tc struct {
		Type     string `json:"type"`
		Name     string `json:"name"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &tc); err == nil && tc.Type == "function" {
		name := strings.TrimSpace(tc.Name)
		if name == "" {
			name = strings.TrimSpace(tc.Function.Name)
		}
		if name == "" {
			return raw, nil
		}
		return json.Marshal(map[string]string{
			"type": "tool",
			"name": name,
		})
	}

	// Pass through unknown
	return raw, nil
}
