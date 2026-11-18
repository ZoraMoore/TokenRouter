package anthropic

import (
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/protocol"
)

func MergeAnthropicUsage(dst *protocol.TokenUsage, src AnthropicUsage) {
	if dst == nil {
		return
	}

	// 部分 Anthropic 兼容提供商同时返回 OpenAI 风格的 prompt/cache 字段。
	// 优先使用总输入或缓存命中、未命中分桶，input_tokens 在 Kimi 的不同事件中含义不同。
	// 此处同时处理 Kimi 的流式差异和 GLM/DeepSeek 的缓存别名。
	if src.PromptTokens > 0 || src.PromptCacheHitTokens != nil || src.PromptCacheMissTokens != nil {
		cacheReadTokens := src.CacheReadInputTokens
		if cacheReadTokens == 0 && src.CachedTokens > 0 {
			cacheReadTokens = src.CachedTokens
		}
		if cacheReadTokens == 0 && src.PromptTokensDetails != nil && src.PromptTokensDetails.CachedTokens > 0 {
			cacheReadTokens = src.PromptTokensDetails.CachedTokens
		}
		if cacheReadTokens == 0 && src.PromptCacheHitTokens != nil {
			cacheReadTokens = max(*src.PromptCacheHitTokens, 0)
		}

		if src.PromptCacheMissTokens != nil {
			dst.InputTokens = max(*src.PromptCacheMissTokens, 0)
		} else {
			dst.InputTokens = max(src.PromptTokens-cacheReadTokens-src.CacheCreationInputTokens, 0)
		}
		dst.CacheReadInputTokens = cacheReadTokens
		dst.CacheCreationInputTokens = src.CacheCreationInputTokens
	} else {
		if src.InputTokens > 0 {
			dst.InputTokens = src.InputTokens
		}
		if src.CacheReadInputTokens > 0 {
			dst.CacheReadInputTokens = src.CacheReadInputTokens
		} else if src.CachedTokens > 0 {
			dst.CacheReadInputTokens = src.CachedTokens
		}
		if src.CacheCreationInputTokens > 0 {
			dst.CacheCreationInputTokens = src.CacheCreationInputTokens
		}
	}
	if src.OutputTokens > 0 {
		dst.OutputTokens = src.OutputTokens
	}
	if src.CacheReadInputTokens > 0 {
		dst.CacheReadInputTokens = src.CacheReadInputTokens
	}
	if src.CacheCreationInputTokens > 0 {
		dst.CacheCreationInputTokens = src.CacheCreationInputTokens
	}
	if strings.TrimSpace(src.Speed) != "" {
		dst.Speed = src.Speed
	}
}
