package testkit

import (
	"context"
	"sync/atomic"

	"github.com/TokenFlux/TokenRouter/internal/scheduler"
)

type ConcurrencyHooks struct {
	AcquireUserSlotFn     func(ctx context.Context, userID int64, maxConcurrency int, requestID string) (bool, error)
	AcquireProviderSlotFn func(ctx context.Context, providerID int64, maxConcurrency int, requestID string) (bool, error)
	AcquireIngressLeaseFn func(ctx context.Context, apiKeyID int64, maxConnections int, leaseID string) (bool, error)
	ReleaseIngressLeaseFn func(ctx context.Context, apiKeyID int64, leaseID string) error
	ReleaseUserCalled     int32
	ReleaseProviderCalled int32
	ReleaseIngressCalled  int32
}

func (m *ConcurrencyHooks) AcquireProviderSlot(ctx context.Context, providerID int64, maxConcurrency int, requestID string) (bool, error) {
	if m.AcquireProviderSlotFn != nil {
		return m.AcquireProviderSlotFn(ctx, providerID, maxConcurrency, requestID)
	}
	return false, nil
}

func (m *ConcurrencyHooks) ReleaseProviderSlot(ctx context.Context, providerID int64, requestID string) error {
	atomic.AddInt32(&m.ReleaseProviderCalled, 1)
	return nil
}

func (m *ConcurrencyHooks) GetProviderConcurrency(ctx context.Context, providerID int64) (int, error) {
	return 0, nil
}

func (m *ConcurrencyHooks) GetProviderConcurrencyBatch(ctx context.Context, providerIDs []int64) (map[int64]int, error) {
	result := make(map[int64]int, len(providerIDs))
	for _, providerID := range providerIDs {
		result[providerID] = 0
	}
	return result, nil
}

func (m *ConcurrencyHooks) IncrementProviderWaitCount(ctx context.Context, providerID int64, maxWait int) (bool, error) {
	return true, nil
}

func (m *ConcurrencyHooks) DecrementProviderWaitCount(ctx context.Context, providerID int64) error {
	return nil
}

func (m *ConcurrencyHooks) GetProviderWaitingCount(ctx context.Context, providerID int64) (int, error) {
	return 0, nil
}

func (m *ConcurrencyHooks) AcquireUserSlot(ctx context.Context, userID int64, maxConcurrency int, requestID string) (bool, error) {
	if m.AcquireUserSlotFn != nil {
		return m.AcquireUserSlotFn(ctx, userID, maxConcurrency, requestID)
	}
	return false, nil
}

func (m *ConcurrencyHooks) ReleaseUserSlot(ctx context.Context, userID int64, requestID string) error {
	atomic.AddInt32(&m.ReleaseUserCalled, 1)
	return nil
}

func (m *ConcurrencyHooks) GetUserConcurrency(ctx context.Context, userID int64) (int, error) {
	return 0, nil
}

func (m *ConcurrencyHooks) IncrementWaitCount(ctx context.Context, userID int64, maxWait int) (bool, error) {
	return true, nil
}

func (m *ConcurrencyHooks) DecrementWaitCount(ctx context.Context, userID int64) error {
	return nil
}

func (m *ConcurrencyHooks) GetProvidersLoadBatch(ctx context.Context, providers []scheduler.ProviderWithConcurrency) (map[int64]*scheduler.ProviderLoadInfo, error) {
	return map[int64]*scheduler.ProviderLoadInfo{}, nil
}

func (m *ConcurrencyHooks) GetUsersLoadBatch(ctx context.Context, users []scheduler.UserWithConcurrency) (map[int64]*scheduler.UserLoadInfo, error) {
	return map[int64]*scheduler.UserLoadInfo{}, nil
}

func (m *ConcurrencyHooks) CleanupExpiredProviderSlots(ctx context.Context, providerID int64) error {
	return nil
}

func (m *ConcurrencyHooks) CleanupExpiredProviderSlotKeys(ctx context.Context) error {
	return nil
}

func (m *ConcurrencyHooks) CleanupStaleProcessSlots(ctx context.Context, activeRequestPrefix string) error {
	return nil
}

func (m *ConcurrencyHooks) AcquireOpenAIWSIngressLease(ctx context.Context, apiKeyID int64, maxConnections int, leaseID string) (bool, error) {
	if m.AcquireIngressLeaseFn != nil {
		return m.AcquireIngressLeaseFn(ctx, apiKeyID, maxConnections, leaseID)
	}
	return false, nil
}

func (m *ConcurrencyHooks) RefreshOpenAIWSIngressLease(context.Context, int64, string) (bool, error) {
	return true, nil
}

func (m *ConcurrencyHooks) ReleaseOpenAIWSIngressLease(ctx context.Context, apiKeyID int64, leaseID string) error {
	atomic.AddInt32(&m.ReleaseIngressCalled, 1)
	if m.ReleaseIngressLeaseFn != nil {
		return m.ReleaseIngressLeaseFn(ctx, apiKeyID, leaseID)
	}
	return nil
}
