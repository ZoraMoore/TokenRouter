package provider

import (
	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/upstream/grok"
)

// NewGrokQuotaView 为提供商展示绑定供应商档位解析函数。
func NewGrokQuotaView() *provider.GrokQuotaView {
	return &provider.GrokQuotaView{
		FreeTokenLimit:      grok.GrokFreeRolling24hTokenLimit,
		NeedsReauth:         provider.GrokNeedsReauth,
		JWTSubscriptionTier: grok.SubscriptionTierFromJWT,
		CanonicalPlan:       grok.CanonicalGrokPlan,
		ParseTime:           provider.ParseUsageTime,
	}
}
