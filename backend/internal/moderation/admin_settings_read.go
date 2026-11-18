package moderation

import (
	"strconv"
	"strings"
)

// AdminReadSettings 包含风险控制和 Cyber 会话封禁设置。
type AdminReadSettings struct {
	CyberSessionBlockEnabled    bool
	CyberSessionBlockTTLSeconds int
	RiskControlEnabled          bool
}

// ReadAdminSettings 从传入的设置值解析风险控制和 Cyber 会话封禁设置。
func ReadAdminSettings(settings map[string]string) *AdminReadSettings {
	result := &AdminReadSettings{}
	result.RiskControlEnabled = settings[SettingKeyRiskControlEnabled] == "true"
	result.CyberSessionBlockEnabled = settings[SettingKeyCyberSessionBlockEnabled] == "true"
	if seconds, err := strconv.Atoi(strings.TrimSpace(settings[SettingKeyCyberSessionBlockTTLSeconds])); err == nil && seconds > 0 {
		result.CyberSessionBlockTTLSeconds = seconds
	} else {
		result.CyberSessionBlockTTLSeconds = 3600
	}
	return result
}
