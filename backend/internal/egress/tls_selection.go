package egress

import (
	"strconv"
)

// TLSSelection 包含提供商 TLS 开关、绑定配置和本次 Router 匹配结果。
type TLSSelection struct {
	Enabled                   bool
	DirectProfileID           int64
	RouterMatched             bool
	RouterID, RouterProfileID int64
}

// ResolveRequestPolicy 保留 Router -> 提供商绑定 -> 内置默认的选择顺序。
func (s *TLSFingerprintProfileService) ResolveRequestPolicy(input TLSSelection) EgressPolicy {
	if input.RouterMatched {
		if p, ok := s.ResolveRoutableTLSProfileByID(input.Enabled, input.RouterProfileID); ok {
			return RequestPolicy(RequestPolicyInput{TLSProfile: p})
		}
	}
	return RequestPolicy(RequestPolicyInput{TLSProfile: s.ResolveTLSProfileByID(input.Enabled, input.DirectProfileID)})
}

// WebSocketTLSIdentity 为连接池生成稳定配置键，随机模板共用同一个键。
// Router 命中且回退配置可用时，配置键仍由 Router ID 和配置 ID 组成。
func WebSocketTLSIdentity(input TLSSelection, hasProfile bool, profileCacheKey string) string {
	if !hasProfile {
		return ""
	}
	if input.RouterMatched {
		if input.RouterProfileID == -1 {
			return "tls-router-random"
		}
		return "tls-router-" + strconv.FormatInt(input.RouterID, 10) + "-" + strconv.FormatInt(input.RouterProfileID, 10)
	}
	if input.DirectProfileID == -1 {
		return "tls-random"
	}
	return profileCacheKey
}
