package provider

import (
	"maps"
	"strings"

	infraerrors "github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

func NormalizeProviderConcurrency(platform, providerType string, concurrency int) int {
	if platform == PlatformGrok && providerType == ProviderTypeOAuth {
		if concurrency <= 0 {
			return 1
		}
	}
	return concurrency
}

// NormalizeCNProviderCredentials 校验国产平台提供商组合，并为新提供商补齐历史默认值。
// 旧记录缺少 mode/protocol 时，读取按 payg + chat_completions 处理。
// 新建记录保存默认值，前端和监控据此选择适配器。
// @project-doc docs/interfaces/upstream_provider_matrix.md#cn_provider_protocols
func NormalizeCNProviderCredentials(provider *Record, isCreate bool) error {
	if provider == nil || !IsCNProvider(provider.Platform) {
		return nil
	}
	if provider.Type != ProviderTypeAPIKey {
		return infraerrors.BadRequest("CN_PROVIDER_PROVIDER_TYPE_INVALID", "CN providers must use API Key credentials")
	}
	if provider.Credentials == nil {
		provider.Credentials = make(map[string]any)
	}
	mode, _ := provider.Credentials["provider_mode"].(string)
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = ProviderModePayG
		if isCreate {
			provider.Credentials["provider_mode"] = mode
		}
	}
	if mode != ProviderModePayG && mode != ProviderModeCoding {
		return infraerrors.BadRequest("CN_PROVIDER_PROVIDER_MODE_INVALID", "provider_mode must be payg or coding")
	}
	protocol, _ := provider.Credentials["api_protocol"].(string)
	protocol = strings.TrimSpace(protocol)
	if protocol == "" {
		protocol = APIProtocolChatCompletions
		if _, unified := provider.Credentials[UpstreamProtocolsKey]; isCreate && !unified {
			provider.Credentials["api_protocol"] = protocol
		}
	}
	switch protocol {
	case APIProtocolAdaptive, APIProtocolChatCompletions, APIProtocolAnthropic:
	case APIProtocolResponses:
		// 保存校验与转发共用平台协议能力列表。
		if !provider.SupportsNativeCNResponses() {
			return infraerrors.BadRequest("CN_PROVIDER_PROTOCOL_INVALID", "only DeepSeek and Kimi support Responses protocol")
		}
	default:
		return infraerrors.BadRequest("CN_PROVIDER_PROTOCOL_INVALID", "api_protocol is unsupported")
	}
	if mode == ProviderModeCoding && provider.Platform == PlatformDeepseek {
		return infraerrors.BadRequest("CN_PROVIDER_MODE_INVALID", "DeepSeek does not support coding plan mode")
	}
	return nil
}

// ValidateGrokMediaEligibilityExtra 校验可选的媒体调度覆盖；null 表示删除覆盖，
// 让提供商恢复为根据上游观测自动判断。
func ValidateGrokMediaEligibilityExtra(platform string, extra map[string]any) error {
	if platform != PlatformGrok || extra == nil {
		return nil
	}
	raw, exists := extra[GrokMediaEligibleExtraKey]
	if !exists || raw == nil {
		return nil
	}
	if _, ok := raw.(bool); !ok {
		return infraerrors.BadRequest(
			"GROK_MEDIA_ELIGIBILITY_INVALID",
			"grok_media_eligible must be a boolean or null",
		)
	}
	return nil
}

func NormalizeGrokMediaEligibilityExtra(platform string, extra map[string]any) (map[string]any, error) {
	if platform != PlatformGrok {
		return extra, nil
	}
	if err := ValidateGrokMediaEligibilityExtra(platform, extra); err != nil {
		return nil, err
	}
	normalized := maps.Clone(extra)
	if normalized != nil && normalized[GrokMediaEligibleExtraKey] == nil {
		delete(normalized, GrokMediaEligibleExtraKey)
	}
	return normalized, nil
}

func NormalizeGrokMediaEligibilityUpdateExtra(provider *Record, input *UpdateProviderInput, normalized map[string]any) (map[string]any, error) {
	if provider == nil || provider.Platform != PlatformGrok {
		return normalized, nil
	}
	if err := ValidateGrokMediaEligibilityExtra(provider.Platform, input.Extra); err != nil {
		return nil, err
	}
	normalized = maps.Clone(normalized)
	if normalized == nil {
		normalized = make(map[string]any)
	}
	raw, provided := input.Extra[GrokMediaEligibleExtraKey]
	if provided {
		if raw == nil {
			delete(normalized, GrokMediaEligibleExtraKey)
		}
		return normalized, nil
	}
	if current, ok := provider.Extra[GrokMediaEligibleExtraKey].(bool); ok {
		normalized[GrokMediaEligibleExtraKey] = current
	}
	return normalized, nil
}

// ValidateGeminiThirdPartyBaseURL 阻止第三方来源回退到官方 Gemini 默认端点。
func ValidateGeminiThirdPartyBaseURL(provider *Record) error {
	if provider == nil || !provider.IsGeminiThirdPartyProvider() || provider.HasGeminiThirdPartyBaseURL() {
		return nil
	}
	return infraerrors.BadRequest(
		"GEMINI_THIRD_PARTY_BASE_URL_REQUIRED",
		"Gemini third-party API Key providers require a non-Google base_url",
	)
}
