//go:build wireinject

package app

import (
	"github.com/google/wire"
)

// promotionAssemblyProviders 汇总 promotion 模块的 Wire provider。
var promotionAssemblyProviders = wire.NewSet(
	providePromotionPromoHTTP,
	providePromotionAffiliateHTTP,
	providePromotionPromoStore,
	providePromotionPromo,
	providePromotionAffiliateStore,
	providePromotionAffiliate,
	providePromotionSettings,
	providePromotionUserHTTP,
)
