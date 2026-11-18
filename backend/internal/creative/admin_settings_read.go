package creative

// AdminReadSettings 包含创作台开关、模型和 worker 数量。
type AdminReadSettings struct {
	CreativeEnabled       bool
	CreativeModelSettings []CreativeModelSetting
	CreativeWorkerCount   int
}

// ReadAdminSettings 从传入的设置值解析创作台开关、模型和 worker 数量。
func ReadAdminSettings(settings map[string]string) *AdminReadSettings {
	result := &AdminReadSettings{}
	result.CreativeModelSettings = ParseCreativeModelSettings(settings[SettingKeyCreativeModelSettings])
	result.CreativeWorkerCount = ParseCreativeWorkerCount(settings[SettingKeyCreativeWorkerCount])
	result.CreativeEnabled = settings[SettingKeyCreativeEnabled] != "false"

	return result
}
