package apikey

import (
	"github.com/TokenFlux/TokenRouter/internal/pkg/authenticatedingress"
)

type (
	AuthenticatedIngressRequestSnapshot = authenticatedingress.AuthenticatedIngressRequestSnapshot
	AuthenticatedIngressIdentity        = authenticatedingress.AuthenticatedIngressIdentity
	AuthenticatedIngressRequestObserver = authenticatedingress.AuthenticatedIngressRequestObserver
)

func (s *APIKeyService) SetAuthenticatedIngressRequestObserver(observer AuthenticatedIngressRequestObserver) {
	if s == nil {
		return
	}
	s.authenticatedIngressMu.Lock()
	s.authenticatedIngress = observer
	s.authenticatedIngressMu.Unlock()
}

func (s *APIKeyService) ObserveAuthenticatedIngressRequest(requestSnapshot AuthenticatedIngressRequestSnapshot, apiKey *APIKey) {
	if s == nil || apiKey == nil {
		return
	}
	// 无论观察器是否启用，都先删除 Nginx 内部头，禁止它们进入任何上游请求。
	s.authenticatedIngressMu.RLock()
	observer := s.authenticatedIngress
	s.authenticatedIngressMu.RUnlock()
	if observer == nil {
		return
	}

	userID := apiKey.UserID
	if apiKey.User != nil {
		userID = apiKey.User.ID
	}
	observer.ObserveAuthenticatedIngressRequest(requestSnapshot, AuthenticatedIngressIdentity{
		APIKeyID: apiKey.ID,
		UserID:   userID,
	})
}
