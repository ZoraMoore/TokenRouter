package search

import (
	"encoding/json"
)

// AdminReadSettings 包含搜索模拟的启用状态。
type AdminReadSettings struct{ WebSearchEmulationEnabled bool }

// ReadAdminSettings 从传入的设置值解析搜索模拟的启用状态。
func ReadAdminSettings(settings map[string]string) *AdminReadSettings {
	result := &AdminReadSettings{}

	if raw := settings[SettingKeyWebSearchEmulationConfig]; raw != "" {
		var wsCfg WebSearchEmulationConfig
		if err := json.Unmarshal([]byte(raw), &wsCfg); err == nil {
			result.WebSearchEmulationEnabled = wsCfg.Enabled && len(wsCfg.Providers) > 0
		}
	}
	return result
}
