//go:build wireinject

package app

import (
	"github.com/google/wire"
)

// auditAssemblyProviders 汇总 audit 模块的 Wire provider。
var auditAssemblyProviders = wire.NewSet(
	provideAuditSettings,
	provideAuditRepository,
	provideAuditService,
	provideAuditHTTP,
	provideAuditMiddleware,
	provideAuditRedactor,
)
