package httpapi

import (
	"net/http"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/apikey"
)

const (
	authenticatedIngressEdgeTokenHeader          = "X-TokenRouter-Ingress-Edge-Token"
	authenticatedIngressConnectionIDHeader       = "X-TokenRouter-Ingress-Connection-Id"
	authenticatedIngressConnectionRequestsHeader = "X-TokenRouter-Ingress-Connection-Requests"
	authenticatedIngressRemoteAddressHeader      = "X-TokenRouter-Ingress-Remote-Addr"
	authenticatedIngressResolvedAddressHeader    = "X-TokenRouter-Ingress-Resolved-Client-Addr"
	authenticatedIngressRemotePortHeader         = "X-TokenRouter-Ingress-Remote-Port"
	authenticatedIngressServerAddressHeader      = "X-TokenRouter-Ingress-Server-Addr"
	authenticatedIngressServerPortHeader         = "X-TokenRouter-Ingress-Server-Port"
	authenticatedIngressSessionReusedHeader      = "X-TokenRouter-Ingress-Session-Reused"
	authenticatedIngressALPNHeader               = "X-TokenRouter-Ingress-Alpn"
	authenticatedIngressHTTPProtocolHeader       = "X-TokenRouter-Ingress-Http-Protocol"
)

func takeAuthenticatedIngressRequestSnapshot(request *http.Request) apikey.AuthenticatedIngressRequestSnapshot {
	if request == nil {
		return apikey.AuthenticatedIngressRequestSnapshot{}
	}
	header := request.Header
	result := apikey.AuthenticatedIngressRequestSnapshot{
		UserAgent:          strings.TrimSpace(header.Get("User-Agent")),
		Originator:         strings.TrimSpace(header.Get("originator")),
		EdgeToken:          strings.TrimSpace(header.Get(authenticatedIngressEdgeTokenHeader)),
		ConnectionID:       strings.TrimSpace(header.Get(authenticatedIngressConnectionIDHeader)),
		ConnectionRequests: strings.TrimSpace(header.Get(authenticatedIngressConnectionRequestsHeader)),
		RemoteAddress:      strings.TrimSpace(header.Get(authenticatedIngressRemoteAddressHeader)),
		ResolvedAddress:    strings.TrimSpace(header.Get(authenticatedIngressResolvedAddressHeader)),
		RemotePort:         strings.TrimSpace(header.Get(authenticatedIngressRemotePortHeader)),
		ServerAddress:      strings.TrimSpace(header.Get(authenticatedIngressServerAddressHeader)),
		ServerPort:         strings.TrimSpace(header.Get(authenticatedIngressServerPortHeader)),
		SessionReused:      strings.TrimSpace(header.Get(authenticatedIngressSessionReusedHeader)),
		ALPN:               strings.TrimSpace(header.Get(authenticatedIngressALPNHeader)),
		HTTPProtocol:       strings.TrimSpace(header.Get(authenticatedIngressHTTPProtocolHeader)),
	}
	for _, name := range []string{
		authenticatedIngressEdgeTokenHeader,
		authenticatedIngressConnectionIDHeader,
		authenticatedIngressConnectionRequestsHeader,
		authenticatedIngressRemoteAddressHeader,
		authenticatedIngressResolvedAddressHeader,
		authenticatedIngressRemotePortHeader,
		authenticatedIngressServerAddressHeader,
		authenticatedIngressServerPortHeader,
		authenticatedIngressSessionReusedHeader,
		authenticatedIngressALPNHeader,
		authenticatedIngressHTTPProtocolHeader,
	} {
		header.Del(name)
	}
	return result
}

// TakeAuthenticatedIngressRequestSnapshot 提取连接字段并清除内部 Header。
func TakeAuthenticatedIngressRequestSnapshot(r *http.Request) apikey.AuthenticatedIngressRequestSnapshot {
	return takeAuthenticatedIngressRequestSnapshot(r)
}
