package postgres

import (
	"context"
	"time"

	dbprovider "github.com/TokenFlux/TokenRouter/ent/provider"
	providercore "github.com/TokenFlux/TokenRouter/internal/provider"
)

// ListSchedulableProviderLoads 只加载 Ops 队列深度采样所需的提供商 ID 与并发字段。
func (r *ProviderStore) ListSchedulableProviderLoads(ctx context.Context) ([]providercore.LoadObservation, error) {
	providers, err := r.SchedulableProvidersQuery(time.Now()).
		Select(
			dbprovider.FieldID,
			dbprovider.FieldConcurrency,
			dbprovider.FieldLoadFactor,
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	loads := make([]providercore.LoadObservation, 0, len(providers))
	for _, provider := range providers {
		projection := providercore.Record{
			ID:          provider.ID,
			Concurrency: provider.Concurrency,
			LoadFactor:  provider.LoadFactor,
		}
		loads = append(loads, providercore.LoadObservation{
			ID:             provider.ID,
			MaxConcurrency: projection.EffectiveLoadFactor(),
		})
	}
	return loads, nil
}
