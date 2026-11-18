package provider

import (
	"context"
	"errors"

	"github.com/TokenFlux/TokenRouter/internal/provider"
	provideradapter "github.com/TokenFlux/TokenRouter/internal/provider/provider"
)

// GrokMediaEligibilityProber 在缺少计费资格观测时探测媒体资格。
type GrokMediaEligibilityProber interface {
	ProbeMediaEligibility(context.Context, int64) (bool, string, error)
}

// CheckGrokMediaEligibility 根据提供商配置检查媒体资格，缺少观测时调用探测器。
func CheckGrokMediaEligibility(ctx context.Context, value *ExecutionProvider, probe GrokMediaEligibilityProber) (bool, string, error) {
	if value == nil {
		return false, "missing_provider", errors.New("grok media provider is required")
	}
	eligible, reason := provider.GrokMediaGenerationEligibility(ExecutionRecord(value), provideradapter.GrokTierRules())
	if eligible || reason != "billing_unobserved" {
		return eligible, reason, nil
	}
	if probe == nil {
		return false, "billing_probe_unavailable", errors.New("grok media eligibility probe is not configured")
	}
	return probe.ProbeMediaEligibility(ctx, value.Record.ID)
}
