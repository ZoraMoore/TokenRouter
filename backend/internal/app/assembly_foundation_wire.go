//go:build wireinject

package app

import (
	"github.com/google/wire"
)

// foundationAssemblyProviders 汇总基础设施和存储的 Wire provider。
var foundationAssemblyProviders = wire.NewSet(
	provideCalendar,
	cacheProviders,
	provideEnt,
	provideRedis,
	storageProviders,
	moduleStorageProviders,
)
