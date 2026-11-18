//go:build wireinject

package app

import (
	"github.com/google/wire"
)

// tasksAssemblyProviders 汇总 tasks 模块的 Wire provider。
var tasksAssemblyProviders = wire.NewSet(
	provideTaskActivity,
	provideCreativePublic,
	provideCreativeWorkerRuntime,
	provideBatchPricing,
	provideBatchPublic,
	provideBatchDownload,
	provideBatchCleanup,
	provideBatchCleanupRuntime,
	provideBatchRuntime,
	provideBatchRegistry,
	provideCreativeHTTP,
	provideBatchHTTP,
	provideCreativeRuntimeSettings,
	provideCreativeSettingsHTTP,
)
