package provider

import (
	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/upstream/grok"
)

func GrokTierRules() provider.GrokTierRules {
	return provider.GrokTierRules{
		SubscriptionTierFromJWT:   grok.SubscriptionTierFromJWT,
		NormalizeSubscriptionTier: grok.NormalizeSubscriptionTier,
		IsFreeRollingTokenLimit:   grok.IsGrokFreeRolling24hTokenLimit,
	}
}
