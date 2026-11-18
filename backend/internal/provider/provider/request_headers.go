package provider

import (
	"net/http"

	"github.com/TokenFlux/TokenRouter/internal/egress"
	egressprovider "github.com/TokenFlux/TokenRouter/internal/egress/provider"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/upstream/anthropic"
)

// ApplyProviderHeaderOverrides 在原调用位置应用提供商策略，保留 wire 大小写和后续会话头覆盖顺序。
func ApplyProviderHeaderOverrides(value *provider.Record, headers http.Header) {
	if headers == nil {
		return
	}
	policy := egress.RequestPolicy(egress.RequestPolicyInput{Headers: value.HeaderOverrides()})
	egressprovider.ApplyRequestHeaders(headers, policy, anthropic.ResolveWireCasing)
}

// HeaderOverrideValue 让协议适配在原 body 净化时点读取一项覆写值。
func HeaderOverrideValue(value *provider.Record, lowerName string) (string, bool) {
	result, ok := value.HeaderOverrides()[lowerName]
	return result, ok
}
