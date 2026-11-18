package transfer

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestProviderArchiveVersionBoundary 检查导入使用支持的版本和提供商集合，出现旧账号集合时拒绝。
func TestProviderArchiveVersionBoundary(t *testing.T) {
	var payload DataPayload
	require.NoError(t, json.Unmarshal([]byte(`{"type":"tokenrouter-data","version":2,"proxies":[],"providers":[]}`), &payload))
	require.NoError(t, ValidateHeader(payload))
	legacy := payload
	legacy.Type = LegacyDataType
	require.NoError(t, ValidateHeader(legacy))
	for _, version := range []int{0, 1, 3} {
		candidate := payload
		candidate.Version = version
		require.Error(t, ValidateHeader(candidate))
	}
	for _, format := range []string{"", "tokenrouter-bundle"} {
		candidate := payload
		candidate.Type = format
		require.Error(t, ValidateHeader(candidate))
	}
	require.Error(t, json.Unmarshal([]byte(`{"type":"tokenrouter-data","version":2,"proxies":[],"providers":[],"accounts":[]}`), &payload))
}

// TestLegacySub2APIArchive 检查旧账号导出转换后保留凭据及调度设置。
func TestLegacySub2APIArchive(t *testing.T) {
	for _, header := range []string{"", `"type":"sub2api-data","version":1,`} {
		var payload DataPayload
		body := `{` + header + `"exported_at":"2026-10-11T07:26:23Z","proxies":[],"accounts":[{"name":"fixture","platform":"openai","type":"oauth","credentials":{"access_token":"fixture-access","refresh_token":"fixture-refresh","expires_at":1791700000},"extra":{"token_source":"fixture"},"concurrency":10,"priority":1,"rate_multiplier":1,"auto_pause_on_expired":true,"plan_type":"plus"}]}`
		require.NoError(t, json.Unmarshal([]byte(body), &payload))
		require.NoError(t, ValidateHeader(payload))
		require.Equal(t, DataType, payload.Type)
		require.Equal(t, DataVersion, payload.Version)
		require.Len(t, payload.Providers, 1)
		item := payload.Providers[0]
		require.Equal(t, "fixture-access", item.Credentials["access_token"])
		require.Equal(t, "fixture-refresh", item.Credentials["refresh_token"])
		require.Equal(t, "plus", item.Credentials["plan_type"])
		require.Equal(t, 10, *item.Concurrency)
		require.Equal(t, 1, *item.Priority)
		require.True(t, *item.AutoPauseOnExpired)
	}
}

// TestLegacySub2APIArchiveRejectsAmbiguousFormats 拒绝混合集合和未知版本。
func TestLegacySub2APIArchiveRejectsAmbiguousFormats(t *testing.T) {
	for _, body := range []string{
		`{"accounts":[],"providers":[],"proxies":[]}`,
		`{"type":"sub2api-data","version":2,"accounts":[],"proxies":[]}`,
		`{"type":"other","accounts":[],"proxies":[]}`,
		`{"accounts":null,"proxies":[]}`,
		`{"accounts":[],"proxies":[],"unexpected":true}`,
	} {
		var payload DataPayload
		require.Error(t, json.Unmarshal([]byte(body), &payload))
	}
}

// TestLegacySub2APIArchiveOptionalProxies 检查无代理文件及已存在的套餐字段。
func TestLegacySub2APIArchiveOptionalProxies(t *testing.T) {
	var payload DataPayload
	require.NoError(t, json.Unmarshal([]byte(`{"accounts":[{"name":"fixture","platform":"openai","type":"oauth","credentials":{"access_token":"fixture","plan_type":"pro"},"plan_type":"plus"}]}`), &payload))
	require.NoError(t, ValidateHeader(payload))
	require.Empty(t, payload.Proxies)
	require.Equal(t, "pro", payload.Providers[0].Credentials["plan_type"])
}
