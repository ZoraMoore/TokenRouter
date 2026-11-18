package app

import (
	"time"

	"github.com/TokenFlux/TokenRouter/internal/billing"
	"github.com/TokenFlux/TokenRouter/internal/config"
	"github.com/TokenFlux/TokenRouter/internal/infra/telemetry/logging"
)

// gatewayBillingRates 保存两套独立的完成倍率缓存，供完成器和应用清理任务共享。
type gatewayBillingRates struct {
	Forward *billing.GroupRateResolver
	OpenAI  *billing.GroupRateResolver
}

func provideGatewayBillingRates(repo billing.UserGroupRateRepository, cfg *config.Config) *gatewayBillingRates {
	ttl := gatewayGroupRateCacheTTL(cfg)
	return &gatewayBillingRates{
		Forward: billing.NewGroupRateResolver(repo, nil, ttl, nil, "service.gateway", logging.LegacyPrintf),
		OpenAI:  billing.NewGroupRateResolver(repo, nil, ttl, nil, "service.openai_gateway", logging.LegacyPrintf),
	}
}

// gatewayGroupRateCacheTTL 返回配置的正数 TTL，其他情况使用默认值。
func gatewayGroupRateCacheTTL(cfg *config.Config) time.Duration {
	if cfg == nil || cfg.Gateway.UserGroupRateCacheTTLSeconds <= 0 {
		return billing.DefaultGroupRateCacheTTL
	}
	return time.Duration(cfg.Gateway.UserGroupRateCacheTTLSeconds) * time.Second
}

// Expire 由应用时间轮调用，清理两套完成缓存，并随时间轮停止。
func (r *gatewayBillingRates) Expire() {
	if r != nil {
		r.Forward.DeleteExpired()
		r.OpenAI.DeleteExpired()
	}
}
