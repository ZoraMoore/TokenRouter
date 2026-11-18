package selection

import (
	"time"

	gatewayprovider "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	providercore "github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
)

func isOpenAIProvider(provider *gatewayprovider.ExecutionProvider) bool {
	return provider != nil && (provider.Record.Platform == capability.PlatformOpenAI || provider.Record.Platform == capability.PlatformGrok)
}

func (s *Compatible) isOpenAIProviderRuntimeBlocked(provider *gatewayprovider.ExecutionProvider) bool {
	if s == nil || !isOpenAIProvider(provider) {
		return false
	}
	return s.runtimeBlockState().Blocked(provider.Record.ID, func() string {
		return providercore.RefreshCredentialIdentity(gatewayprovider.ExecutionRecord(provider))
	})
}

func openAIProviderModelTransientModel(canonicalModel string) string {
	return providercore.NormalizeTransientModel(canonicalModel)
}

func (s *Compatible) clearOpenAIProviderModelTransientState(providerID int64, model string) {
	state := s.getOpenAIProviderModelTransientState()
	if state == nil {
		return
	}
	state.RecordSuccess(providerID, model)
}

func (s *Compatible) isOpenAIProviderModelRuntimeBlocked(provider *gatewayprovider.ExecutionProvider, requestedModel string) bool {
	if s == nil || provider == nil {
		return false
	}
	state := s.getOpenAIProviderModelTransientState()
	if state == nil {
		return false
	}
	canonicalModel := gatewayprovider.ExecutionModelPolicy(provider).CanonicalSchedulingModel(requestedModel)
	return state.IsBlocked(provider.Record.ID, openAIProviderModelTransientModel(canonicalModel), time.Now())
}

func (s *Compatible) isOpenAIProviderRequestRuntimeBlocked(provider *gatewayprovider.ExecutionProvider, requestedModel string) bool {
	return s != nil && (s.isOpenAIProviderRuntimeBlocked(provider) || s.isOpenAIProviderModelRuntimeBlocked(provider, requestedModel))
}
