package scheduler

import "context"

// SnapshotPublicationCache 在事务提交后同步提供商快照，变更事件由写入方在事务中记录到 outbox。
type SnapshotPublicationCache interface {
	SetProvider(context.Context, SnapshotProvider) error
	DeleteProvider(context.Context, int64) error
}

// SnapshotPublisher 将批量提供商 ID 去重后逐项发布。
type SnapshotPublisher struct {
	Read        func(context.Context, int64) (SnapshotProvider, error)
	ReadMany    func(context.Context, []int64) ([]SnapshotProvider, error)
	Cache       SnapshotPublicationCache
	Diagnostics Diagnostics
}

func (p SnapshotPublisher) Publish(ctx context.Context, providerID int64) {
	if p.Cache == nil || providerID <= 0 {
		return
	}
	provider, err := p.Read(ctx, providerID)
	if err != nil {
		p.Diagnostics.printf("repository.provider", "[Scheduler] sync provider snapshot read failed: id=%d err=%v", providerID, err)
		return
	}
	if err := p.Cache.SetProvider(ctx, provider); err != nil {
		p.Diagnostics.printf("repository.provider", "[Scheduler] sync provider snapshot write failed: id=%d err=%v", providerID, err)
	}
}

func (p SnapshotPublisher) PublishMany(ctx context.Context, providerIDs []int64) {
	if p.Cache == nil || len(providerIDs) == 0 {
		return
	}

	uniqueIDs := make([]int64, 0, len(providerIDs))
	seen := make(map[int64]struct{}, len(providerIDs))
	for _, id := range providerIDs {
		if id <= 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}
	if len(uniqueIDs) == 0 {
		return
	}

	providers, err := p.ReadMany(ctx, uniqueIDs)
	if err != nil {
		p.Diagnostics.printf("repository.provider", "[Scheduler] batch sync provider snapshot read failed: count=%d err=%v", len(uniqueIDs), err)
		return
	}

	for _, provider := range providers {
		if provider == nil {
			continue
		}
		if err := p.Cache.SetProvider(ctx, provider); err != nil {
			p.Diagnostics.printf("repository.provider", "[Scheduler] batch sync provider snapshot write failed: id=%d err=%v", provider.SnapshotMetadata().ID, err)
		}
	}
}

// Drop 在提供商删除后主动清理调度器缓存中的单提供商快照。
func (p SnapshotPublisher) Drop(ctx context.Context, providerID int64) {
	if p.Cache == nil || providerID <= 0 {
		return
	}
	if err := p.Cache.DeleteProvider(ctx, providerID); err != nil {
		p.Diagnostics.printf("repository.provider", "[Scheduler] delete provider snapshot failed: id=%d err=%v", providerID, err)
	}
}
