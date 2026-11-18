package billing

import (
	"context"

	"github.com/TokenFlux/TokenRouter/internal/billing/pricing"
)

// PublicQuoteInput 包含公开报价的价卡和倍率，FreeFastApplicable 表示该场景允许展示免费 Fast。
type PublicQuoteInput struct {
	PricingInput
	RateMultiplier     float64
	FreeFastApplicable bool
}

// PublicQuote 保留共享查价顺序；free Fast 只调整已支持该档位的展示副本。
func (r *PriceResolver) PublicQuote(ctx context.Context, input PublicQuoteInput) ModelDisplayPricing {
	resolved := r.Resolve(ctx, input.PricingInput)
	settings := r.BillingSettings(ctx, input.GroupID)
	if resolved.Mode == pricing.BillingModeImage && settings.ImageRateIndependent {
		input.RateMultiplier = settings.ImageRateMultiplier
	}
	if settings.FreeOpenAIFast && input.FreeFastApplicable && pricing.ResolvedHasFastModeDisplayPricing(resolved) {
		cloned := *resolved
		standardMultiplier := 1.0
		pricing.ApplyPricingModifiers(&cloned, &ModelPricingEntry{FastMultiplier: &standardMultiplier})
		resolved = &cloned
	}
	return r.calculator.DisplayPricingWithResolvedMultipliers(input.Model, input.RateMultiplier, resolved)
}
