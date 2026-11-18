package routing

import (
	settingvalues "github.com/TokenFlux/TokenRouter/internal/settings"
)

// AdminReadSettings 包含模型回退和市场可用性展示设置。
type AdminReadSettings struct {
	EnableModelFallback                  bool
	FallbackModelAnthropic               string
	FallbackModelAntigravity             string
	FallbackModelGemini                  string
	FallbackModelOpenAI                  string
	MarketplaceAvailabilityBucketMinutes int
	MarketplaceAvailabilityWindowDays    int
}

// ReadAdminSettings 从传入的设置值解析模型回退和市场可用性展示设置。
func ReadAdminSettings(settings map[string]string) *AdminReadSettings {
	result := &AdminReadSettings{}

	result.MarketplaceAvailabilityWindowDays, result.MarketplaceAvailabilityBucketMinutes = ParseMarketplaceAvailabilityWindowSettings(settings)
	result.EnableModelFallback = settings[SettingKeyEnableModelFallback] == "true"
	result.FallbackModelAnthropic = settingvalues.StringOrDefault(settings, SettingKeyFallbackModelAnthropic, "claude-3-5-sonnet-20241022")
	result.FallbackModelOpenAI = settingvalues.StringOrDefault(settings, SettingKeyFallbackModelOpenAI, "gpt-4o")
	result.FallbackModelGemini = settingvalues.StringOrDefault(settings, SettingKeyFallbackModelGemini, "gemini-2.5-pro")
	result.FallbackModelAntigravity = settingvalues.StringOrDefault(settings, SettingKeyFallbackModelAntigravity, "gemini-2.5-pro")
	return result
}
