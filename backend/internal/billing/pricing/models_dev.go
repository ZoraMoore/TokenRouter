package pricing

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/modelcatalog"
)

// ModelsDevPrices 将统一目录转换为计费字段，区分缺价和配置的零价。
func ModelsDevPrices(catalog *modelcatalog.Catalog) map[string]json.RawMessage {
	result := map[string]json.RawMessage{}
	for key, entry := range catalog.Entries {
		if entry.Cost.Input == nil && entry.Cost.Output == nil {
			continue
		}
		fields := modelsDevPriceFields(entry, entry.Cost)
		var tiers []map[string]any
		for _, tier := range entry.Tiers {
			cost := entry.Cost
			if tier.Input != nil {
				cost.Input = tier.Input
			}
			if tier.Output != nil {
				cost.Output = tier.Output
			}
			if tier.CacheRead != nil {
				cost.CacheRead = tier.CacheRead
			}
			if tier.CacheWrite != nil {
				cost.CacheWrite = tier.CacheWrite
			}
			tiers = append(tiers, map[string]any{"threshold": tier.Tier.Size, "pricing": modelsDevPriceFields(entry, cost)})
		}
		if len(tiers) > 0 {
			fields["context_prices"] = tiers
		}
		body, _ := json.Marshal(fields)
		result[key] = body
	}
	return result
}

func modelsDevPriceFields(entry modelcatalog.Entry, cost modelcatalog.Cost) map[string]any {
	provider := entry.Provider
	switch provider {
	case "google":
		provider = "gemini"
	case "moonshotai", "moonshotai-cn":
		provider = "moonshot"
	case "zai", "zhipuai":
		provider = "zhipu"
	}
	fields := map[string]any{"source": "models.dev", "provider": provider, "mode": "chat"}
	if entry.Attributes.InputModalities != nil {
		fields["supported_modalities"] = *entry.Attributes.InputModalities
	}
	if entry.Attributes.OutputModalities != nil {
		fields["supported_output_modalities"] = *entry.Attributes.OutputModalities
	}
	// 原厂 Gemini Image 系列的 output 是图片 token 费率，文本与思考价由本地补充提供。
	// Omni、Deep Research 和中继继续使用普通输出价，输出模态本身不决定价格单位。
	imageOutput := entry.FirstParty && entry.Provider == "google" && strings.HasPrefix(entry.Model, "gemini-") &&
		strings.Contains(entry.Model, "-image") && entry.Attributes.OutputModalities != nil &&
		slices.Contains(*entry.Attributes.OutputModalities, "image")
	put := func(key string, value *float64) {
		if value != nil {
			fields[key] = *value / 1e6
		}
	}
	put("input_cost_per_token", cost.Input)
	if imageOutput {
		put("output_cost_per_image_token", cost.Output)
	} else {
		put("output_cost_per_token", cost.Output)
	}
	put("cache_read_input_token_cost", cost.CacheRead)
	put("cache_creation_input_token_cost", cost.CacheWrite)
	fields["supports_prompt_caching"] = cost.CacheRead != nil || cost.CacheWrite != nil
	// 服务层级字段记录文本 token 价格，图片 Fast 价单独处理。
	if entry.Fast != nil && !imageOutput {
		fields["supports_service_tier"] = true
		put("input_cost_per_token_priority", modelsDevContextFastPrice(entry.Fast.Input, entry.Cost.Input, cost.Input))
		put("output_cost_per_token_priority", modelsDevContextFastPrice(entry.Fast.Output, entry.Cost.Output, cost.Output))
		put("cache_read_input_token_cost_priority", modelsDevContextFastPrice(entry.Fast.CacheRead, entry.Cost.CacheRead, cost.CacheRead))
		put("cache_creation_input_token_cost_priority", modelsDevContextFastPrice(entry.Fast.CacheWrite, entry.Cost.CacheWrite, cost.CacheWrite))
	}
	return fields
}

// modelsDevContextFastPrice 将基础 Fast 单价按对应标准价的上下文档位比例换算。
// 基础标准价缺失或为零时无法推导比例，使用目录给出的 Fast 单价。
func modelsDevContextFastPrice(fast, base, tier *float64) *float64 {
	if fast == nil || base == nil || tier == nil || *base <= 0 {
		return fast
	}
	price := *fast * (*tier / *base)
	return &price
}
