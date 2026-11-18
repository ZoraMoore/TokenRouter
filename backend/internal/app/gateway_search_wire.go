//go:build wireinject

package app

import "github.com/google/wire"

// gatewaySearchProviders 汇总搜索工具和 HTTP 入口的构造函数。
var gatewaySearchProviders = wire.NewSet(ProvideGatewaySearchTools, ProvideGatewaySearchHTTP, provideGrokSearchExecutor)
