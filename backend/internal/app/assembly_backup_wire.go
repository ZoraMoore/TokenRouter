//go:build wireinject

package app

import (
	"github.com/google/wire"
)

// backupAssemblyProviders 汇总 backup 模块的 Wire provider。
var backupAssemblyProviders = wire.NewSet(
	provideBackup,
	provideBackupHTTP,
	provideDataManagementHTTP,
)
