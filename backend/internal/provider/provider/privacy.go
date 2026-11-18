package provider

import (
	"log/slog"

	"github.com/TokenFlux/TokenRouter/internal/infra/telemetry/logging"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/upstream/openai"
)

// PrivacyOptions 组合供应商隐私请求，生命周期和条件写入由提供商用例拥有。
func PrivacyOptions(factory openai.PrivacyClientFactory, endpoints openai.PrivacyEndpoints) provider.PrivacyOptions {
	antigravity := provider.AntigravityAuthorization{Options: AntigravityAuthorizationOptions(nil)}
	options := provider.PrivacyOptions{
		Antigravity: antigravity.SetPrivacy,
		Warn:        slog.Warn,
		Info:        slog.Info,
		AdminObserve: func(format string, args ...any) {
			logging.LegacyPrintf("service.admin", format, args...)
		},
	}
	if factory != nil {
		authorization := OpenAIAuthorizationOptions(&OpenAIAuthorizationDependencies{
			PrivacyFactory: factory, PrivacyEndpoints: endpoints,
		})
		options.OpenAI = authorization.DisableTraining
	}
	return options
}
