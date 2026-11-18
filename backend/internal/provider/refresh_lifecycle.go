package provider

import (
	"context"
	"errors"
)

var ErrRefreshStopped = errors.New("oauth refresh coordinator is stopped")

type refreshActivity = operationActivity

// beginRefresh 为刷新操作登记活动和取消函数，停止后拒绝新操作。
func (api *OAuthRefreshAPI) beginRefresh(parent context.Context) (context.Context, func(), error) {
	return api.activity.begin(parent, ErrRefreshStopped)
}

// checkRefreshActive 在取得提供商锁后复查停止状态，已停止时拒绝 token 交换。
func (api *OAuthRefreshAPI) checkRefreshActive(ctx context.Context) error {
	api.activity.mu.Lock()
	defer api.activity.mu.Unlock()
	if api.activity.stopped {
		return context.Canceled
	}
	return ctx.Err()
}

// StopContext 复用首次停止结果，超时不能报告已排空。
func (api *OAuthRefreshAPI) StopContext(ctx context.Context) error {
	if api == nil {
		return nil
	}
	return api.activity.stop(ctx, "oauth refresh")
}
