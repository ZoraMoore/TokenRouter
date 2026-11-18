package provider

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/upstream/openai"
)

// CanRetryOpenAI429 只决定原同提供商恢复窗口资格；实际重试循环仍由网关拥有。
func CanRetryOpenAI429(state *provider.RuntimeBlockState, value *provider.Record, headers http.Header, body []byte) bool {
	if state == nil || value == nil || !value.IsOpenAIOAuthLike() || value.IsShadow() ||
		state.Blocked(value.ID, func() string { return provider.RefreshCredentialIdentity(value) }) {
		return false
	}
	disposition, _ := ClassifyOpenAI429(headers, body)
	if disposition != provider.OpenAI429Transient {
		return false
	}
	return state.RetryWindowActive(value.ID)
}

// ClassifyOpenAI429 先识别额度耗尽，再依次读取重置响应头和正文。
func ClassifyOpenAI429(headers http.Header, body []byte) (provider.OpenAI429Disposition, *time.Time) {
	if kind, reset := provider.OpenAIExhaustedWindow(openai.ParseCodexRateLimitHeaders(headers), time.Now); kind != provider.OpenAI429Transient {
		return kind, reset
	}
	if reset := provider.OpenAI429ResetTime(openai.ParseCodexRateLimitHeaders(headers), time.Now, slog.Info); reset != nil {
		return provider.OpenAI429QuotaReset, reset
	}
	if unix := openai.ParseUsageLimitResetTime(body, time.Now); unix != nil {
		reset := time.Unix(*unix, 0)
		return provider.OpenAI429QuotaReset, &reset
	}
	return provider.OpenAI429Transient, nil
}
