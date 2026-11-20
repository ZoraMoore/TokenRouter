package app

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/billing"
	"github.com/TokenFlux/TokenRouter/internal/billing/pricing"
	"github.com/TokenFlux/TokenRouter/internal/config"
	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
	"github.com/TokenFlux/TokenRouter/internal/routing"
	routinghttp "github.com/TokenFlux/TokenRouter/internal/routing/httpapi"
	routingdto "github.com/TokenFlux/TokenRouter/internal/routing/httpapi/dto"
	routingpostgres "github.com/TokenFlux/TokenRouter/internal/routing/postgres"
	"github.com/TokenFlux/TokenRouter/internal/settings"
	"github.com/TokenFlux/TokenRouter/internal/site"
	"github.com/TokenFlux/TokenRouter/internal/usage"
)

type marketplacePrices struct {
	resolver   *billing.PriceResolver
	calculator *billing.Calculator
}

// marketplaceStats 返回公开首页需要的 Dashboard 计数。
type marketplaceStats struct {
	source *usage.DashboardService
	store  *settings.Store
}

// provideMarketplace 绑定分组、设置和报价查询，平台模型信息在查询时读取。
func provideMarketplace(groups *routingpostgres.GroupStore, store *settings.Store, catalogue *routing.RequestableCatalogue, prices *billing.PriceResolver, calculator *billing.Calculator, capacity *routing.CapacityService, availability routing.GroupAvailabilityProbeRepository, cfg *config.Config, attributes *routing.ModelAttributeService) *routing.Marketplace {
	options := routing.MarketplaceOptions{Attributes: attributes.ResolveGroups, Timezone: cfg.Timezone, Now: time.Now, Warn: slog.Warn}
	return routing.NewMarketplace(groups, store, catalogue, catalogue.Resolver, marketplacePrices{prices, calculator}, capacity, availability, options)
}

// PublicStats 按管理员选中的子项返回计数。模型数由首页用公开模型列表计算。
func (s marketplaceStats) PublicStats(ctx context.Context) (routingdto.ModelMarketplaceStats, error) {
	metrics, enabled := site.ReadPublicHomeStatsMetrics(ctx, s.store)
	if !enabled {
		return routingdto.ModelMarketplaceStats{}, apperror.Forbidden("PUBLIC_HOME_STATS_DISABLED", "Public home page stats are disabled")
	}
	showTodayTokens := slices.Contains(metrics, site.HomeStatsTodayTokens)
	showTotalTokens := slices.Contains(metrics, site.HomeStatsTotalTokens)
	showTotalUsers := slices.Contains(metrics, site.HomeStatsTotalUsers)
	stats := routingdto.ModelMarketplaceStats{ShowSupportedModels: slices.Contains(metrics, site.HomeStatsSupportedModels)}
	if !showTodayTokens && !showTotalTokens && !showTotalUsers {
		return stats, nil
	}
	value, err := s.source.GetPublicDashboardStats(ctx)
	if err != nil {
		return routingdto.ModelMarketplaceStats{}, err
	}
	if showTodayTokens {
		stats.TodayTokens = &value.TodayTokens
	}
	if showTotalTokens {
		stats.TotalTokens = &value.TotalTokens
	}
	if showTotalUsers {
		stats.TotalUsers = &value.TotalUsers
	}
	return stats, nil
}

func (p marketplacePrices) Quote(ctx context.Context, request routing.MarketplaceQuoteRequest) pricing.ModelDisplayPricing {
	return p.resolver.PublicQuote(ctx, billing.PublicQuoteInput{PricingInput: billing.PricingInput{
		Model: request.Model, GroupID: &request.GroupID,
	}, RateMultiplier: request.RateMultiplier, FreeFastApplicable: request.FreeFastApplicable})
}

func (p marketplacePrices) GetModelModalities(model string) ([]string, []string) {
	return p.calculator.GetModelModalities(model)
}

// provideMarketplaceHTTP 为市场 Handler 绑定公开统计查询。
func provideMarketplaceHTTP(core *routing.Marketplace, dashboard *usage.DashboardService, store *settings.Store) *routinghttp.MarketplaceHandler {
	return routinghttp.NewMarketplaceHandler(core, marketplaceStats{source: dashboard, store: store})
}
