//go:build wireinject

package app

import (
	"github.com/google/wire"
)

// runtimeAssemblyProviders 汇总应用生命周期的 Wire provider。
var runtimeAssemblyProviders = wire.NewSet(
	provideModelCatalogRuntime,
	provideSettingsRuntime,
	provideAuthRuntime,
	provideMaintenanceRuntime,
	provideOpsRuntime,
	provideQueuesRuntime,
	provideJobsRuntime,
	provideCoreRuntime,
	provideRuntime,
	provideApplication,
)
