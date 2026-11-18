package completion

import (
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/billing/pricing"
	"github.com/TokenFlux/TokenRouter/internal/protocol"
	"github.com/TokenFlux/TokenRouter/internal/protocol/openai"
)

type ServiceTierBillingResolution struct {
	Requested  string // 请求发送的档位，空值表示未指定
	Observed   string // 上游响应声明的档位，空值表示未声明
	Billing    string // 实际用于计费和日志的档位
	Downgraded bool   // 计费档位是否低于请求档位
}

func ResolveBillingServiceTier(requested, observed string) ServiceTierBillingResolution {
	requested = normalizeBillingServiceTier(requested)
	observed = normalizeBillingServiceTier(observed)
	resolution := ServiceTierBillingResolution{Requested: requested, Observed: observed, Billing: requested}
	if observed == "" || observed == requested {
		return resolution
	}
	observedRank, known := ServiceTierCostRank(observed)
	if !known {
		return resolution
	}
	requestedRank, _ := ServiceTierCostRank(requested)
	if observedRank >= requestedRank {
		return resolution
	}
	resolution.Billing = observed
	resolution.Downgraded = true
	return resolution
}

func ServiceTierCostRank(tier string) (rank int, known bool) {
	switch normalizeBillingServiceTier(tier) {
	case "flex":
		return 0, true
	case "", "default", "standard", "auto", "scale":
		return 1, true
	case "priority", "fast":
		return 2, true
	case openai.ServiceTierUltrafast:
		return 3, true
	default:
		return 1, false
	}
}

func applyCacheOverride(usage *TokenUsage, target string) bool {
	projected := protocol.TokenUsage{CacheCreationInputTokens: usage.CacheCreationInputTokens, CacheCreation5mTokens: usage.CacheCreation5mTokens, CacheCreation1hTokens: usage.CacheCreation1hTokens}
	changed := protocol.ApplyCacheTTLOverride(&projected, target)
	usage.CacheCreation5mTokens = projected.CacheCreation5mTokens
	usage.CacheCreation1hTokens = projected.CacheCreation1hTokens
	return changed
}

// normalizeResult 根据请求快照和响应观测规范化本次结果的档位。
func (s *Recorder) normalizeResult(r *Result, a *ProviderSnapshot, openAI bool, observedProvider *ProviderSnapshot, requestID string) ServiceTierBillingResolution {
	// 缺失结果不产生档位调整或观测事件。
	if r == nil {
		return ServiceTierBillingResolution{}
	}
	if r.ImageCount > 0 && (!openAI || r.VideoCount <= 0) {
		input := strings.TrimSpace(r.ImageInputSize)
		if input == "" && strings.TrimSpace(r.ImageSize) != pricing.ImageBillingSize2K {
			input = strings.TrimSpace(r.ImageSize)
		}
		sizes := r.ImageOutputSizes
		if len(sizes) == 0 && strings.TrimSpace(r.ImageOutputSize) != "" {
			sizes = []string{r.ImageOutputSize}
		}
		v := pricing.ResolveImageBillingSize(input, sizes)
		r.ImageSize, r.ImageInputSize, r.ImageOutputSize, r.ImageSizeSource, r.ImageSizeBreakdown = v.BillingSize, v.InputSize, v.OutputSize, v.Source, v.Breakdown
	}
	requested := stringValueOrEmpty(r.ServiceTier)
	if !openAI && requested == "" {
		requested = strings.TrimSpace(r.Usage.Speed)
	}
	// 影子提供商借用母提供商凭据，响应计费开关仍取自本次选中的提供商。
	responseBilling := openAI && observedProvider != nil && observedProvider.OpenAIResponseTierBillingEnabled
	var resolution ServiceTierBillingResolution
	if openAI {
		resolution = resolveOpenAIServiceTierBilling(a != nil && a.OAuthLike, responseBilling, requested, r.UpstreamResponseServiceTier)
	} else {
		resolution = ResolveBillingServiceTier(requested, r.UpstreamResponseServiceTier)
	}
	if resolution.Billing != resolution.Requested {
		tier := resolution.Billing
		r.ServiceTier = &tier
		if !openAI && r.Usage.Speed != "" {
			r.Usage.Speed = tier
		}
	}
	if responseBilling || resolution.Downgraded {
		event := BillingEvent{Kind: "tier_downgrade", Component: "service.gateway", RequestID: strings.TrimSpace(requestID), RequestedTier: resolution.Requested, ObservedTier: resolution.Observed, BilledTier: resolution.Billing}
		if responseBilling {
			event.Kind = "tier_response"
			if resolution.Observed == "" {
				event.Kind = "tier_response_missing"
			}
		}
		if openAI {
			event.Component = "service.openai_gateway"
		}
		if observedProvider != nil {
			event.ProviderID = observedProvider.ID
			event.Platform = observedProvider.Platform
		}
		s.observeEvent(event)
	}
	return resolution
}

// resolveOpenAIServiceTierBilling 处理响应结算开关与 Codex OAuth 的 default 回显。
func resolveOpenAIServiceTierBilling(oauthLike, responseBilling bool, requested, observed string) ServiceTierBillingResolution {
	if responseBilling {
		observed = openai.ObservedServiceTier(observed)
	}
	if !responseBilling && oauthLike && CodexOAuthResponseTierIsNonAuthoritative(observed) {
		return ServiceTierBillingResolution{Requested: normalizeBillingServiceTier(requested), Observed: normalizeBillingServiceTier(observed), Billing: normalizeBillingServiceTier(requested)}
	}
	resolution := ResolveBillingServiceTier(requested, observed)
	if responseBilling && observed != "" {
		resolution.Billing = observed
	}
	return resolution
}

func CodexOAuthResponseTierIsNonAuthoritative(observed string) bool {
	return normalizeBillingServiceTier(observed) == "default"
}
