//go:build wireinject

package app

import "github.com/google/wire"

// modelCatalogAssemblyProviders 构造唯一统一目录实例，价格与属性消费者共享同一发布版本。
var modelCatalogAssemblyProviders = wire.NewSet(
	provideModelCatalogService,
	provideModelCatalogRemoteClient,
)
