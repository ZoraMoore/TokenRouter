package billing

import (
	"context"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/billing/pricing"
)

// ResolveImageUnitPrice 为创作台和批量图片解析固定单张价；token 价不作为单张价。
func (r *PriceResolver) ResolveImageUnitPrice(ctx context.Context, input PricingInput, size string) (float64, error) {
	if r == nil || r.calculator == nil || strings.TrimSpace(input.Model) == "" {
		return 0, pricing.ErrModelPricingUnavailable
	}
	resolved := r.Resolve(ctx, input)
	return r.calculator.resolvedImageUnitPrice(input.Model, size, resolved)
}

// resolvedImageUnitPrice 为展示、预占和结算复用同一逐尺寸查价顺序。
func (s *Calculator) resolvedImageUnitPrice(model, size string, resolved *ResolvedPricing) (float64, error) {
	return pricing.ResolveImageUnitPrice(resolved, s.RawModelPricing(model), size)
}
