package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/tidwall/gjson"

	"github.com/TokenFlux/TokenRouter/internal/gateway/media"
	protocolopenai "github.com/TokenFlux/TokenRouter/internal/protocol/openai"
	"github.com/TokenFlux/TokenRouter/internal/protocol/wirejson"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	provideradapter "github.com/TokenFlux/TokenRouter/internal/provider/provider"
	"github.com/TokenFlux/TokenRouter/internal/upstream/openai"
)

const (
	// 旧标记仍用于识别历史请求，避免重复注入并维持桥接判断。
	legacyClaudeCodeTodoGuardMarker = "<sub2api-claude-code-todo-guard>"

	OpenAICompatClaudeCodeTodoGuardMarker = "<tokenrouter-claude-code-todo-guard>"
	OpenAICompatClaudeCodeTodoGuardText   = OpenAICompatClaudeCodeTodoGuardMarker + "\nWhen using Claude Code todo or task tracking tools, keep the visible task list consistent. Do not send final or summary text while any item remains in_progress. Before finishing, asking the user to choose, or reporting a blocker, update the todo list so completed work is completed and deferred work is pending/open; leave an item in_progress only when active work will continue in the same turn.\n</tokenrouter-claude-code-todo-guard>"
)

// OpenAIModelSession 保存当前连接设置的图片工具型号，供省略 tools 的下一轮请求检查。
type OpenAIModelSession struct {
	ImageTools []string
}

// openAIOAuthModelFields 只保留协议中能指定执行型号的字段，普通消息里的 JSON 不参与判断。
type openAIOAuthModelFields struct {
	model, tools, choice, session, response, input gjson.Result
}

// CheckFrame 在模型名单变更后复核继承的工具，并在会话设置通过时替换本地记录。
func (s *OpenAIModelSession) CheckFrame(ctx context.Context, target *ExecutionProvider, model string, body []byte) error {
	if value := target.View(); value == nil || !value.IsOpenAIOAuth() {
		return nil
	}
	inherited := s.ImageTools
	event := gjson.GetBytes(body, "type").String()
	var changedTools gjson.Result
	switch event {
	case "session.update":
		changedTools = gjson.GetBytes(body, "session.tools")
		if changedTools.IsArray() {
			inherited = nil
		}
	case "response.create":
		fields := gjson.GetManyBytes(body, "tools", "response.tools", "response.model")
		if fields[0].IsArray() || fields[1].IsArray() {
			inherited = nil
		}
		if fields[2].Type == gjson.String {
			model = fields[2].String()
		}
	}
	if err := CheckOpenAIOAuthRequestModels(ctx, target, model, body, inherited...); err != nil {
		return err
	}
	if changedTools.IsArray() {
		s.ImageTools = media.SnapshotOpenAIImageToolModels(changedTools)
	}
	return nil
}

// readOpenAIOAuthModelFields 拒绝有歧义的重复字段，保证上游与网关检查同一份配置。
func readOpenAIOAuthModelFields(object gjson.Result) (openAIOAuthModelFields, bool) {
	var fields openAIOAuthModelFields
	valid := true
	object.ForEach(func(key, value gjson.Result) bool {
		var dest *gjson.Result
		switch key.Str {
		case "model":
			dest = &fields.model
		case "tools":
			dest = &fields.tools
		case "tool_choice":
			dest = &fields.choice
		case "session":
			dest = &fields.session
		case "response":
			dest = &fields.response
		case "input":
			dest = &fields.input
		default:
			return true
		}
		if dest.Exists() {
			valid = false
			return false
		}
		*dest = value
		return true
	})
	return fields, valid
}

// CheckOpenAIOAuthRequestModels 在请求发出前检查实际的主模型和图片工具指定的型号。
// WebSocket 的模型可以从上一轮继承，调用方传入本轮已解析的最终型号。
func CheckOpenAIOAuthRequestModels(ctx context.Context, target *ExecutionProvider, model string, body []byte, inheritedTools ...string) error {
	value := target.View()
	global := value.OpenAIOAuthFinalModelPolicy(ctx)
	if !global.Configured() {
		return nil
	}
	allows := value.OpenAIOAuthTargetFilter(global, provideradapter.ModelDefaults())
	root, ok := readOpenAIOAuthModelFields(wirejson.ParseView(body))
	if !ok || model == "" && !root.model.Exists() || model != "" && !allows(model) {
		return provider.ErrOpenAIOAuthModelNotAllowed
	}
	session, sessionOK := readOpenAIOAuthModelFields(root.session)
	response, responseOK := readOpenAIOAuthModelFields(root.response)
	if !sessionOK || !responseOK {
		return provider.ErrOpenAIOAuthModelNotAllowed
	}
	for _, field := range []gjson.Result{root.model, session.model, response.model} {
		if field.Exists() && (field.Type != gjson.String || !allows(field.Str)) {
			return provider.ErrOpenAIOAuthModelNotAllowed
		}
	}
	for _, tool := range inheritedTools {
		if !allows(tool) {
			return provider.ErrOpenAIOAuthModelNotAllowed
		}
	}
	// 普通文本请求可跳过较大的 input 数组。JSON 的 Unicode 转义也可能组成工具名。
	if !bytes.Contains(body, []byte("image_generation")) && !bytes.Contains(body, []byte(`\u`)) {
		return nil
	}
	allowedTools := func(tools, choice gjson.Result) bool {
		declared := false
		for id := range media.OpenAIImageToolModels(tools) {
			declared = true
			if !allows(id) {
				return false
			}
		}
		usesImage := openai.IsOpenAIImageGenerationType(choice.Str) || openai.IsOpenAIImageGenerationType(choice.Get("type").Str)
		return !usesImage || declared || len(inheritedTools) > 0 || allows(media.DefaultOpenAIImageModel)
	}
	for _, fields := range []openAIOAuthModelFields{root, session, response} {
		if !allowedTools(fields.tools, fields.choice) {
			return provider.ErrOpenAIOAuthModelNotAllowed
		}
	}
	valid := true
	root.input.ForEach(func(_, entry gjson.Result) bool {
		if entry.Get("type").String() == "additional_tools" {
			fields, ok := readOpenAIOAuthModelFields(entry)
			valid = ok && allowedTools(fields.tools, fields.choice)
		}
		return valid
	})
	if !valid {
		return provider.ErrOpenAIOAuthModelNotAllowed
	}
	return nil
}

func ApplyCodexOAuthTransform(reqBody map[string]any, isCodexCLI bool, isCompact bool) openai.CodexTransformResult {
	return ApplyCodexOAuthTransformWithOptions(reqBody, openai.CodexOAuthTransformOptions{IsCodexCLI: isCodexCLI, IsCompact: isCompact})
}

func ApplyCodexOAuthTransformWithOptions(reqBody map[string]any, opts openai.CodexOAuthTransformOptions) openai.CodexTransformResult {
	opts.IsMessagesBridge = IsOpenAICompatMessagesBridgeRequestBody
	return openai.ApplyCodexOAuthTransformWithOptions(reqBody, opts)
}

func IsCodexSparkModel(model string) bool {
	return openai.IsCodexSparkModel(model)
}

func StripOpenAIImageGenerationToolsFromRawPayload(payload []byte) ([]byte, bool, error) {
	return openai.StripOpenAIImageGenerationToolsFromRawPayload(payload, ImageIntent().OpenAIRequestBodyHasImageGenerationDeclaration(payload))
}

func ValidateCodexSparkInput(reqBody map[string]any, model string) error {
	return openai.ValidateCodexSparkInput(reqBody, model, IsCodexSparkModel(model))
}

func EnsureOpenAIResponsesImageGenerationTool(reqBody map[string]any) bool {
	return openai.EnsureOpenAIResponsesImageGenerationTool(reqBody, IsCodexSparkModel(openai.FirstNonEmptyString(reqBody["model"])))
}

func EnsureOpenAIResponsesImageGenerationToolChoiceAuto(reqBody map[string]any) bool {
	return openai.EnsureOpenAIResponsesImageGenerationToolChoiceAuto(reqBody, IsCodexSparkModel(openai.FirstNonEmptyString(reqBody["model"])))
}

func ApplyCodexImageGenerationBridgeInstructions(reqBody map[string]any) bool {
	return openai.ApplyCodexImageGenerationBridgeInstructions(reqBody, IsCodexSparkModel(openai.FirstNonEmptyString(reqBody["model"])))
}

func ValidateOpenAIResponsesImageModel(reqBody map[string]any, model string) error {
	return openai.ValidateOpenAIResponsesImageModel(reqBody, model, media.IsImageGenerationModel(strings.TrimSpace(model)))
}

func NormalizeOpenAIResponsesImageOnlyModel(reqBody map[string]any) bool {
	return openai.NormalizeOpenAIResponsesImageOnlyModel(reqBody, media.IsImageGenerationModel(openai.FirstNonEmptyString(reqBody["model"])))
}

func ApplyCodexClientMetadata(reqBody map[string]any, provider *ExecutionProvider) bool {
	if provider == nil {
		return false
	}
	return openai.ApplyCodexClientMetadata(reqBody, provider.View().GetOpenAIDeviceID())
}

func IsOpenAICompatMessagesBridgeBody(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	if bytes.Contains(body, []byte(OpenAICompatClaudeCodeTodoGuardMarker)) || bytes.Contains(body, []byte(legacyClaudeCodeTodoGuardMarker)) {
		return true
	}
	return IsOpenAICompatMessagesBridgePromptCacheKey(gjson.GetBytes(body, "prompt_cache_key").String())
}

func IsOpenAICompatMessagesBridgeRequestBody(reqBody map[string]any) bool {
	if reqBody == nil {
		return false
	}
	if input, ok := reqBody["input"].([]any); ok && (InputContainsText(input, OpenAICompatClaudeCodeTodoGuardMarker) || InputContainsText(input, legacyClaudeCodeTodoGuardMarker)) {
		return true
	}
	return IsOpenAICompatMessagesBridgePromptCacheKey(openai.FirstNonEmptyString(reqBody["prompt_cache_key"]))
}

func IsOpenAICompatMessagesBridgePromptCacheKey(key string) bool {
	key = strings.TrimSpace(key)
	return strings.HasPrefix(key, "anthropic-metadata-") ||
		strings.HasPrefix(key, "anthropic-cache-") ||
		strings.HasPrefix(key, "anthropic-digest-")
}

func AppendOpenAICompatClaudeCodeTodoGuard(req *protocolopenai.ResponsesRequest) bool {
	if req == nil || len(req.Input) == 0 {
		return false
	}

	var items []protocolopenai.ResponsesInputItem
	if err := json.Unmarshal(req.Input, &items); err != nil {
		return false
	}
	if len(items) == 0 || (ResponsesInputItemsContainText(items, OpenAICompatClaudeCodeTodoGuardMarker) || ResponsesInputItemsContainText(items, legacyClaudeCodeTodoGuardMarker)) {
		return false
	}

	content, err := json.Marshal([]protocolopenai.ResponsesContentPart{{
		Type: "input_text",
		Text: OpenAICompatClaudeCodeTodoGuardText,
	}})
	if err != nil {
		return false
	}

	guard := protocolopenai.ResponsesInputItem{
		Type:    "message",
		Role:    "developer",
		Content: content,
	}

	insertAt := 0
	for insertAt < len(items) && items[insertAt].Type == "message" && items[insertAt].Role == "developer" {
		insertAt++
	}

	items = append(items, protocolopenai.ResponsesInputItem{})
	copy(items[insertAt+1:], items[insertAt:])
	items[insertAt] = guard

	input, err := json.Marshal(items)
	if err != nil {
		return false
	}
	req.Input = input
	return true
}

func AppendOpenAICompatClaudeCodeTodoGuardToRequestBody(reqBody map[string]any) bool {
	if reqBody == nil {
		return false
	}

	input, ok := reqBody["input"].([]any)
	if !ok || len(input) == 0 || (InputContainsText(input, OpenAICompatClaudeCodeTodoGuardMarker) || InputContainsText(input, legacyClaudeCodeTodoGuardMarker)) {
		return false
	}

	guard := map[string]any{
		"type": "message",
		"role": "developer",
		"content": []any{
			map[string]any{
				"type": "input_text",
				"text": OpenAICompatClaudeCodeTodoGuardText,
			},
		},
	}

	insertAt := 0
	for insertAt < len(input) {
		item, ok := input[insertAt].(map[string]any)
		if !ok || strings.TrimSpace(openai.FirstNonEmptyString(item["type"])) != "message" || strings.TrimSpace(openai.FirstNonEmptyString(item["role"])) != "developer" {
			break
		}
		insertAt++
	}

	input = append(input, nil)
	copy(input[insertAt+1:], input[insertAt:])
	input[insertAt] = guard
	reqBody["input"] = input
	return true
}

func ResponsesInputItemsContainText(items []protocolopenai.ResponsesInputItem, needle string) bool {
	needle = strings.TrimSpace(needle)
	if needle == "" {
		return false
	}
	for _, item := range items {
		if strings.Contains(string(item.Content), needle) {
			return true
		}
	}
	return false
}

func InputContainsText(input []any, needle string) bool {
	needle = strings.TrimSpace(needle)
	if needle == "" {
		return false
	}
	for _, item := range input {
		b, err := json.Marshal(item)
		if err == nil && strings.Contains(string(b), needle) {
			return true
		}
	}
	return false
}
