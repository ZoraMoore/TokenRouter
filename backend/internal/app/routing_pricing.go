package app

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/apikey"
	"github.com/TokenFlux/TokenRouter/internal/billing"
	pricingprovider "github.com/TokenFlux/TokenRouter/internal/billing/provider"
	catalogprovider "github.com/TokenFlux/TokenRouter/internal/modelcatalog/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing"
	routingpostgres "github.com/TokenFlux/TokenRouter/internal/routing/postgres"
)

func providePricingConfigService(repo *routingpostgres.PricingConfigStore, groups *routingpostgres.GroupStore, invalidator apikey.APIKeyAuthCacheInvalidator) *routing.PricingConfigService {
	return routing.NewPricingConfigService(repo, invalidator, routing.PricingConfigOptions{Warn: slog.Warn, Now: time.Now, LoadLocation: pricingprovider.LoadPricingLocation, ReadGroup: func(ctx context.Context, id int64) (*routing.Group, error) {
		access, ok := apikey.AccessSnapshotFromContext(ctx)
		if ok {
			key := access.KeyView()
			if key != nil && key.Group != nil && key.Group.ID == id && key.Group.Hydrated {
				return key.Group, nil
			}
		}
		return groups.GetByIDLite(ctx, id)
	}})
}

// providePricingCatalog 将目录版本与后台分页读取绑定。
func providePricingCatalog(calculator *billing.Calculator, prices *catalogprovider.Service) *routing.PricingCatalog {
	var mu sync.Mutex
	var index *routing.DefaultPricingIndex
	var generation uint64
	currentIndex := func() (catalogprovider.DefaultPricingView, *routing.DefaultPricingIndex) {
		mu.Lock()
		defer mu.Unlock()
		// 在索引锁内读取目录，索引按发布代数向前更新。
		view := prices.DefaultPricingView()
		if index == nil || generation != view.Generation {
			index = buildDefaultPricingIndex(view)
			generation = view.Generation
		}
		return view, index
	}
	return &routing.PricingCatalog{
		Prices: calculator,
		Update: calculator.ForceUpdatePricing,
		List: func(query routing.DefaultPricingQuery) routing.DefaultPricingPage {
			view, index := currentIndex()
			frozen := calculator.WithPriceCatalog(view)
			result := index.Page(query, frozen.DefaultModelPrice)
			result.UpdatedAt, result.Version, result.LastError = view.UpdatedAt, view.Version, view.LastError
			return result
		},
	}
}

// buildDefaultPricingIndex 计算与时刻无关的筛选字段。
func buildDefaultPricingIndex(view catalogprovider.DefaultPricingView) *routing.DefaultPricingIndex {
	var models []routing.DefaultPricingModel
	view.EachModel(func(model, platform, mode string) {
		if strings.Contains(model, "grok-imagine-video") {
			mode = "video"
		}
		models = append(models, routing.DefaultPricingModel{
			Model:       model,
			Platform:    platform,
			Mode:        mode,
			BillingMode: billing.DefaultModelBillingMode(model, mode, view.GetModelPricing(model)),
		})
	})
	return routing.NewDefaultPricingIndex(models)
}
