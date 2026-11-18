package pricing

import (
	"fmt"
	"strings"
)

// ResolveModelPricing 将目录报价转换为模型计费单价。
func ResolveModelPricing(model string, catalogPrice *CatalogModelPricing) (*ModelPricing, error) {
	// 标准化模型名称（转小写）
	model = strings.ToLower(model)
	if catalogPrice == nil || catalogPrice.Source == "unpriced" || catalogPrice.TokenPricingAbsent {
		return nil, fmt.Errorf("%w for model: %s", ErrModelPricingUnavailable, model)
	}

	catalogPricing := catalogPrice
	rules := catalogPricing.Clone()
	inclusiveThreshold := strings.EqualFold(catalogPricing.Provider, "xai")
	var contextPrices []ContextModelPrice
	for _, tier := range catalogPricing.ContextPrices {
		if tier.Pricing == nil {
			continue
		}
		price, err := ResolveModelPricing(model, tier.Pricing)
		if err == nil {
			threshold := tier.Threshold
			// 内部阶梯统一使用 (min,max]；xAI 达到阈值即切档，先转换成前一档的最大 token 数。
			if inclusiveThreshold && threshold > 0 {
				threshold--
			}
			contextPrices = append(contextPrices, ContextModelPrice{Threshold: threshold, Pricing: price})
		}
	}
	// models.dev 明确给出 1h 单价时按 TTL 拆分，显式零价也有效。
	// 旧格式沿用 1h 高于 5m 的兼容判断。
	price5m := catalogPricing.CacheCreationInputTokenCost
	price1h := catalogPricing.CacheCreationInputTokenCostAbove1hr
	enableBreakdown := price1h > 0 && price1h > price5m || catalogPricing.CacheCreation1hPricePresent
	return &ModelPricing{
		FastMultiplier:                     rules.FastMultiplier,
		FlexMultiplier:                     rules.FlexMultiplier,
		TimePricing:                        rules.TimePricing,
		CatalogSource:                      catalogPricing.Source,
		CacheCreationPriceExplicit:         catalogPricing.Source != "" && catalogPricing.CacheCreationPricePresent && catalogPricing.CacheCreationInputTokenCost == 0,
		PriorityInputPresent:               catalogPricing.PriorityInputPresent,
		PriorityOutputPresent:              catalogPricing.PriorityOutputPresent,
		PriorityCacheReadPresent:           catalogPricing.PriorityCacheReadPresent,
		PriorityCacheWritePresent:          catalogPricing.PriorityCacheWritePresent,
		ContextPrices:                      contextPrices,
		InputPricePerToken:                 catalogPricing.InputCostPerToken,
		InputPricePerTokenPriority:         catalogPricing.InputCostPerTokenPriority,
		OutputPricePerToken:                catalogPricing.OutputCostPerToken,
		OutputPricePerTokenPriority:        catalogPricing.OutputCostPerTokenPriority,
		CacheCreationPricePerToken:         catalogPricing.CacheCreationInputTokenCost,
		CacheCreationPricePerTokenPriority: catalogPricing.CacheCreationInputTokenCostPriority,
		CacheReadPricePerToken:             catalogPricing.CacheReadInputTokenCost,
		CacheReadPricePerTokenPriority:     catalogPricing.CacheReadInputTokenCostPriority,
		CacheCreation5mPrice:               price5m,
		CacheCreation1hPrice:               price1h,
		SupportsCacheBreakdown:             enableBreakdown,
		SupportsServiceTier:                catalogPricing.SupportsServiceTier,
		// xAI 的目录语义是达到阈值即进入高档，其他提供商保持严格大于。
		LongContextThresholdInclusive: inclusiveThreshold,
		LongContextInputThreshold:     catalogPricing.LongContextInputTokenThreshold,
		LongContextInputMultiplier:    catalogPricing.LongContextInputCostMultiplier,
		LongContextOutputMultiplier:   catalogPricing.LongContextOutputCostMultiplier,
		ImageInputPricePerToken:       catalogPricing.InputCostPerImageToken,
		ImageOutputPricePerToken:      catalogPricing.OutputCostPerImageToken,
		ImageOutputPriceExplicit:      catalogPricing.ImageOutputPricePresent,
		MaxReasoningEffortMultiplier:  rules.MaxReasoningEffortMultiplier,
	}, nil
}
