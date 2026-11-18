package provider

import (
	"strings"

	"github.com/tidwall/gjson"

	"github.com/TokenFlux/TokenRouter/internal/provider"
)

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
