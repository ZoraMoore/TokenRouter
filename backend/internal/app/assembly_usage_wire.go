//go:build wireinject

package app

import (
	"github.com/google/wire"
)

// usageAssemblyProviders 汇总 usage 模块的 Wire provider。
var usageAssemblyProviders = wire.NewSet(
	provideUsageDashboardCache,
	providePublicUsage,
	provideUsageKeys,
	provideUsageUsers,
	provideUsageHTTP,
	provideUsageSettings,
	provideAdminUsageHTTP,
	provideDashboardHTTP,
	provideUsageOptions,
	provideUsageStore,
	provideUsageRepository,
	provideUsageService,
	provideUsageAggregationRepository,
	provideUsageCleanupRepository,
	provideUsageAggregation,
	provideUsageCleanup,
	provideUsageDashboard,
)
