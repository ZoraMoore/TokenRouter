package pricing

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/modelcatalog"
)

// modelsDevFastContextFixture 固定 Astra 的标准、Fast 与长上下文费率，回归测试离线运行。
const modelsDevFastContextFixture = `{"models":{"openai/gpt-6-astra":{}},"providers":{"openai":{"models":{
	"gpt-6-astra":{
		"cost":{"input":10,"output":50,"cache_read":1,"cache_write":12.5,
			"tiers":[{"tier":{"type":"context","size":272000},"input":20,"output":75,"cache_read":2,"cache_write":25}]},
		"experimental":{"modes":{"fast":{"cost":{"input":20,"output":100,"cache_read":2,"cache_write":25}}}}
	}
}}}}`

// TestModelsDevFastContextCosts 覆盖实际少计请求与包含缓存的上下文阈值。
func TestModelsDevFastContextCosts(t *testing.T) {
	catalog, err := modelcatalog.Parse([]byte(modelsDevFastContextFixture))
	require.NoError(t, err)
	entries, diagnostics, err := ParsePricingEntries(ModelsDevPrices(catalog))
	require.NoError(t, err)
	require.NoError(t, diagnostics.ValidationError())
	price, err := ResolveModelPricing("gpt-6-astra", entries["gpt-6-astra"])
	require.NoError(t, err)

	for _, tc := range []struct {
		name        string
		tokens      UsageTokens
		tier        string
		disableLong bool
		want        float64
		wantLong    bool
	}{
		{name: "short fast", tokens: UsageTokens{InputTokens: 100000, OutputTokens: 262}, tier: "priority", want: 2.0262},
		{name: "boundary includes both caches", tokens: UsageTokens{InputTokens: 100000, CacheReadTokens: 100000, CacheCreationTokens: 72000, OutputTokens: 1000}, tier: "priority", want: 4.1},
		{name: "above boundary includes both caches", tokens: UsageTokens{InputTokens: 100000, CacheReadTokens: 100000, CacheCreationTokens: 72001, OutputTokens: 1000}, tier: "priority", want: 8.15005, wantLong: true},
		{name: "uncached production regression", tokens: UsageTokens{InputTokens: 315620, OutputTokens: 262}, tier: "priority", want: 12.6641, wantLong: true},
		{name: "cached production regression", tokens: UsageTokens{InputTokens: 4335, CacheReadTokens: 315392, OutputTokens: 316}, tier: "priority", want: 1.482368, wantLong: true},
		{name: "fast alias", tokens: UsageTokens{InputTokens: 315620, OutputTokens: 262}, tier: "fast", want: 12.6641, wantLong: true},
		{name: "standard long context", tokens: UsageTokens{InputTokens: 315620, OutputTokens: 262}, want: 6.33205, wantLong: true},
		{name: "long context disabled", tokens: UsageTokens{InputTokens: 315620, OutputTokens: 262}, tier: "priority", disableLong: true, want: 6.3386},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cost := ComputeTokenBreakdown(price, tc.tokens, 0.2, tc.tier, !tc.disableLong)
			require.InDelta(t, tc.want, cost.TotalCost, 1e-10)
			require.InDelta(t, tc.want*0.2, cost.ActualCost, 1e-10)
			require.Equal(t, tc.wantLong, cost.LongContextBillingApplied)
		})
	}
}

// TestModelsDevFastContextPriceOverrides 同一目录价格同时用于提供商成本、用户价卡和价格展示。
func TestModelsDevFastContextPriceOverrides(t *testing.T) {
	catalog, err := modelcatalog.Parse([]byte(modelsDevFastContextFixture))
	require.NoError(t, err)
	entries, _, err := ParsePricingEntries(ModelsDevPrices(catalog))
	require.NoError(t, err)
	price, err := ResolveModelPricing("gpt-6-astra", entries["gpt-6-astra"])
	require.NoError(t, err)
	inputPrice, outputPrice, cachePrice, priceMultiplier := 10e-6, 50e-6, 1e-6, 1.125
	fastMultiplier := 3.0
	for _, tc := range []struct {
		name string
		card *ModelPricingEntry
		want float64
	}{
		{name: "catalog", want: 12.6641},
		{name: "custom base prices and markup", card: &ModelPricingEntry{
			BillingMode: BillingModeToken, InputPrice: &inputPrice, OutputPrice: &outputPrice,
			CacheReadPrice: &cachePrice, PriceMultiplier: &priceMultiplier,
		}, want: 14.2471125},
		{name: "explicit fast multiplier", card: &ModelPricingEntry{BillingMode: BillingModeToken, FastMultiplier: &fastMultiplier}, want: 18.99615},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resolved := ResolvePriceCards(tc.card, price, PricingSourceCatalog, true)
			cost, err := CalculateTokenCost(resolved, CostInput{
				Model: "gpt-6-astra", Tokens: UsageTokens{InputTokens: 315620, OutputTokens: 262},
				RateMultiplier: 0.2, ServiceTier: "priority",
			})
			require.NoError(t, err)
			require.InDelta(t, tc.want, cost.TotalCost, 1e-10)
			require.InDelta(t, tc.want*0.2, cost.ActualCost, 1e-10)
		})
	}
	display := BuildTokenDisplayPricing(price, 0.2)
	intervals := display.ContextIntervals
	require.Len(t, intervals, 2)
	require.InDelta(t, 8e-6, intervals[1].FastInputPricePerToken, 1e-12)
	require.InDelta(t, 30e-6, intervals[1].FastOutputPricePerToken, 1e-12)
	require.InDelta(t, 0.8e-6, intervals[1].FastCacheReadPricePerToken, 1e-12)
	// 价卡覆盖与展示换算完成后，目录基础价仍可用于下一次请求。
	require.InDelta(t, 20e-6, price.InputPricePerTokenPriority, 1e-12)
	require.InDelta(t, 40e-6, price.ContextPrices[0].Pricing.InputPricePerTokenPriority, 1e-12)
}

// TestModelsDevFastContextMultipleTiers 每个上下文档位从基础价换算，防止后续档位重复加倍。
func TestModelsDevFastContextMultipleTiers(t *testing.T) {
	catalog, err := modelcatalog.Parse([]byte(`{"providers":{"openai":{"models":{"gpt-tier-test":{
		"cost":{"input":2,"output":10,"cache_read":0.2,"tiers":[
			{"tier":{"type":"context","size":100},"input":4,"output":15,"cache_read":0.4},
			{"tier":{"type":"context","size":200},"input":6,"output":20,"cache_read":0.6}]},
		"experimental":{"modes":{"fast":{"cost":{"input":4,"output":20,"cache_read":0.4}}}}
	}}}}}`))
	require.NoError(t, err)
	entries, _, err := ParsePricingEntries(ModelsDevPrices(catalog))
	require.NoError(t, err)
	price, err := ResolveModelPricing("gpt-tier-test", entries["gpt-tier-test"])
	require.NoError(t, err)
	for _, tc := range []struct {
		input int
		want  float64
	}{
		{100, 0.0006},
		{101, 0.001108},
		{200, 0.0019},
		{201, 0.002812},
	} {
		cost := ComputeTokenBreakdown(price, UsageTokens{InputTokens: tc.input, OutputTokens: 10}, 1, "priority", true)
		require.InDelta(t, tc.want, cost.TotalCost, 1e-12, "input=%d", tc.input)
	}
}

// TestModelsDevFastContextZeroAndMissingPrices 区分免费与缺价，基础零价无法推导倍率时使用明确的 Fast 单价。
func TestModelsDevFastContextZeroAndMissingPrices(t *testing.T) {
	const fixture = `{"providers":{"openai":{"models":{
		"gpt-zero-fast":{
			"cost":{"input":2,"output":10,"cache_read":0.2,"cache_write":2.5,"tiers":[{"tier":{"type":"context","size":272000},"input":4,"output":15,"cache_read":0.4,"cache_write":5}]},
			"experimental":{"modes":{"fast":{"cost":{"input":0,"output":0,"cache_read":0,"cache_write":0}}}}
		},
		"gpt-zero-base":{
			"cost":{"input":0,"output":10,"cache_read":0,"tiers":[{"tier":{"type":"context","size":272000},"input":4,"output":15,"cache_read":0.4,"cache_write":5}]},
			"experimental":{"modes":{"fast":{"cost":{"input":4,"output":20,"cache_read":0.4,"cache_write":6}}}}
		},
		"gpt-partial-fast":{
			"cost":{"input":2,"output":10,"cache_read":0.2,"cache_write":2.5,"tiers":[{"tier":{"type":"context","size":272000},"input":4,"output":15,"cache_read":0.4,"cache_write":0}]},
			"experimental":{"modes":{"fast":{"cost":{"input":4,"output":20}}}}
		},
		"gpt-free-long-cache":{
			"cost":{"input":2,"output":10,"cache_read":0.2,"cache_write":2.5,"tiers":[{"tier":{"type":"context","size":272000},"input":4,"output":15,"cache_read":0,"cache_write":0}]},
			"experimental":{"modes":{"fast":{"cost":{"input":4,"output":20,"cache_read":0.4,"cache_write":5}}}}
		}
	}}}}`
	catalog, err := modelcatalog.Parse([]byte(fixture))
	require.NoError(t, err)
	before, err := modelcatalog.Parse([]byte(fixture))
	require.NoError(t, err)
	entries, _, err := ParsePricingEntries(ModelsDevPrices(catalog))
	require.NoError(t, err)
	for _, tc := range []struct {
		model string
		want  float64
	}{
		{"gpt-zero-fast", 0},
		{"gpt-zero-base", 1.2364},
		{"gpt-partial-fast", 2.4304},
		{"gpt-free-long-cache", 2.43},
	} {
		t.Run(tc.model, func(t *testing.T) {
			price, err := ResolveModelPricing(tc.model, entries[tc.model])
			require.NoError(t, err)
			cost := ComputeTokenBreakdown(price, UsageTokens{
				InputTokens: 300000, OutputTokens: 1000, CacheReadTokens: 1000, CacheCreationTokens: 1000,
			}, 1, "priority", true)
			require.InDelta(t, tc.want, cost.TotalCost, 1e-12)
		})
	}
	require.Equal(t, before, catalog)
}

func TestModelsDevContextPricesAndCacheTTL(t *testing.T) {
	catalog, err := modelcatalog.Parse([]byte(`{"models":{"anthropic/claude-test":{"name":"Claude"}},"providers":{"anthropic":{"models":{"claude-test":{"cost":{"input":3,"output":15,"cache_write":3.75,"cache_read":0.3,"tiers":[{"tier":{"type":"context","size":100},"input":6,"output":30,"cache_write":7.5,"cache_read":0},{"tier":{"type":"context","size":200},"input":9,"output":45,"cache_write":11.25,"cache_read":0.9}]}}}}}}`))
	require.NoError(t, err)
	entries, _, err := ParsePricingEntries(ModelsDevPrices(catalog))
	require.NoError(t, err)
	base, err := ResolveModelPricing("claude-test", entries["claude-test"])
	require.NoError(t, err)
	for _, tc := range []struct {
		input int
		price float64
	}{{100, 3}, {101, 6}, {200, 6}, {201, 9}} {
		cost := ComputeTokenBreakdown(base, UsageTokens{InputTokens: tc.input}, 1, "", true)
		require.InDelta(t, float64(tc.input)*tc.price/1e6, cost.TotalCost, 1e-12)
	}
	cache := ComputeTokenBreakdown(base, UsageTokens{CacheCreationTokens: 30, CacheCreation5mTokens: 10, CacheCreation1hTokens: 20}, 1, "", true)
	require.InDelta(t, 30*3.75/1e6, cache.TotalCost, 1e-12)
	intervals := LongContextDisplayPricingIntervals(base, 2)
	require.Len(t, intervals, 3)
	require.InDelta(t, 12e-6, intervals[1].InputPricePerToken, 1e-12)
	require.Equal(t, 0.0, intervals[1].CacheReadPricePerToken)
	require.Nil(t, WithoutLongContextDisplayPricing(base).ContextPrices)
	paid := MultiplyModelPricing(base, 2)
	require.InDelta(t, 101*12e-6, ComputeTokenBreakdown(paid, UsageTokens{InputTokens: 101}, 1, "", true).TotalCost, 1e-12)
}

func TestModelsDevExplicitFreeFastAndCacheTTL(t *testing.T) {
	catalog, err := modelcatalog.Parse([]byte(`{"models":{"anthropic/claude-test":{"name":"Claude"},"openai/gpt-5.5":{"name":"GPT"}},"providers":{"anthropic":{"models":{"claude-test":{"cost":{"input":3,"output":15,"cache_write":3.75},"experimental":{"modes":{"fast":{"cost":{"input":6,"output":30,"cache_write":7.5}}}}}}},"openai":{"models":{"gpt-5.5":{"cost":{"input":3,"output":15},"experimental":{"modes":{"fast":{"cost":{"input":0,"output":0}}}}}}}}}`))
	require.NoError(t, err)
	entries, _, err := ParsePricingEntries(ModelsDevPrices(catalog))
	require.NoError(t, err)
	claude, err := ResolveModelPricing("claude-test", entries["claude-test"])
	require.NoError(t, err)
	usage := UsageTokens{CacheCreationTokens: 30, CacheCreation5mTokens: 10, CacheCreation1hTokens: 20}
	cost := ComputeTokenBreakdown(claude, usage, 1, "priority", true)
	require.InDelta(t, 30*7.5/1e6, cost.TotalCost, 1e-12)
	fast, ok := FastModeDisplayPricing(claude)
	require.True(t, ok)
	require.Zero(t, fast.CacheCreation1hPrice)
	gpt, err := ResolveModelPricing("gpt-5.5", entries["gpt-5.5"])
	require.NoError(t, err)
	require.Zero(t, ComputeTokenBreakdown(gpt, UsageTokens{InputTokens: 10, OutputTokens: 10}, 1, "priority", true).TotalCost)
}

func TestModelsDevExplicitZeroOneHourCachePrice(t *testing.T) {
	entries, _, err := ParsePricingEntries(map[string]json.RawMessage{"claude-test": json.RawMessage(`{"source":"models.dev","input_cost_per_token":0.000003,"output_cost_per_token":0.000015,"cache_creation_input_token_cost":0.00000375,"cache_creation_input_token_cost_above_1hr":0}`)})
	require.NoError(t, err)
	base, err := ResolveModelPricing("claude-test", entries["claude-test"])
	require.NoError(t, err)
	require.True(t, base.SupportsCacheBreakdown)
	require.Zero(t, ComputeTokenBreakdown(base, UsageTokens{CacheCreationTokens: 10, CacheCreation1hTokens: 10}, 1, "", true).TotalCost)
}

func TestModelsDevZeroCacheWriteDoesNotUseModelFallback(t *testing.T) {
	entries, _, err := ParsePricingEntries(map[string]json.RawMessage{"gpt-5.6-sol": json.RawMessage(`{"source":"models.dev","input_cost_per_token":0.000003,"output_cost_per_token":0.000015,"cache_creation_input_token_cost":0}`)})
	require.NoError(t, err)
	base, err := ResolveModelPricing("gpt-5.6-sol", entries["gpt-5.6-sol"])
	require.NoError(t, err)
	require.Zero(t, ComputeTokenBreakdown(base, UsageTokens{CacheCreationTokens: 10}, 1, "", true).TotalCost)
}

// TestModelsDevImageOutputScope 限定生图费率解释范围，避免根据图片模态改写研究模型或中继报价。
func TestModelsDevImageOutputScope(t *testing.T) {
	catalog, err := modelcatalog.Parse([]byte(`{"providers":{
		"google":{"models":{
			"gemini-image-test":{"cost":{"input":2,"output":120},"modalities":{"output":["text","image"]}},
			"deep-research-test":{"cost":{"input":2,"output":12},"modalities":{"output":["text","image"]}},
			"gemini-text-test":{"cost":{"input":2,"output":12},"modalities":{"output":["text"]}},
			"gemini-omni-test":{"cost":{"input":2,"output":12},"modalities":{"output":["text","image","video"]}}
		}},
		"openrouter":{"models":{"gemini-image-test":{"cost":{"input":1,"output":9},"modalities":{"output":["text","image"]}}}}
	}}`))
	require.NoError(t, err)
	entries, _, err := ParsePricingEntries(ModelsDevPrices(catalog))
	require.NoError(t, err)
	for model, price := range map[string]float64{"deep-research-test": 12e-6, "gemini-text-test": 12e-6, "gemini-omni-test": 12e-6, "openrouter/gemini-image-test": 9e-6} {
		value, err := ResolveModelPricing(model, entries[model])
		require.NoError(t, err)
		require.InDelta(t, price, value.OutputPricePerToken, 1e-12)
	}
	// 已知缺少文本价时，不借用跨模型或旧媒体兜底价。
	_, err = ResolveModelPricing("gemini-image-test", entries["gemini-image-test"])
	require.ErrorIs(t, err, ErrModelPricingUnavailable)
}
