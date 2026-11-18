package app

import (
	"context"

	"github.com/TokenFlux/TokenRouter/internal/app/lifecycle"
	"github.com/TokenFlux/TokenRouter/internal/gateway"
	"github.com/TokenFlux/TokenRouter/internal/infra/telemetry/logging"
	"github.com/TokenFlux/TokenRouter/internal/server/runtimeconfig"
)

// settingsRuntimeReady 标记转发设置加载和默认模型设置迁移已登记到启动流程。
type settingsRuntimeReady struct{}

// provideSettingsRuntime 在目录初始化前加载运行设置；单项失败记录告警并继续启动。
// @project-doc docs/architecture/system_architecture.md#startup_and_shutdown
func provideSettingsRuntime(
	manager *lifecycle.Manager,
	settingService *gateway.RuntimeSettings,
	forwarded *runtimeconfig.ForwardedSettings,
) *settingsRuntimeReady {
	manager.Register(lifecycle.Hook{Name: "RuntimeSettingsInitialization", StartOrder: 181, StopOrder: 800, Start: func(ctx context.Context) error {
		if err := forwarded.LoadForwardedClientIPSettings(ctx); err != nil {
			logging.LegacyPrintf("service.setting", "Warning: load forwarded client IP settings failed: %v", err)
		}
		if err := settingService.MigrateGrokDefaultTextModel(ctx); err != nil {
			logging.LegacyPrintf("service.setting", "Warning: migrate Grok default text model failed: %v", err)
		}
		return nil
	}})
	return &settingsRuntimeReady{}
}
