package routing

import (
	"slices"
	"strings"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/billing/pricing"
)

type ModelPriceReader interface {
	GetModelPricing(string) (*ModelPricing, error)
}

// PricingCatalog 提供目录分页、单型号查价和手动更新。
type PricingCatalog struct {
	List   func(DefaultPricingQuery) DefaultPricingPage
	Update func() error
	Prices ModelPriceReader
}

// DefaultPricingQuery 包含管理页读取的筛选条件和页码。
type DefaultPricingQuery struct {
	Search      string
	Platform    string
	BillingMode string
	Page        int
	PageSize    int
}

// DefaultPricingPage 返回目录状态、平台选项和当前页的报价。
type DefaultPricingPage struct {
	Items     []pricing.DefaultModelPrice `json:"items"`
	Total     int                         `json:"total"`
	UpdatedAt time.Time                   `json:"last_updated"`
	Platforms []string                    `json:"platforms"`
	Version   string                      `json:"version"`
	LastError string                      `json:"last_error"`
}

// DefaultPricingModel 保存筛选字段和报价所需的目录模式。
type DefaultPricingModel struct {
	Model       string
	Platform    string
	Mode        string
	BillingMode string
}

type defaultPricingIndexRow struct {
	DefaultPricingModel
	searchableModel string
}

// DefaultPricingIndex 保存同一版目录的排序及筛选字段。
type DefaultPricingIndex struct {
	rows      []defaultPricingIndexRow
	platforms []string
}

type ModelPricing = pricing.ModelPricing

func (c *PricingCatalog) DefaultPricing(model string) (*ModelPricing, error) {
	return c.Prices.GetModelPricing(model)
}

// NewDefaultPricingIndex 按模型和平台排序，并从整份目录收集平台选项。
func NewDefaultPricingIndex(models []DefaultPricingModel) *DefaultPricingIndex {
	rows := make([]defaultPricingIndexRow, 0, len(models))
	platforms := make([]string, 0)
	seen := make(map[string]struct{})
	for _, model := range models {
		rows = append(rows, defaultPricingIndexRow{DefaultPricingModel: model, searchableModel: strings.ToLower(model.Model)})
		if _, exists := seen[model.Platform]; !exists {
			seen[model.Platform] = struct{}{}
			platforms = append(platforms, model.Platform)
		}
	}
	slices.SortFunc(rows, func(a, b defaultPricingIndexRow) int {
		if order := strings.Compare(a.Model, b.Model); order != 0 {
			return order
		}
		return strings.Compare(a.Platform, b.Platform)
	})
	slices.Sort(platforms)
	return &DefaultPricingIndex{rows: rows, platforms: platforms}
}

// Page 计算符合筛选条件的总数，并对当页模型调用 quote。
// @project-doc docs/interfaces/model_catalog_and_marketplace.md#gateway_default_pricing
func (i *DefaultPricingIndex) Page(query DefaultPricingQuery, quote func(model, platform, mode string) pricing.DefaultModelPrice) DefaultPricingPage {
	platform := strings.ToLower(strings.TrimSpace(query.Platform))
	search := strings.ToLower(strings.TrimSpace(query.Search))
	page, size := max(1, query.Page), query.PageSize
	if size <= 0 {
		size = 20
	}
	// 先比较页码，超页时把起点放在索引末尾，乘法不会溢出。
	start := len(i.rows)
	if page-1 <= len(i.rows)/size {
		start = (page - 1) * size
	}
	selected := make([]DefaultPricingModel, 0, min(size, len(i.rows)-start))
	total := 0
	for _, row := range i.rows {
		if platform != "" && row.Platform != platform || query.BillingMode != "" && row.BillingMode != query.BillingMode {
			continue
		}
		if search != "" && !strings.Contains(row.searchableModel, search) {
			continue
		}
		if total >= start && len(selected) < size {
			selected = append(selected, row.DefaultPricingModel)
		}
		total++
	}
	items := make([]pricing.DefaultModelPrice, 0, len(selected))
	for _, model := range selected {
		items = append(items, quote(model.Model, model.Platform, model.Mode))
	}
	return DefaultPricingPage{Items: items, Total: total, Platforms: slices.Clone(i.platforms)}
}
