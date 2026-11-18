package provider

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
)

const StateProbeDefaultModel = "gpt-6-astra"

// StateProbeResult 记录同一个 Codex 会话的两次 Responses 请求及续接状态长度。
type StateProbeResult struct {
	AccountID             int64     `json:"account_id"`
	RequestedModel        string    `json:"requested_model"`
	Model                 string    `json:"model"`
	Verdict               string    `json:"verdict"`
	Reason                string    `json:"reason"`
	Failure               string    `json:"failure,omitempty"`
	MintStatus            int       `json:"mint_status"`
	ContinueStatus        int       `json:"continue_status"`
	Minted                bool      `json:"minted"`
	NewTicket             bool      `json:"new_ticket"`
	MintCompleted         bool      `json:"mint_completed"`
	ContinueCompleted     bool      `json:"continue_completed"`
	TicketLength          int       `json:"ticket_length"`
	ContinueTicketLength  int       `json:"continue_ticket_length"`
	ReportedModel         string    `json:"reported_model,omitempty"`
	MintReportedModel     string    `json:"mint_reported_model,omitempty"`
	ContinueReportedModel string    `json:"continue_reported_model,omitempty"`
	StartedAt             time.Time `json:"started_at"`
	FinishedAt            time.Time `json:"finished_at"`
	LatencyMs             int64     `json:"latency_ms"`
	MintLatencyMs         int64     `json:"mint_latency_ms"`
	ContinueLatencyMs     int64     `json:"continue_latency_ms"`
}

type stateProbeTarget interface {
	ProbeTurnState(context.Context, string) StateProbeResult
}

// SupportsStateProbe 允许用账号自己的订阅凭据测试已启用的 Responses 协议。
func SupportsStateProbe(record *Record) bool {
	return record != nil && supportsStateProbeSnapshot(record.RoutingSnapshot())
}

func supportsStateProbeSnapshot(info ProviderSnapshot) bool {
	return info.Platform == PlatformOpenAI && info.Type == ProviderTypeOAuth && info.AuthMode == "" && info.ParentProviderID == nil && slices.Contains(info.EnabledProtocols, capability.ProtocolOpenAIResponses)
}

// ValidStateProbeModelName 限制探针接收的模型别名和上游返回的模型标识。
func ValidStateProbeModelName(model string) bool {
	return validOpenAIModelID(model)
}

// SupportsStateProbeModel 判断送往 Responses 的模型名是否适合文字对话。
func SupportsStateProbeModel(model string) bool {
	return ValidStateProbeModelName(model) && iqTextModel(model)
}

// RunStateProbe 对指定账号发起一次续接测试。手动和定时请求共用同一账号的名额。
func (s *TestService) RunStateProbe(ctx context.Context, id int64, model string) (StateProbeResult, error) {
	if err := ctx.Err(); err != nil {
		return StateProbeResult{}, err
	}
	if model == "" {
		model = StateProbeDefaultModel
	}
	if !validIQRequest(id, IQTestRequest{ModelID: model}) {
		return StateProbeResult{}, ErrIQInvalid
	}
	model = strings.TrimSpace(model)
	if !ValidStateProbeModelName(model) {
		return StateProbeResult{}, ErrIQInvalid
	}
	if _, busy := s.stateProbes.LoadOrStore(id, struct{}{}); busy {
		return StateProbeResult{}, ErrIQBusy
	}
	defer s.stateProbes.Delete(id)
	started := s.iqNow()
	target, err := s.iqTarget(ctx, id, IQTestRequest{ModelID: model})
	if err != nil {
		return StateProbeResult{}, err
	}
	info := target.Information().ProviderSnapshot
	if info.ID != id {
		return StateProbeResult{}, ErrIQUnavailable
	}
	adapter, ok := target.(stateProbeTarget)
	if !ok || !supportsStateProbeSnapshot(info) {
		return StateProbeResult{}, ErrIQUnsupported
	}
	mapped := model
	if resolver, ok := target.(interface{ IQMappedModel(string) string }); ok {
		mapped = resolver.IQMappedModel(model)
	}
	if !SupportsStateProbeModel(mapped) {
		return StateProbeResult{}, ErrIQUnsupported
	}
	result := adapter.ProbeTurnState(ctx, mapped)
	if err = ctx.Err(); err != nil {
		return StateProbeResult{}, err
	}
	result.AccountID, result.RequestedModel = id, model
	result.StartedAt, result.FinishedAt = started, s.iqNow()
	result.LatencyMs = result.FinishedAt.Sub(started).Milliseconds()
	result.Verdict = "inconclusive"
	if result.Failure == "" {
		switch {
		case result.MintStatus != 200 || !result.MintCompleted:
			result.Failure = "stream_error"
		case !result.Minted:
			result.Failure = "no_ticket"
		case result.ContinueStatus != 200 || !result.ContinueCompleted:
			result.Failure = "stream_error"
		case result.NewTicket:
			result.Verdict = "degraded"
			result.Reason = "续接完成，上游返回了不同的会话状态。"
		default:
			result.Verdict = "healthy"
			result.Reason = "续接完成，上游沿用首发状态或省略了状态头。"
		}
	}
	if result.Failure != "" {
		result.Reason = stateProbeFailureReason(result)
	}
	return result, nil
}

func stateProbeFailureReason(r StateProbeResult) string {
	step := "首发"
	if r.Minted {
		step = "续接"
	}
	switch r.Failure {
	case "no_ticket":
		return "首发完成，但上游没有返回可用于续接的状态。"
	case "account_error":
		return step + "未能使用该账号的授权。"
	case "model_unsupported":
		return step + "请求的模型不受该账号支持。"
	case "rate_limited":
		return step + "请求被上游限流。"
	case "network_error":
		return step + "未收到上游 HTTP 响应。"
	case "stream_error":
		return step + "未收到完整的 Responses 完成事件。"
	case "timeout":
		return step + "超过本次请求的时限。"
	case "output_limit":
		return step + "的响应超过读取上限。"
	case "invalid_state":
		return step + "返回了无法用于续接的状态头。"
	default:
		return step + "请求未能完成。"
	}
}
