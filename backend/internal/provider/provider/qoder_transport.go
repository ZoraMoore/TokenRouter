package provider

import (
	"net/http"

	"github.com/TokenFlux/TokenRouter/internal/egress"
	egressprovider "github.com/TokenFlux/TokenRouter/internal/egress/provider"
	"github.com/TokenFlux/TokenRouter/internal/infra/httpclient/tlsfingerprint"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/upstream/qoder"
)

// QoderTransport 接收单次上游请求，provider 管理缓存和授权状态。
type QoderTransport interface {
	DoWithTLS(*http.Request, string, int64, int, *tlsfingerprint.Profile) (*http.Response, error)
}

// QoderRequestDoer 为请求绑定代理和 TLS 选择函数，共用客户端池。
func QoderRequestDoer(value *provider.Record, transport QoderTransport, profiles *egressprovider.TLSProfiles) qoder.RequestDoer {
	if transport == nil || value == nil {
		return nil
	}
	proxyURL := ""
	if value.ProxyID != nil && value.Proxy != nil {
		proxyURL = value.Proxy.URL()
	}
	var profile *tlsfingerprint.Profile
	if profiles != nil {
		profile = profiles.ResolveRequestTLS(egress.TLSSelection{
			Enabled:         value.IsTLSFingerprintEnabled(),
			DirectProfileID: value.GetTLSFingerprintProfileID(),
		})
	}
	return func(req *http.Request) (*http.Response, error) {
		return transport.DoWithTLS(req, proxyURL, value.ID, value.Concurrency, profile)
	}
}
