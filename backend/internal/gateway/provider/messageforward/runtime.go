package messageforward

import (
	"context"

	"github.com/TokenFlux/TokenRouter/internal/gateway/forward"
	gatewayadapter "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	"github.com/TokenFlux/TokenRouter/internal/gateway/requeststate"
)

// Execute 为一次 Messages 尝试创建准备状态，提供商切换仍由外层网关决定。
// @project-doc docs/architecture/gateway_request_lifecycle.md#protocol_conversion_boundary
func (r *Runtime) Execute(ctx context.Context, output HTTPBoundary, target *gatewayadapter.ExecutionProvider, parsed *requeststate.ParsedRequest) (*forward.MessagesResult, error) {
	adapter := newAttempt(r, output, target)
	result, err := forward.Messages(ctx, adapter, adapter.input(), parsed)
	return messagesResult(result), err
}

// Count 保留计数入口自己的模型准备和错误响应，不取得生成请求的资源。
func (r *Runtime) Count(ctx context.Context, output HTTPBoundary, target *gatewayadapter.ExecutionProvider, parsed *requeststate.ParsedRequest) error {
	adapter := &countAttempt{attempt: newAttempt(r, output, target)}
	return forward.CountTokens(ctx, adapter, adapter.input(), parsed)
}

// Chat 和 Responses 复用现有协议转换状态机，每次调用独立持有工具恢复状态。
func (r *Runtime) Chat(ctx context.Context, output HTTPBoundary, target *gatewayadapter.ExecutionProvider, body []byte) (*forward.MessagesResult, error) {
	adapter := &conversionAttempt{s: r, c: output, provider: target, state: &AttemptState{}}
	result, err := forward.AsChat(ctx, adapter, forward.ConversionInput{OAuth: target.View().IsOAuth()}, body)
	return messagesResult(result), err
}

func (r *Runtime) Responses(ctx context.Context, output HTTPBoundary, target *gatewayadapter.ExecutionProvider, body []byte) (*forward.MessagesResult, error) {
	adapter := &conversionAttempt{s: r, c: output, provider: target, responses: true, state: &AttemptState{}}
	result, err := forward.AsResponses(ctx, adapter, forward.ConversionInput{OAuth: target.View().IsOAuth()}, body)
	return messagesResult(result), err
}

// messagesResult 同步结果保留浅复制行为；每条转换流的状态不随结果传出。
func messagesResult(result *forward.Result) *forward.MessagesResult {
	if result == nil {
		return nil
	}
	copy := forward.MessagesResult(*result)
	return &copy
}
