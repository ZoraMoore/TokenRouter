//go:build wireinject

package app

import (
	"github.com/google/wire"
)

// searchAssemblyProviders 汇总 search 模块的 Wire provider。
var searchAssemblyProviders = wire.NewSet(
	provideSearchRegistry,
	provideSearchRuntime,
	provideSearchHTTP,
)
