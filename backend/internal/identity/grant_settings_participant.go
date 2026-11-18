package identity

import (
	"context"
	"encoding/json"

	"github.com/TokenFlux/TokenRouter/internal/settings"
)

// AuthSourceParticipantFields 将已合并配置编码为更新字段，金额和历史值使用存储编码规则。
func AuthSourceParticipantFields(value *AuthSourceDefaultSettings) settings.Fields {
	result := settings.Fields{}
	if value == nil {
		return result
	}
	for key, encoded := range encodeAuthSourceSettings(value) {
		// 编码字符串不会把旧数据库的非规范数值变成新的 JSON 数字校验。
		raw, _ := json.Marshal(encoded)
		result[key] = raw
	}
	return result
}

// prepareAuthSourceFields 准备认证来源的待保存配置。
func (g *GrantSettings) prepareAuthSourceFields(ctx context.Context, input settings.Fields) (map[string]string, error) {
	values := map[string]string{}
	for _, key := range AuthSourceSettingKeys() {
		raw, ok := input[key]
		if !ok {
			continue
		}
		var stored string
		if err := json.Unmarshal(raw, &stored); err != nil {
			stored = string(raw)
		}
		values[key] = stored
	}
	if len(values) == 0 {
		return nil, nil
	}
	prepared, err := g.PrepareAuthSourceDefaults(ctx, parseAuthSourceSettings(values))
	if err != nil {
		return nil, err
	}
	for key := range prepared {
		if _, present := values[key]; !present {
			delete(prepared, key)
		}
	}
	return prepared, nil
}
