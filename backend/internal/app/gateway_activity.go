package app

import "github.com/TokenFlux/TokenRouter/internal/app/lifecycle"

// gatewayRequestActivity 让请求准入、平台尝试和完成入队共用一次关闭屏障。
// 供应商请求使用自己的 context，Qoder 尾部用量收集使用独立的时间预算。
type gatewayRequestActivity struct{ *lifecycle.Operations }

func provideGatewayRequestActivity(manager *lifecycle.Manager) *gatewayRequestActivity {
	activity := &gatewayRequestActivity{Operations: lifecycle.NewOperations("GatewayRequestsAndAttempts")}
	manager.Register(lifecycle.Hook{Name: "GatewayRequestsAndAttempts", StopOrder: 15, Stop: activity.StopContext})
	return activity
}
