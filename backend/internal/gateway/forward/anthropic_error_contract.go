package forward

import (
	"context"

	"github.com/TokenFlux/TokenRouter/internal/gateway/errorpolicy"
	"github.com/TokenFlux/TokenRouter/internal/upstream"
)

// ErrorInput 保存当前错误响应的数据和处理配置。
type ErrorInput struct {
	ProviderID                                      int64
	ProviderName, ProviderType, Platform, RequestID string
	Status                                          int
	LogBody                                         bool
	LogBodyMaxBytes                                 int
	RequestedModels                                 []string
}

// ErrorPorts 提供健康状态写入、规则查找和 HTTP 输出操作，错误处理流程决定调用顺序。
type ErrorPorts interface {
	ScheduleActivity()
	ReadBody() ([]byte, error)
	ResetBody([]byte)
	Health(context.Context, int, []string) ErrorDecision
	RetryHealth(context.Context, []string) ErrorDecision
	Failover(int, []byte, bool) error
	Commit()
	Message(int, string, string)
	Raw(int, []byte)
	MatchRule(string, int, []byte) *errorpolicy.ErrorPassthroughRule
	SkipMonitoring()
	ScopeDiagnostic(string, int, string)
	SetError(int, string, string)
	Observe(Notice)
	Log(string)
	Truncate(string, int) string
	TruncateBytes([]byte, int) string
	Sanitize(string) string
}

// applyAnthropicDisplayRule 调用规则匹配服务，选择当前协议的响应参数。
// 透传消息继续用原提取器，不扩大原始 body 暴露，也不改变监控/SLA 标记。
func applyAnthropicDisplayRule(p ErrorPorts, platform string, upstreamStatus int, body []byte, defaultStatus int, defaultType, defaultMessage string) (int, string, string, bool) {
	rule := p.MatchRule(platform, upstreamStatus, body)
	if rule == nil {
		return defaultStatus, defaultType, defaultMessage, false
	}
	status := upstreamStatus
	if !rule.PassthroughCode && rule.ResponseCode != nil {
		status = *rule.ResponseCode
	}
	message := upstream.ExtractErrorMessage(body)
	if !rule.PassthroughBody && rule.CustomMessage != nil {
		message = *rule.CustomMessage
	}
	if rule.SkipMonitoring {
		p.SkipMonitoring()
	}
	return status, "upstream_error", message, true
}
