package billing

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/billing/pricing"
)

// TestCalculateCostUnifiedUltrafastCatalog 检查两种查价入口如何按目录快照中的上下文阶梯结算。
func TestCalculateCostUnifiedUltrafastCatalog(t *testing.T) {
	const model = "custom"
	long := &pricing.CatalogModelPricing{
		InputCostPerToken: 3, OutputCostPerToken: 5,
		InputCostPerImageToken: 1, OutputCostPerImageToken: 7,
		CacheReadInputTokenCost: 2, CacheCreationInputTokenCost: 4, CacheCreationInputTokenCostAbove1hr: 6,
	}
	catalog := defaultCatalogStub{entries: map[string]*pricing.CatalogModelPricing{
		model: {InputCostPerToken: 1, OutputCostPerToken: 2, ContextPrices: []pricing.CatalogContextPrice{{Threshold: 8, Pricing: long}}},
	}}
	calculator := NewCalculator(nil, CalculatorOptions{}).WithPriceCatalog(catalog)
	resolver := NewPriceResolver(nil, calculator, nil, nil)
	for _, tc := range []struct {
		name     string
		resolver *PriceResolver
	}{
		{"catalog", nil},
		{"resolved", resolver},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := CostInput{
				Ctx: context.Background(), Model: model, Resolver: tc.resolver, ServiceTier: "ultrafast", RateMultiplier: 0.25,
				Tokens: UsageTokens{
					InputTokens: 4, ImageInputTokens: 1, OutputTokens: 4, ImageOutputTokens: 2,
					CacheReadTokens: 3, CacheCreationTokens: 2, CacheCreation5mTokens: 1, CacheCreation1hTokens: 1,
				},
			}
			cost, err := calculator.CalculateCostUnified(input)
			require.NoError(t, err)
			require.True(t, cost.LongContextBillingApplied)
			require.Equal(t, 54.0, cost.InputCost)
			require.Equal(t, 6.0, cost.ImageInputCost)
			require.Equal(t, 60.0, cost.OutputCost)
			require.Equal(t, 84.0, cost.ImageOutputCost)
			require.Equal(t, 36.0, cost.CacheReadCost)
			require.Equal(t, 60.0, cost.CacheCreationCost)
			require.Equal(t, 300.0, cost.TotalCost)
			require.Equal(t, 75.0, cost.ActualCost)

			input.Model = "unpriced"
			_, err = calculator.CalculateCostUnified(input)
			require.ErrorIs(t, err, pricing.ErrModelPricingUnavailable)
		})
	}
}

// TestPerRequestContextPricingIgnoresModelName 验证模型名称不改变按次区间及默认价的选择。
func TestPerRequestContextPricingIgnoresModelName(t *testing.T) {
	for _, model := range []string{"custom-chat", "custom-image-analyzer", "gpt-image-alias"} {
		for _, withDefault := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/default=%t", model, withDefault), func(t *testing.T) {
				defaultPrice, shortPrice, longPrice := 0.05, 0.03, 0.10
				boundary := 128000
				card := &ModelPricingEntry{
					Models: []string{model}, BillingMode: pricing.BillingModePerRequest,
					Intervals: []pricing.PricingInterval{
						{MinTokens: 0, MaxTokens: &boundary, PerRequestPrice: &shortPrice},
						{MinTokens: boundary, PerRequestPrice: &longPrice},
					},
				}
				if withDefault {
					card.PerRequestPrice = &defaultPrice
				}
				calculator := NewCalculator(nil, CalculatorOptions{})
				resolver := NewPriceResolver(mediaPriceCards{card: card}, calculator, nil, nil)
				groupID := int64(1)
				for _, tc := range []struct {
					input int
					want  float64
				}{
					{1000, 0.03}, {127000, 0.03}, {127001, 0.10}, {200000, 0.10},
				} {
					resolved := resolver.Resolve(context.Background(), PricingInput{Model: model, GroupID: &groupID})
					before := append([]pricing.PricingInterval(nil), resolved.RequestTiers...)
					cost, err := calculator.CalculateCostUnified(CostInput{
						Ctx: context.Background(), Model: model, GroupID: &groupID,
						Tokens:       UsageTokens{InputTokens: tc.input, CacheReadTokens: 500, CacheCreationTokens: 500},
						RequestCount: 2, RateMultiplier: 1.5, Resolver: resolver, Resolved: resolved,
					})
					require.NoError(t, err)
					require.InDelta(t, tc.want*2, cost.TotalCost, 1e-12)
					require.InDelta(t, tc.want*3, cost.ActualCost, 1e-12)
					require.Equal(t, string(pricing.BillingModePerRequest), cost.BillingMode)
					require.Equal(t, before, resolved.RequestTiers, "不能清空或修改已解析区间")
				}
			})
		}
	}
}

// TestPerRequestLabelStillPrecedesContext 检查按次标签优先于上下文区间和默认价。
func TestPerRequestLabelStillPrecedesContext(t *testing.T) {
	labelPrice, contextPrice, defaultPrice := 0.07, 0.10, 0.05
	card := &ModelPricingEntry{
		BillingMode: pricing.BillingModePerRequest, PerRequestPrice: &defaultPrice,
		Intervals: []pricing.PricingInterval{
			{TierLabel: "HD", PerRequestPrice: &labelPrice, MinTokens: 300000},
			{MinTokens: 128000, PerRequestPrice: &contextPrice},
		},
	}
	calculator := NewCalculator(nil, CalculatorOptions{})
	resolver := NewPriceResolver(mediaPriceCards{card: card}, calculator, nil, nil)
	groupID := int64(1)
	for _, tc := range []struct {
		size   string
		tokens int
		want   float64
	}{
		{"HD", 200000, 0.07}, {"", 200000, 0.10}, {"", 1000, 0.05},
	} {
		cost, err := calculator.CalculateCostUnified(CostInput{
			Ctx: context.Background(), Model: "custom-image-analyzer", GroupID: &groupID,
			Tokens: UsageTokens{InputTokens: tc.tokens}, SizeTier: tc.size,
			RequestCount: 1, RateMultiplier: 1, Resolver: resolver,
		})
		require.NoError(t, err)
		require.InDelta(t, tc.want, cost.ActualCost, 1e-12)
	}
}
