package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/app/lifecycle"
	"github.com/TokenFlux/TokenRouter/internal/config"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/usage"
)

// selectionFreeQuotaGates 保存两种普通选择流程和高级选择器各自的免费额度缓存。
type selectionFreeQuotaGates struct {
	Generic    *provider.FreeQuotaGate
	Compatible *provider.FreeQuotaGate
	Advanced   func() *provider.FreeQuotaGate
}

// provideSelectionFreeQuota 绑定免费额度配置和用量来源，缓存及后台任务由额度组件管理。
func provideSelectionFreeQuota(cfg *config.Config, reader usage.UsageLogRepository, tasks *lifecycle.Tasks) *selectionFreeQuotaGates {
	metrics := &provider.FreeQuotaMetrics{}
	options := func() provider.FreeQuotaOptions {
		if cfg == nil {
			return provider.FreeQuotaOptions{}
		}
		v := cfg.Gateway.Grok
		return provider.FreeQuotaOptions{
			Enabled: v.FreeQuotaSoftGateEnabled, TokenLimit: v.FreeQuotaTokenLimit,
			Percent: v.FreeQuotaSoftGatePercent, WindowHours: v.FreeQuotaWindowHours, CacheSeconds: v.FreeQuotaStatsCacheSeconds,
		}
	}
	var load func(context.Context, []int64, time.Time) (map[int64]int64, error)
	if reader != nil {
		load = func(ctx context.Context, ids []int64, start time.Time) (map[int64]int64, error) {
			return usage.ReadProviderTokenWindow(ctx, reader, ids, start)
		}
	}
	factory := func() *provider.FreeQuotaGate {
		return provider.NewFreeQuotaGate(options, load, tasks.Go, time.Now, func(failed bool, message string, fields ...any) {
			if failed {
				slog.Warn(message, fields...)
			} else {
				slog.Info(message, fields...)
			}
		}, metrics)
	}
	// 两种普通选择流程各自使用一份缓存，高级调度器的每个实例也有独立缓存。
	return &selectionFreeQuotaGates{Generic: factory(), Compatible: factory(), Advanced: factory}
}
