package selection

import (
	"context"

	"github.com/TokenFlux/TokenRouter/internal/apikey"
	"github.com/TokenFlux/TokenRouter/internal/gateway/requeststate"
	"github.com/TokenFlux/TokenRouter/internal/routing"
)

// currentSelectionGroup 只核对入口已授权的当前分组，回退链必须在入口重新准入后生效。
func currentSelectionGroup(ctx context.Context, id *int64, read func(context.Context, int64) (*routing.Group, error)) (*routing.Group, error) {
	if id == nil || *id <= 0 {
		return nil, nil
	}
	if plan, ok := requeststate.RoutePlanFromContext(ctx); ok && plan.GroupID() != *id {
		return nil, apikey.ErrGroupNotAllowed
	}
	group, ok := requeststate.GroupFromContext(ctx)
	if ok && group != nil && group.ID > 0 && group.ID != *id {
		return nil, apikey.ErrGroupNotAllowed
	}
	if ok && routing.IsGroupContextValid(group) {
		return validateCurrentSelectionGroup(ctx, group, *id)
	} else if read != nil {
		var err error
		group, err = read(ctx, *id)
		if err != nil {
			return nil, err
		}
		if group == nil {
			return nil, routing.ErrGroupNotFound
		}
	} else {
		group = nil
	}
	return validateCurrentSelectionGroup(ctx, group, *id)
}

// validateCurrentSelectionGroup 对缓存与回源结果执行相同的组身份和客户端检查。
func validateCurrentSelectionGroup(ctx context.Context, group *routing.Group, id int64) (*routing.Group, error) {
	if group != nil {
		if group.ID != id {
			return nil, apikey.ErrGroupNotAllowed
		}
		if group.ClaudeCodeOnly && !requeststate.IsClaudeCodeClient(ctx) {
			return nil, routing.ErrClaudeCodeOnly
		}
	}
	return group, nil
}
