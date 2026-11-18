package forward

import (
	"context"
	"fmt"
)

// InvalidJSONInput 记录上游 2xx 响应和 JSON 解析失败，健康策略使用 502 状态。
type InvalidJSONInput struct {
	ProviderID              int64
	ProviderName, RequestID string
	UpstreamStatus          int
	Headers                 map[string][]string
	Body                    []byte
	ParseError              error
	RequestedModels         []string
}
type InvalidJSONPorts interface {
	Log(string)
	Health(context.Context, int, []byte, []string) ErrorDecision
	Failover(int, map[string][]string, []byte, bool) error
}

// InvalidJSON 解释原生解码失败的恢复结果，不创建额外请求或改变提供商策略。
func InvalidJSON(ctx context.Context, p InvalidJSONPorts, in InvalidJSONInput) error {
	const status = 502
	p.Log(fmt.Sprintf("Provider %d(%s): upstream returned non-JSON 2xx response, attempting failover: status=%d request_id=%s error=%v", in.ProviderID, in.ProviderName, in.UpstreamStatus, in.RequestID, in.ParseError))
	decision := p.Health(ctx, status, in.Body, in.RequestedModels)
	if decision.Generic {
		return fmt.Errorf("upstream returned invalid JSON (not in custom error codes): %w", in.ParseError)
	}
	return p.Failover(status, in.Headers, in.Body, decision.RetrySameProvider)
}
