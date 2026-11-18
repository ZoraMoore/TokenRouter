package ops

import (
	"strconv"
	"strings"

	settingvalues "github.com/TokenFlux/TokenRouter/internal/settings"
)

// AdminReadSettings 包含运维监控及额度自动暂停设置。
type AdminReadSettings struct {
	OpenAIQuotaAutoPauseSettings OpsOpenAIProviderQuotaAutoPauseSettings
	OpsMetricsIntervalSeconds    int
	OpsMonitoringEnabled         bool
	OpsRealtimeMonitoringEnabled bool
}

// ReadAdminSettings 从传入的设置值解析运维监控及额度自动暂停设置。
func ReadAdminSettings(settings map[string]string) *AdminReadSettings {
	result := &AdminReadSettings{}

	result.OpsMonitoringEnabled = !settingvalues.IsExplicitFalse(settings[SettingKeyOpsMonitoringEnabled])
	result.OpsRealtimeMonitoringEnabled = !settingvalues.IsExplicitFalse(settings[SettingKeyOpsRealtimeMonitoringEnabled])
	result.OpsMetricsIntervalSeconds = 60
	if raw := strings.TrimSpace(settings[SettingKeyOpsMetricsIntervalSeconds]); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			if v < 60 {
				v = 60
			}
			if v > 3600 {
				v = 3600
			}
			result.OpsMetricsIntervalSeconds = v
		}
	}
	result.OpenAIQuotaAutoPauseSettings = ParseQuotaAutoPauseSettings(settings[SettingKeyOpsAdvancedSettings])
	return result
}
