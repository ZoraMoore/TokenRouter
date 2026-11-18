package provider

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/upstream/qoder"
)

// QoderHealthStore 提供限流和短暂过载的写入操作。
type QoderHealthStore interface {
	SetRateLimited(context.Context, int64, time.Time) error
	SetOverloaded(context.Context, int64, time.Time) error
}

// ObserveQoderUpstreamError 使用独立的五秒预算尝试保存错误状态。
func ObserveQoderUpstreamError(ctx context.Context, providerID int64, store QoderHealthStore, err error) {
	if store == nil || err == nil {
		return
	}
	var apiErr *qoder.APIError
	if !errors.As(err, &apiErr) {
		return
	}
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	stateCtx, cancel := context.WithTimeout(base, 5*time.Second)
	defer cancel()
	switch {
	case apiErr.IsAgentLimit():
		resetAt, ok := apiErr.AgentLimitResetAt()
		if !ok {
			resetAt = time.Now().Add(30 * time.Second)
		}
		_ = store.SetRateLimited(stateCtx, providerID, resetAt)
	case apiErr.StatusCode == http.StatusTooManyRequests:
		_ = store.SetRateLimited(stateCtx, providerID, time.Now().Add(30*time.Second))
	case apiErr.StatusCode >= 500:
		_ = store.SetOverloaded(stateCtx, providerID, time.Now().Add(30*time.Second))
	}
}
