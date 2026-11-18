package provider

import (
	"context"
	"fmt"
)

// acquireRefreshLock 保留进程锁、Redis 竞争与故障降级；停止等待由调用者的活动上下文约束。
func (api *OAuthRefreshAPI) acquireRefreshLock(ctx context.Context, providerID int64, cacheKey string) (func(), bool, error) {
	// 0. 获取进程内互斥锁（防止同一进程内的并发刷新竞争）
	distributed := false
	localMu := api.getLocalLock(cacheKey)
	if err := localMu.Lock(ctx); err != nil {
		return nil, false, fmt.Errorf("oauth refresh local lock: %w", err)
	}

	if err := api.checkRefreshActive(ctx); err != nil {
		localMu.Unlock()
		return nil, false, err
	}

	// 1. 获取分布式锁
	if api.tokenCache != nil {
		acquired, lockErr := api.tokenCache.AcquireRefreshLock(ctx, cacheKey, api.lockTTL)
		if lockErr != nil {
			// Redis 错误，降级为无锁刷新（进程内互斥锁仍生效）
			api.options.Warn("oauth_refresh_lock_failed_degraded",
				"provider_id", providerID,
				"cache_key", cacheKey,
				"error", lockErr,
			)
		} else if !acquired {
			// 锁被其他 worker 持有
			localMu.Unlock()
			return nil, true, nil
		} else {
			distributed = true
		}
	}

	return func() {
		if distributed {
			api.releaseRefreshLock(ctx, cacheKey)
		}
		localMu.Unlock()
	}, false, nil
}
