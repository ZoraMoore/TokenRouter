//go:build wireinject

package app

import (
	"github.com/google/wire"
)

// notificationAssemblyProviders 汇总 notification 模块的 Wire provider。
var notificationAssemblyProviders = wire.NewSet(
	provideRiskDelivery,
	provideAlertDelivery,
	provideMailer,
	provideNotification,
	provideEmailQueue,
	provideNotificationHTTP,
)
