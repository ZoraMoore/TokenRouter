package routing

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/provider"
)

// pelicanProbeEvidence 把同一会话的两次观测整理为分组用户可读的文本。
func pelicanProbeEvidence(sample provider.StateProbeResult, err error) provider.PelicanEvidence {
	e := provider.PelicanEvidence{
		Prompt: "在同一账号和模型上连续发起两次短请求，第二次使用首次获得的会话状态。",
		Status: "inconclusive",
	}
	observation := "本次观测不足，无法比较两次响应的会话状态。"
	switch {
	case err != nil || sample.Failure != "":
		e.DetailCode = pelicanProbeFailure(sample.Failure, err)
	case sample.Verdict == "healthy":
		e.Status = "passed"
		observation = "会话续接完成，未观察到新的状态。"
	case sample.Verdict == "degraded":
		e.Status, e.DetailCode = "suspected", "state_changed"
		observation = "会话续接完成，第二次响应返回了新的会话状态。"
	default:
		e.DetailCode = "incomplete"
	}
	e.Answer = strings.Join([]string{
		pelicanProbeStep("首次请求", sample.MintStatus, sample.MintLatencyMs, sample.TicketLength, sample.MintCompleted),
		pelicanProbeStep("续接请求", sample.ContinueStatus, sample.ContinueLatencyMs, sample.ContinueTicketLength, sample.ContinueCompleted),
		observation,
	}, "\n")
	return e
}

func pelicanProbeStep(label string, status int, latency int64, length int, completed bool) string {
	if status < 100 || status > 599 {
		return label + "未记录 HTTP 响应。"
	}
	completion := "未收到完成事件"
	if completed {
		completion = "已完成"
	}
	return fmt.Sprintf("%s HTTP %d，%s，%d ms，状态长度 %d 字节。", label, status, completion, max(0, latency), max(0, length))
}

// pelicanProbeFailure 允许已知的分类进入用户详情，提供商错误由管理员在原服务中排查。
func pelicanProbeFailure(code string, err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, provider.ErrIQUnsupported), errors.Is(err, provider.ErrIQInvalid):
		return "model_unsupported"
	case errors.Is(err, provider.ErrIQNotFound), errors.Is(err, provider.ErrIQBusy), errors.Is(err, provider.ErrIQUnavailable):
		return "account_error"
	}
	switch code {
	case "account_error", "rate_limited", "model_unsupported", "upstream_error", "network_error",
		"stream_error", "no_ticket", "timeout", "output_limit", "invalid_state":
		return code
	default:
		return "incomplete"
	}
}
