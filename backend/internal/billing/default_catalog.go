package billing

import (
	"maps"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/billing/pricing"
)

// WithPriceCatalog 在独立目录快照上复用相同算法、数据规则。
func (s *Calculator) WithPriceCatalog(catalog PriceCatalog) *Calculator {
	out := *s
	out.catalog = catalog
	now := s.options.Now()
	out.options.Now = func() time.Time { return now }
	return &out
}

// DefaultModelPrice 返回模型的基础计费规则，配置的零价按已定价处理。
func (s *Calculator) DefaultModelPrice(model, platform, mode string) pricing.DefaultModelPrice {
	result := pricing.DefaultModelPrice{Model: model, Platform: platform, BillingMode: "token", PriceStatus: "unpriced", Prices: []pricing.DefaultPriceValue{}}
	add := func(key string, value float64, unit string) {
		result.Prices = append(result.Prices, pricing.DefaultPriceValue{Key: key, Value: &value, Unit: unit})
	}
	raw := s.RawModelPricing(model)
	if raw != nil {
		result.Source = raw.Source
		result.PriceSources = maps.Clone(raw.PriceSources)
	}
	knownVideo := raw != nil && len(raw.VideoPrices) > 0
	if knownVideo {
		if result.PriceSources == nil {
			result.PriceSources = map[string]string{}
		}
		result.BillingMode = "video"
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
	knownImage := raw != nil && len(raw.ImagePrices) > 0
	imageModel := mode == "image" || pricing.HasExplicitImageGenerationPricing(raw) || pricing.LooksLikeImageModel(model)
	// 聊天模型附带的图片元数据不切换 token 模式；纯按张价和明确的图片模型使用媒体单位。
	hasImagePrice := knownImage || pricing.HasImageUnitPrice(raw) && (imageModel || raw.TokenPricingAbsent)
	switch mode {
	case "video", "image":
		result.BillingMode = mode
	case "image_generation":
		result.BillingMode = "image"
	}
	if hasImagePrice {
		if result.PriceSources == nil {
			result.PriceSources = map[string]string{}
		}
		result.BillingMode = "image"
		imageSource := result.PriceSources["image"]
		if imageSource == "" {
			imageSource = result.Source
		}
		if knownImage {
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
	// 媒体维度缺价时仍可展示已有 token 报价，不借用通用按张或按秒兜底。
	if !hasImagePrice {
		result.BillingMode = "token"
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
