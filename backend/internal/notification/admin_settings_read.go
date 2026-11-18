package notification

import (
	"strconv"
)

// AdminReadSettings 包含 SMTP 参数及密码配置状态。
type AdminReadSettings struct {
	SMTPFrom               string
	SMTPFromName           string
	SMTPHost               string
	SMTPPassword           string
	SMTPPasswordConfigured bool
	SMTPPort               int
	SMTPUseTLS             bool
	SMTPUsername           string
}

// ReadAdminSettings 从传入的设置值解析 SMTP 参数及密码配置状态。
func ReadAdminSettings(settings map[string]string) *AdminReadSettings {
	result := &AdminReadSettings{}
	result.SMTPHost = settings[SettingKeySMTPHost]
	result.SMTPUsername = settings[SettingKeySMTPUsername]
	result.SMTPFrom = settings[SettingKeySMTPFrom]
	result.SMTPFromName = settings[SettingKeySMTPFromName]
	result.SMTPUseTLS = settings[SettingKeySMTPUseTLS] == "true"
	result.SMTPPasswordConfigured = settings[SettingKeySMTPPassword] != ""
	if port, err := strconv.Atoi(settings[SettingKeySMTPPort]); err == nil {
		result.SMTPPort = port
	} else {
		result.SMTPPort = 587
	}
	result.SMTPPassword = settings[SettingKeySMTPPassword]
	return result
}
