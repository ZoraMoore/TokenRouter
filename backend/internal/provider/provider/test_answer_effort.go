package provider

import (
	"slices"
	"strings"

	"github.com/tidwall/gjson"

	"github.com/TokenFlux/TokenRouter/internal/provider"
)

// claudeDrawingEfforts 按 Sub2API 测智题库的 Claude 型号记录可发送的推理档位。
var claudeDrawingEfforts = []struct {
	families []string
	efforts  []string
	adaptive bool
}{
	{
		families: []string{"claude-opus-4-5"},
		efforts:  []string{"low", "medium", "high"},
	},
	{
		families: []string{"claude-mythos-preview", "claude-sonnet-4-6", "claude-opus-4-6"},
		efforts:  []string{"low", "medium", "high", "max"},
		adaptive: true,
	},
	{
		families: []string{"claude-mythos-5", "claude-fable-5", "claude-sonnet-5", "claude-opus-4-8", "claude-opus-4-7", "claude-opus-5", "claude-haiku-5-5"},
		efforts:  []string{"low", "medium", "high", "xhigh", "max"},
		adaptive: true,
	},
}

// applyClaudeDrawingEffort 根据实际请求的模型选择档位。Opus 4.5 支持 effort，思考模式由模型决定。
func applyClaudeDrawingEffort(payload map[string]any, model, effort string) {
	id := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(model)), "models/")
	if _, suffix, ok := strings.Cut(id, "/"); ok {
		id = strings.TrimPrefix(strings.TrimSpace(suffix), "models/")
	}
	for _, region := range []string{"us.", "eu.", "apac.", "jp.", "au.", "us-gov.", "global."} {
		id = strings.TrimPrefix(id, region)
	}
	id = strings.TrimPrefix(id, "anthropic.")
	id = strings.TrimSuffix(id, "-thinking")
	switch id {
	case "claude-opus-5.5", "claude-sonnet-5.5", "claude-haiku-5.5":
		id = strings.Replace(id, "5.5", "5-5", 1)
	}
	for _, rule := range claudeDrawingEfforts {
		if !slices.Contains(rule.efforts, effort) {
			continue
		}
		for _, family := range rule.families {
			if id == family || strings.HasPrefix(id, family+"-") {
				payload["output_config"] = map[string]string{"effort": effort}
				if rule.adaptive {
					payload["thinking"] = map[string]string{"type": "adaptive"}
				}
				return
			}
		}
	}
}

// sentAnswerEffort 读取发给上游的报文。只有 token 预算或思考开关时，无法据此推断具体档位。
func sentAnswerEffort(protocol answerProtocol, payload []byte) string {
	if !gjson.ValidBytes(payload) {
		return ""
	}
	body := gjson.ParseBytes(payload)
	if !body.IsObject() {
		return ""
	}
	var field gjson.Result
	switch protocol {
	case answerChat:
		field = body.Get("reasoning_effort")
	case answerResponses:
		field = body.Get("reasoning.effort")
	case answerClaude, answerBedrock:
		field = body.Get("output_config.effort")
		if !field.Exists() && body.Get("thinking").Exists() {
			if body.Get("thinking.type").String() == "disabled" {
				return "none"
			}
			return ""
		}
	case answerGemini:
		if inner := body.Get("request"); inner.IsObject() {
			body = inner
		}
		thinking := body.Get("generationConfig.thinkingConfig")
		field = thinking.Get("thinkingLevel")
		if budget := thinking.Get("thinkingBudget"); !field.Exists() && budget.Exists() {
			if budget.Type == gjson.Number && budget.Float() == 0 {
				return "none"
			}
			return ""
		}
	case answerQoder:
		field = body.Get("parameters.reasoning_effort")
		if enabled := body.Get("model_config.is_reasoning"); !field.Exists() && enabled.Exists() {
			if enabled.Type == gjson.False {
				return "none"
			}
			return ""
		}
	default:
		return ""
	}
	if !field.Exists() {
		return "default"
	}
	value := strings.ToLower(field.String())
	switch value {
	case "none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra":
		return value
	default:
		return ""
	}
}

// answerRequestError 使用上游指出的参数路径，把拒绝档位与其他请求错误区分开。
func answerRequestError(upstream gjson.Result) error {
	switch upstream.Get("param").String() {
	case "reasoning", "reasoning.effort", "reasoning_effort", "output_config.effort", "thinking":
		return provider.ErrTestAnswerEffort
	default:
		return errAnswerUpstream
	}
}
