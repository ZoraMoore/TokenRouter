package selection

import (
	"strings"
	"time"

	gatewayprovider "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	providercore "github.com/TokenFlux/TokenRouter/internal/provider"
)

func filterGrokModelQuotaBlockedProviders(providers []gatewayprovider.ExecutionProvider, model string, now time.Time) []gatewayprovider.ExecutionProvider {
	if len(providers) == 0 || strings.TrimSpace(model) == "" {
		return providers
	}
	out := make([]gatewayprovider.ExecutionProvider, 0, len(providers))
	for i := range providers {
		upstreamModel := gatewayprovider.ExecutionModelPolicy(&providers[i]).CanonicalSchedulingModel(model)
		if providercore.IsGrokModelQuotaBlocked(providers[i].Record.ID, upstreamModel, now) {
			continue
		}
		out = append(out, providers[i])
	}
	return out
}
