package provider

import (
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/protocol/openai"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
)

// HasBillableGrokChatUsage 只检查聊天结算实际使用的聚合 token 桶。
// 明细字段本身不能证明响应可安全结算，至少一个聚合桶必须为正数。
func HasBillableGrokChatUsage(usage openai.ForwardUsage) bool {
	return usage.InputTokens > 0 ||
		usage.OutputTokens > 0 ||
		usage.CacheCreationInputTokens > 0 ||
		usage.CacheReadInputTokens > 0
}

// RequiresBillableGrokChatUsage 根据实际提供商平台和最终模型身份识别 Grok 流量。
// 通用 OpenAI 兼容提供商也可调用 Grok，因此同时检查提供商平台和映射后的上游模型。
func RequiresBillableGrokChatUsage(provider *ExecutionProvider, models ...string) bool {
	if provider != nil && provider.Record.Platform == capability.PlatformGrok {
		return true
	}
	for _, model := range models {
		normalized := strings.ToLower(strings.TrimSpace(model))
		if separator := strings.LastIndex(normalized, "/"); separator >= 0 {
			normalized = strings.TrimSpace(normalized[separator+1:])
		}
		if normalized == "grok" || strings.HasPrefix(normalized, "grok-") {
			return true
		}
	}
	return false
}
