package testkit

import (
	"context"

	gatewayadapter "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	"github.com/TokenFlux/TokenRouter/internal/provider"
)

// AvailabilityStore 将同一测试仓储的查询结果转换为诊断数据。
type AvailabilityStore struct {
	Source gatewayadapter.ExecutionProviderStore
}

func (s AvailabilityStore) ListModelAvailabilityCandidates(ctx context.Context, group *int64, platforms []string, grouped bool) ([]provider.Record, error) {
	values, err := s.Source.ListModelAvailabilityCandidates(ctx, group, platforms, grouped)
	if err != nil {
		return nil, err
	}
	if values == nil {
		return nil, nil
	}
	out := make([]provider.Record, len(values))
	for i := range values {
		out[i] = *gatewayadapter.ExecutionRecord(&values[i])
	}
	return out, nil
}
