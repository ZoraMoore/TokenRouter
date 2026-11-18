package requeststate

import (
	"context"

	"github.com/TokenFlux/TokenRouter/internal/protocol"
	"github.com/TokenFlux/TokenRouter/internal/routing"
)

// RoutingState 复制入口或当前尝试的分组、协议和模型计划。
type RoutingState struct {
	group          *routing.Group
	plan           routing.RoutePlan
	planSet        bool
	clientProtocol protocol.ProtocolID
}

type routingStateKey struct{}

func RoutingStateFromContext(ctx context.Context) RoutingState {
	if ctx == nil {
		return RoutingState{}
	}
	state, _ := ctx.Value(routingStateKey{}).(RoutingState)
	return state
}

// WithRoutingState 保存已构造的路由状态，身份和资金资格由准入步骤检查。
func WithRoutingState(ctx context.Context, state RoutingState) context.Context {
	return context.WithValue(ctx, routingStateKey{}, state)
}

func WithGroup(ctx context.Context, group *routing.Group) context.Context {
	state := RoutingStateFromContext(ctx)
	state.group = routing.CloneGroup(group)
	return WithRoutingState(ctx, state)
}

func GroupFromContext(ctx context.Context) (*routing.Group, bool) {
	state := RoutingStateFromContext(ctx)
	return routing.CloneGroup(state.group), state.group != nil
}

func WithRoutePlan(ctx context.Context, plan routing.RoutePlan) context.Context {
	state := RoutingStateFromContext(ctx)
	state.plan, state.planSet = plan, true
	return WithRoutingState(ctx, state)
}

func RoutePlanFromContext(ctx context.Context) (routing.RoutePlan, bool) {
	state := RoutingStateFromContext(ctx)
	return state.plan, state.planSet
}

func WithClientProtocol(ctx context.Context, source protocol.ProtocolID) context.Context {
	state := RoutingStateFromContext(ctx)
	state.clientProtocol = source
	return WithRoutingState(ctx, state)
}

func ClientProtocolFromContext(ctx context.Context) (protocol.ProtocolID, bool) {
	state := RoutingStateFromContext(ctx)
	return state.clientProtocol, state.clientProtocol != ""
}
