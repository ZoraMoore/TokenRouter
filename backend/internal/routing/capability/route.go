package capability

import "slices"

// ProviderProtocols 包含候选提供商的平台、类型、认证模式和已启用协议。
type ProviderProtocols struct {
	Platform, Type, AuthMode string
	Enabled                  []ProtocolID
}

// ResolveRoute 保留原生优先、批量 provider 绑定及单步转换规则。
func ResolveRoute(provider ProviderProtocols, source ProtocolID, fallbacks map[ProtocolID][]ProtocolID) (ProtocolID, bool) {
	enabled := provider.Enabled
	if slices.Contains(enabled, source) && slices.Contains(NativeProtocolOptions(provider.Platform, provider.Type, provider.AuthMode), source) {
		return source, true
	}
	if source == ProtocolImageBatches {
		// 批量作业沿用 provider 绑定，仅检查该 provider 的专用上游协议。
		target := ProtocolGeminiBatch
		if provider.Type == ProviderTypeServiceAccount {
			target = ProtocolVertexBatch
		}
		return target, provider.Platform == PlatformGemini && slices.Contains(enabled, target)
	}
	targets, configured := fallbacks[source]
	if !configured {
		targets = AutomaticProtocolFallbackTargets(source)
	}
	for _, target := range targets {
		if slices.Contains(enabled, target) && SupportsProtocolConversion(provider.Platform, provider.Type, provider.AuthMode, source, target) {
			return target, true
		}
	}
	return "", false
}
