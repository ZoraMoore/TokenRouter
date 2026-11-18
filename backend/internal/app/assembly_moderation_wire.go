//go:build wireinject

package app

import (
	"github.com/google/wire"
)

// moderationAssemblyProviders 汇总 moderation 模块的 Wire provider。
var moderationAssemblyProviders = wire.NewSet(
	provideModerationStore,
	provideModerationHashes,
	provideModerationCore,
	provideModerationHTTP,
	provideModerationSettings,
)
