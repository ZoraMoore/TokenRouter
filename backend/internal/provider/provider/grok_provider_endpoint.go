package provider

import (
	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/upstream/grok"
)

// GrokProviderBaseURL 区分文本订阅代理与 API Key 的默认端点，不执行目标安全放行。
func GrokProviderBaseURL(value *provider.Record) string {
	if value == nil || !value.IsGrok() {
		return ""
	}
	if value.IsGrokOAuth() {
		return GrokProviderBaseURLOr(value, grok.DefaultCLIBaseURL)
	}
	return GrokProviderBaseURLOr(value, grok.DefaultBaseURL)
}

// GrokProviderBaseURLOr 优先使用提供商配置的地址，缺省时使用调用方默认值。
func GrokProviderBaseURLOr(value *provider.Record, fallback string) string {
	if value == nil || !value.IsGrok() {
		return ""
	}
	return grok.ResolveProviderBaseURL(value.IsGrokOAuth(), value.GetCredential("base_url"), fallback)
}

// GrokProviderMediaBaseURL 仅将 OAuth 的官方 CLI 主机映射到媒体端点。
func GrokProviderMediaBaseURL(value *provider.Record) string {
	if !value.IsGrok() {
		return ""
	}
	base := GrokProviderBaseURL(value)
	if value.IsGrokOAuth() && (grok.MediaCodec{}).IsGrokCLIProxyTarget(base) {
		return grok.DefaultBaseURL
	}
	return base
}
