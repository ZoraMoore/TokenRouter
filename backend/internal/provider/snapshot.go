package provider

import (
	"slices"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
)

// ProviderSnapshot 包含候选判断需要的身份、资格和运行数据。
// 模型改写和凭据读取由各自的调用入口执行。
type ProviderSnapshot struct {
	// ModelPolicy 仅在实际模型匹配点装配，不随协议预检提前读取动态默认值。
	ModelPolicy      ModelRoutingSnapshot `json:"-"`
	ID               int64
	ParentProviderID *int64
	Platform         string
	Type             string
	AuthMode         string
	EnabledProtocols []capability.ProtocolID
	Status           string
	Schedulable      bool
	Concurrency      int
	Priority         int
	ExpiresAt        *time.Time
}

// RoutingSnapshot 返回请求独立副本；协议缺省解析仍由提供商配置规则唯一负责。
func (r *Record) RoutingSnapshot() ProviderSnapshot {
	if r == nil {
		return ProviderSnapshot{}
	}
	snapshot := ProviderSnapshot{
		ID: r.ID, Platform: r.Platform, Type: r.Type, AuthMode: ProtocolAuthMode(r),
		EnabledProtocols: slices.Clone(r.UpstreamProtocols()), Status: r.Status, Schedulable: r.Schedulable,
		Concurrency: r.Concurrency, Priority: r.Priority,
	}
	if r.ParentProviderID != nil {
		id := *r.ParentProviderID
		snapshot.ParentProviderID = &id
	}
	if r.ExpiresAt != nil {
		at := *r.ExpiresAt
		snapshot.ExpiresAt = &at
	}
	return snapshot
}

// Protocols 将已解析的协议能力传给能力目录。
func (s ProviderSnapshot) Protocols() capability.ProviderProtocols {
	return capability.ProviderProtocols{Platform: s.Platform, Type: s.Type, AuthMode: s.AuthMode, Enabled: slices.Clone(s.EnabledProtocols)}
}
