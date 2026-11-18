package routing

import (
	"context"

	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
)

// RuntimeSettingsStore 按键读取路由设置。
type RuntimeSettingsStore interface {
	GetValue(context.Context, string) (string, error)
}
type RuntimeSettings struct{ settingRepo RuntimeSettingsStore }

// NewRuntimeSettings 构造不回源，参数读取由现有请求编排触发。
func NewRuntimeSettings(repo RuntimeSettingsStore) *RuntimeSettings {
	return &RuntimeSettings{settingRepo: repo}
}

// IsModelFallbackEnabled 保持原缺省、未知平台及故障语义。
func (s *RuntimeSettings) IsModelFallbackEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyEnableModelFallback)
	if err != nil {
		return false // Default: disabled
	}
	return value == "true"
}

// GetFallbackModel 保持原缺省、未知平台及故障语义。
func (s *RuntimeSettings) GetFallbackModel(ctx context.Context, platform string) string {
	var key string
	var defaultModel string

	switch platform {
	case capability.PlatformAnthropic:
		key = SettingKeyFallbackModelAnthropic
		defaultModel = "claude-3-5-sonnet-20241022"
	case capability.PlatformOpenAI:
		key = SettingKeyFallbackModelOpenAI
		defaultModel = "gpt-4o"
	case capability.PlatformGemini:
		key = SettingKeyFallbackModelGemini
		defaultModel = "gemini-2.5-pro"
	case capability.PlatformAntigravity:
		key = SettingKeyFallbackModelAntigravity
		defaultModel = "gemini-2.5-pro"
	default:
		return ""
	}

	value, err := s.settingRepo.GetValue(ctx, key)
	if err != nil || value == "" {
		return defaultModel
	}
	return value
}
