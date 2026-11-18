package capability

import (
	"fmt"
)

// canonicalGroupClientProtocols 从唯一目录派生顺序，避免新增协议遗漏校验。
var canonicalGroupClientProtocols = func() []ProtocolID {
	out := []ProtocolID{}
	for _, protocol := range protocolCatalog {
		if !protocol.UpstreamOnly {
			out = append(out, protocol.ID)
		}
	}
	return out
}()

// SupportedGroupClientProtocols 返回所有公开入口；上游资格在逐提供商选路时判断。
func SupportedGroupClientProtocols(_ string) []ProtocolID {
	return append([]ProtocolID{}, canonicalGroupClientProtocols...)
}

// DefaultGroupClientProtocols 只默认开放三个文本入口，其余能力由管理员显式启用。
func DefaultGroupClientProtocols(_ string) []ProtocolID {
	return []ProtocolID{ProtocolAnthropicMessages, ProtocolOpenAIResponses, ProtocolOpenAIChatCompletions}
}

// ValidateGroupClientProtocols 校验完整协议集合并返回固定顺序的副本。
func ValidateGroupClientProtocols(platform string, protocols []ProtocolID) ([]ProtocolID, error) {
	supported := make(map[ProtocolID]struct{})
	for _, protocol := range SupportedGroupClientProtocols(platform) {
		supported[protocol] = struct{}{}
	}
	known := make(map[ProtocolID]struct{}, len(canonicalGroupClientProtocols))
	for _, protocol := range canonicalGroupClientProtocols {
		known[protocol] = struct{}{}
	}

	seen := make(map[ProtocolID]struct{}, len(protocols))
	for i, protocol := range protocols {
		if _, ok := known[protocol]; !ok {
			return nil, fmt.Errorf("allowed_protocols[%d] contains unknown protocol %q", i, protocol)
		}
		if _, ok := supported[protocol]; !ok {
			return nil, fmt.Errorf("protocol %q is not supported by platform %q", protocol, platform)
		}
		if _, ok := seen[protocol]; ok {
			return nil, fmt.Errorf("protocol %q is duplicated", protocol)
		}
		seen[protocol] = struct{}{}
	}
	out := make([]ProtocolID, 0, len(seen))
	for _, protocol := range canonicalGroupClientProtocols {
		if _, ok := seen[protocol]; ok {
			out = append(out, protocol)
		}
	}
	return out, nil
}

// SetGroupClientProtocol 更新单个协议，并按协议目录顺序返回集合。
func SetGroupClientProtocol(protocols []ProtocolID, target ProtocolID, enabled bool) []ProtocolID {
	selected := make(map[ProtocolID]struct{}, len(protocols)+1)
	for _, protocol := range protocols {
		selected[protocol] = struct{}{}
	}
	if enabled {
		selected[target] = struct{}{}
	} else {
		delete(selected, target)
	}
	out := make([]ProtocolID, 0, len(selected))
	for _, protocol := range canonicalGroupClientProtocols {
		if _, ok := selected[protocol]; ok {
			out = append(out, protocol)
		}
	}
	return out
}

// DefaultProtocolFallbacks 省略入口表示自动匹配，显式空目标列表表示仅原生。
func DefaultProtocolFallbacks(_ string) map[ProtocolID][]ProtocolID {
	return map[ProtocolID][]ProtocolID{}
}
