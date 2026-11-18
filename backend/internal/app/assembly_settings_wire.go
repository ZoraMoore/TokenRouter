//go:build wireinject

package app

import (
	"github.com/google/wire"
)

// settingsAssemblyProviders 汇总 settings 模块的 Wire provider。
var settingsAssemblyProviders = wire.NewSet(
	provideCompositeReadOptions,
	providePreAggregationSettings,
	provideCompositeSettingsHTTP,
	provideCompositeRuntime,
	provideSettingsParticipants,
	providePreAggregationHTTP,
)
