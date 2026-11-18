//go:build wireinject

package app

import (
	"github.com/google/wire"
)

// schedulerAssemblyProviders 汇总 scheduler 模块的 Wire provider。
var schedulerAssemblyProviders = wire.NewSet(
	schedulerProviders,
	provideProviderDiagnostics,
	provideSchedulerAdminDefaults,
)
