package billing

import (
	"maps"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/billing/pricing"
)

// WithPriceCatalog 固定本次报价使用的目录和时刻。
func (s *Calculator) WithPriceCatalog(catalog CatalogPriceReader) *Calculator {
	out := *s
	out.catalog = catalog
	now := s.options.Now()
	out.options.Now = func() time.Time { return now }
	return &out
}

// DefaultModelPrice 返回模型的基础计费规则，配置的零价按已定价处理。
func (s *Calculator) DefaultModelPrice(model, platform, mode string) pricing.DefaultModelPrice {
	raw := s.RawModelPricing(model)
	billingMode, hasMediaPrice := defaultModelBillingMode(model, mode, raw)
	result := pricing.DefaultModelPrice{Model: model, Platform: platform, BillingMode: billingMode, PriceStatus: "unpriced", Prices: []pricing.DefaultPriceValue{}}
	add := func(key string, value float64, unit string) {
		result.Prices = append(result.Prices, pricing.DefaultPriceValue{Key: key, Value: &value, Unit: unit})
	}
	if raw != nil {
		result.Source = raw.Source
		result.PriceSources = maps.Clone(raw.PriceSources)
	}
	if billingMode == "video" && hasMediaPrice {
		if result.PriceSources == nil {
			result.PriceSources = map[string]string{}
		}
		for _, size := range []string{"480p", "720p", "1080p"} {
			price, ok := pricing.DefaultVideoPrice(raw, size)
			if !ok {
				continue
			}
			add(size, price, "USD/s")
			source := raw.PriceSources["video_prices"]
			if source == "" {
				source = raw.Source
			}
			result.PriceSources[size] = source
		}
		result.PriceStatus = "priced"
		return result
	}
	if billingMode == "image" && hasMediaPrice {
		if result.PriceSources == nil {
			result.PriceSources = map[string]string{}
		}
		imageSource := result.PriceSources["image"]
		if imageSource == "" {
			imageSource = result.Source
		}
		if len(raw.ImagePrices) > 0 {
			if source := raw.PriceSources["image_prices"]; source != "" {
				imageSource = source
			}
		}
		for _, size := range []string{"1K", "2K", "4K"} {
			price, ok := pricing.DefaultImagePrice(raw, size)
			if !ok {
				continue
			}
			add(size, price, "USD/image")
			if imageSource != "" {
				result.PriceSources[size] = imageSource
			}
		}
		result.PriceStatus = "priced"
	}
	base, err := pricing.ResolveModelPricing(model, raw)
	if err != nil || base == nil {
		return result
	}
	result.PriceStatus = "priced"
	base = s.applyCatalogTimePricing(base, s.options.Now())
	// 用一个计费单位复用实际算法，避免展示层重新实现 Fast、缓存和图片 token 回退。
	// 长上下文报价需先越过阈值门槛才能触发倍率，与真实计费走同一条分支。
	longGateTokens := 0
	if base.LongContextInputThreshold > 0 {
		longGateTokens = base.LongContextInputThreshold
		if !base.LongContextThresholdInclusive {
			longGateTokens++
		}
	}
	addTokenPrices := func(prefix, tier string, applyLongCtx bool) {
		gate := 0
		if applyLongCtx {
			gate = longGateTokens
		}
		quote := func(tokens UsageTokens) *CostBreakdown {
			tokens.InputTokens += gate
			return pricing.ComputeTokenBreakdown(base, tokens, 1, tier, applyLongCtx)
		}
		perMTok := func(cost float64, tokens int) float64 {
			return cost / float64(tokens) * 1e6
		}
		addOptional := func(key string, value float64, applicable bool) {
			if applicable {
				add(prefix+key, value*1e6, "USD/MTok")
			} else {
				result.Prices = append(result.Prices, pricing.DefaultPriceValue{Key: prefix + key, Unit: "USD/MTok"})
			}
		}
		add(prefix+"input", perMTok(quote(UsageTokens{InputTokens: 1}).InputCost, 1+gate), "USD/MTok")
		add(prefix+"output", quote(UsageTokens{OutputTokens: 1}).OutputCost*1e6, "USD/MTok")
		readPresent := base.CacheReadPricePerToken > 0 || raw != nil && raw.CacheReadPricePresent
		writePresent := base.CacheCreationPricePerToken > 0 || base.SupportsCacheBreakdown || raw != nil && raw.CacheCreationPricePresent
		addOptional("cache_read", quote(UsageTokens{CacheReadTokens: 1}).CacheReadCost, readPresent)
		addOptional("cache_write", quote(UsageTokens{CacheCreationTokens: 1, CacheCreation5mTokens: 1}).CacheCreationCost, writePresent)
		addOptional("cache_write_1h", quote(UsageTokens{CacheCreationTokens: 1, CacheCreation1hTokens: 1}).CacheCreationCost, base.SupportsCacheBreakdown)
		addOptional("image_input", quote(UsageTokens{InputTokens: 1, ImageInputTokens: 1}).ImageInputCost, base.ImageInputPricePerToken > 0 || raw != nil && (raw.SupportsVision || raw.ImageInputPricePresent))
		addOptional("image_output", quote(UsageTokens{OutputTokens: 1, ImageOutputTokens: 1}).ImageOutputCost, base.ImageOutputPricePerToken > 0 || raw != nil && raw.ImageOutputPricePresent)
	}
	addTokenPrices("", "", false)
	hasFast := false
	if _, ok := pricing.FastModeDisplayPricing(base); ok {
		addTokenPrices("fast_", "priority", false)
		hasFast = true
	}
	hasFlex := base.FlexMultiplier != nil
	if hasFlex {
		addTokenPrices("flex_", "flex", false)
	}
	if base.MaxReasoningEffortMultiplier != nil {
		add("max_reasoning", *base.MaxReasoningEffortMultiplier, "multiplier")
	}
	// 长上下文价格投影为应用倍率后的绝对单价（含 Fast/Flex 组合），口径与
	// ShouldApplySessionLongContextPricing 一致：任一倍率大于 1 才存在长上下文阶梯。
	if base.LongContextInputThreshold > 0 && (base.LongContextInputMultiplier > 1 || base.LongContextOutputMultiplier > 1) {
		result.LongContextThreshold = base.LongContextInputThreshold
		result.LongContextThresholdInclusive = base.LongContextThresholdInclusive
		addTokenPrices("long_", "", true)
		if hasFast {
			addTokenPrices("long_fast_", "priority", true)
		}
		if hasFlex {
			addTokenPrices("long_flex_", "flex", true)
		}
	}

	if len(base.ContextPrices) > 0 {
		root, rootPrices := base, result.Prices
		start := 0
		for i := 0; i <= len(root.ContextPrices); i++ {
			var end *int
			if i < len(root.ContextPrices) {
				threshold := root.ContextPrices[i].Threshold
				end = &threshold
			}
			if i > 0 {
				base = root.ContextPrices[i-1].Pricing
			}
			result.Prices = nil
			addTokenPrices("", "", false)
			if _, ok := pricing.FastModeDisplayPricing(base); ok {
				addTokenPrices("fast_", "priority", false)
			}
			if base.SupportsServiceTier {
				addTokenPrices("flex_", "flex", false)
			}
			result.ContextIntervals = append(result.ContextIntervals, pricing.DefaultPriceInterval{MinTokens: start, MaxTokens: end, Prices: result.Prices})
			if end != nil {
				start = *end
			}
		}
		base, result.Prices = root, rootPrices
	}
	return result
}

// DefaultModelBillingMode 返回默认报价的计费模式，可用于分页前的筛选。
func DefaultModelBillingMode(model, mode string, raw *CatalogModelPricing) string {
	billingMode, _ := defaultModelBillingMode(model, mode, raw)
	return billingMode
}

// defaultModelBillingMode 优先使用模型的媒体单价，缺价时按 token 报价或目录模式展示。
func defaultModelBillingMode(model, mode string, raw *CatalogModelPricing) (billingMode string, hasMediaPrice bool) {
	if raw != nil && len(raw.VideoPrices) > 0 {
		return "video", true
	}
	if raw != nil {
		imageModel := mode == "image" || pricing.HasExplicitImageGenerationPricing(raw) || pricing.LooksLikeImageModel(model)
		if len(raw.ImagePrices) > 0 || pricing.HasImageUnitPrice(raw) && (imageModel || raw.TokenPricingAbsent) {
			return "image", true
		}
	}
	if pricing.HasCatalogTokenPricing(raw) {
		return "token", false
	}
	switch mode {
	case "video", "image":
		return mode, false
	case "image_generation":
		return "image", false
	default:
		return "token", false
	}
}
