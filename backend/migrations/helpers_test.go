package migrations_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/billing/pricing"
)

// pricingMergeConflict 记录冲突模型和条目编号，供迁移预检定位需要人工处理的价卡。
type pricingMergeConflict struct {
	Models   []string `json:"models"`
	EntryIDs []int64  `json:"entry_ids"`
	Reason   string   `json:"reason"`
}

// mergePriceCards 合并同一作用域的价卡，对无法比较的配置返回冲突清单。
// 各项单价取较高值，空值表示继承。不同作用域的提供商成本规则需要分别调用。
func mergePriceCards(entries []pricing.ModelPricingEntry) ([]pricing.ModelPricingEntry, []pricingMergeConflict) {
	type mergedEntry struct {
		card pricing.ModelPricingEntry
		ids  []int64
	}
	byModel := make(map[string]*mergedEntry)
	var conflicts []pricingMergeConflict
	for _, entry := range entries {
		for _, model := range entry.Models {
			model = pricing.NormalizePriceModelName(model)
			if model == "" || strings.Contains(strings.TrimSuffix(model, "*"), "*") {
				conflicts = append(conflicts, pricingMergeConflict{Models: []string{model}, EntryIDs: []int64{entry.ID}, Reason: "invalid model pattern"})
				continue
			}
			card := entry.Clone()
			card.Models = []string{model}
			if card.BillingMode == "" {
				card.BillingMode = pricing.BillingModeToken
			}
			previous := byModel[model]
			if previous == nil {
				byModel[model] = &mergedEntry{card: card, ids: []int64{entry.ID}}
				continue
			}
			previous.ids = append(previous.ids, entry.ID)
			merged, reason := mergePriceCard(previous.card, card)
			if reason != "" {
				conflicts = append(conflicts, pricingMergeConflict{Models: []string{model}, EntryIDs: append([]int64(nil), previous.ids...), Reason: reason})
				continue
			}
			previous.card = merged
		}
	}
	models := make([]string, 0, len(byModel))
	for model := range byModel {
		models = append(models, model)
	}
	sort.Strings(models)
	for i, left := range models {
		for _, right := range models[i+1:] {
			if modelPatternsOverlap(left, right) {
				ids := append([]int64(nil), byModel[left].ids...)
				ids = append(ids, byModel[right].ids...)
				conflicts = append(conflicts, pricingMergeConflict{Models: []string{left, right}, EntryIDs: ids, Reason: "overlapping model patterns"})
			}
		}
	}
	if len(conflicts) > 0 {
		return nil, conflicts
	}
	out := make([]pricing.ModelPricingEntry, 0, len(models))
	for _, model := range models {
		out = append(out, byModel[model].card)
	}
	return out, nil
}

// modelPatternsOverlap 检查精确名称和末尾通配符能否匹配同一模型。
func modelPatternsOverlap(left, right string) bool {
	return (strings.HasSuffix(left, "*") && strings.HasPrefix(strings.TrimSuffix(right, "*"), strings.TrimSuffix(left, "*"))) ||
		(strings.HasSuffix(right, "*") && strings.HasPrefix(strings.TrimSuffix(left, "*"), strings.TrimSuffix(right, "*")))
}

// mergePriceCard 先核对非价格规则，再对相同计量单位中已填写的单价取较高值。
func mergePriceCard(left, right pricing.ModelPricingEntry) (pricing.ModelPricingEntry, string) {
	l, err := comparablePriceRules(left)
	if err != nil {
		return pricing.ModelPricingEntry{}, err.Error()
	}
	r, err := comparablePriceRules(right)
	if err != nil {
		return pricing.ModelPricingEntry{}, err.Error()
	}
	if !bytes.Equal(l, r) {
		return pricing.ModelPricingEntry{}, "billing mode, intervals, multipliers or time rules differ"
	}
	merged := left.Clone()
	destination := cardUnitPrices(&merged)
	source := cardUnitPrices(&right)
	for i := range destination {
		a, b := *destination[i], *source[i]
		if (a == nil) != (b == nil) {
			return pricing.ModelPricingEntry{}, "inherited and explicit prices cannot be compared"
		}
		if a != nil {
			if math.IsNaN(*a) || math.IsNaN(*b) || math.IsInf(*a, 0) || math.IsInf(*b, 0) || *a < 0 || *b < 0 {
				return pricing.ModelPricingEntry{}, "invalid unit price"
			}
			value := math.Max(*a, *b)
			*destination[i] = &value
		}
	}
	return merged, ""
}

// cardUnitPrices 返回价卡和各计费区间中的单价字段指针。
func cardUnitPrices(card *pricing.ModelPricingEntry) []**float64 {
	prices := []**float64{&card.InputPrice, &card.OutputPrice, &card.CacheWritePrice, &card.CacheWrite1hPrice, &card.CacheReadPrice, &card.ImageInputPrice, &card.ImageOutputPrice, &card.PerRequestPrice}
	for i := range card.Intervals {
		interval := &card.Intervals[i]
		prices = append(prices, &interval.InputPrice, &interval.OutputPrice, &interval.CacheWritePrice, &interval.CacheWrite1hPrice, &interval.CacheReadPrice, &interval.PerRequestPrice)
	}
	return prices
}

// comparablePriceRules 将价卡的非价格规则序列化为 JSON，供合并时比较。
func comparablePriceRules(card pricing.ModelPricingEntry) ([]byte, error) {
	card = card.Clone()
	card.ID, card.PricingConfigID = 0, 0
	card.Models = nil
	card.CreatedAt, card.UpdatedAt = time.Time{}, time.Time{}
	for i := range card.Intervals {
		interval := &card.Intervals[i]
		interval.ID, interval.PricingID = 0, 0
		interval.CreatedAt, interval.UpdatedAt = time.Time{}, time.Time{}
	}
	for _, price := range cardUnitPrices(&card) {
		*price = nil
	}
	data, err := json.Marshal(card)
	if err != nil {
		return nil, fmt.Errorf("invalid pricing rules: %w", err)
	}
	return data, nil
}
