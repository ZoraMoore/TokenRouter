package provider

import "context"

const (
	AntigravityForceTokenRefreshExtraKey       = "antigravity_force_token_refresh"
	AntigravityForceTokenRefreshReasonExtraKey = "antigravity_force_token_refresh_reason"
	AntigravityForceTokenRefreshAtExtraKey     = "antigravity_force_token_refresh_at"
)

// RefreshRequestClearer 按已完成交换的身份清理单次刷新请求标记。
type RefreshRequestClearer interface {
	ClearAntigravityRefreshRequest(context.Context, CredentialVersion) (bool, error)
}

func ClearedAntigravityRefreshRequest() map[string]any {
	return map[string]any{AntigravityForceTokenRefreshExtraKey: false, AntigravityForceTokenRefreshReasonExtraKey: "", AntigravityForceTokenRefreshAtExtraKey: ""}
}

// NeedsAntigravityRefreshRequest 只识别服务端一次性标记，平台报文解析仍由供应商适配提供。
func NeedsAntigravityRefreshRequest(value *Record) bool {
	return value != nil && value.Platform == PlatformAntigravity && value.Type == ProviderTypeOAuth && value.GetExtraBool(AntigravityForceTokenRefreshExtraKey)
}
