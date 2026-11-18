package billing_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/billing"
	"github.com/TokenFlux/TokenRouter/internal/billing/pricing"
	"github.com/TokenFlux/TokenRouter/internal/billing/testkit"
	"github.com/TokenFlux/TokenRouter/internal/routing"
)

// TestIndependentImageBillingAndPublicQuote 核对图片实扣与公开报价使用相同的独立倍率。
func TestIndependentImageBillingAndPublicQuote(t *testing.T) {
	price := 0.03
	settings := pricing.DefaultBillingSettings()
	settings.ImageRateIndependent = true
	settings.ImageRateMultiplier = 1
	calculator := newTestCalculator()
	resolver, source := settingsResolver(calculator, settings, []routing.ModelPricingEntry{
		{Models: []string{"custom-image"}, BillingMode: routing.BillingModeImage, PerRequestPrice: &price},
	})
	for _, rate := range []float64{0.15, 0.13, 3} {
		t.Run(strconv.FormatFloat(rate, 'f', 2, 64), func(t *testing.T) {
			cost, err := calculator.CalculateCostUnified(billing.CostInput{
				Ctx: context.Background(), Model: "custom-image", GroupID: testkit.GroupID(),
				RequestCount: 2, RateMultiplier: rate, Resolver: resolver,
			})
			require.NoError(t, err)
			require.InDelta(t, 0.06, cost.ActualCost, 1e-12)
			quote := resolver.PublicQuote(context.Background(), billing.PublicQuoteInput{
				PricingInput:   billing.PricingInput{Model: "custom-image", GroupID: testkit.GroupID()},
				RateMultiplier: rate,
			})
			require.InDelta(t, price, quote.ImagePrice1K, 1e-12)
		})
	}
	// 关闭独立倍率后，恢复按调用方有效倍率计费。
	source.settings.ImageRateIndependent = false
	cost, err := calculator.CalculateCostUnified(billing.CostInput{
		Ctx: context.Background(), Model: "custom-image", GroupID: testkit.GroupID(),
		RequestCount: 1, RateMultiplier: 0.15, Resolver: resolver,
	})
	require.NoError(t, err)
	require.InDelta(t, 0.0045, cost.ActualCost, 1e-12)
}
