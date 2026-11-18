package provider

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/upstream/openai"
)

func openAIImageResetAt(headers http.Header, body []byte) time.Time {
	now := time.Now()
	if resetAt := openai.ParseRetryAfterResetTime(headers, now); resetAt != nil && resetAt.After(now) {
		return *resetAt
	}
	if resetAt := provider.OpenAI429ResetTime(openai.ParseCodexRateLimitHeaders(headers), time.Now, slog.Info); resetAt != nil && resetAt.After(now) {
		return *resetAt
	}
	if resetUnix := openai.ParseUsageLimitResetTime(body, time.Now); resetUnix != nil {
		if resetAt := time.Unix(*resetUnix, 0); resetAt.After(now) {
			return resetAt
		}
	}
	if cooldown := openai.ParseImageTryAgainCooldown(body); cooldown > 0 {
		return now.Add(cooldown)
	}
	return now.Add(time.Minute)
}

// ObserveOpenAIImageRateLimit 在提供商规则允许后才解析重置观测，保留日志和取时顺序。
func ObserveOpenAIImageRateLimit(ctx context.Context, health *provider.HealthService, value *provider.Record, status int, headers http.Header, body []byte) bool {
	return health.ApplyImageRateLimit(ctx, value, status, func() (bool, time.Time) {
		if !openai.IsImageRateLimitError(status, body) {
			return false, time.Time{}
		}
		return true, openAIImageResetAt(headers, body)
	})
}

// ObserveOpenAIImageCapabilityLoss 仅把供应商能力拒绝观测传给提供商状态规则。
func ObserveOpenAIImageCapabilityLoss(ctx context.Context, health *provider.HealthService, value *provider.Record, status int, body []byte) bool {
	return health.ApplyImageCapabilityLoss(ctx, value, status, openai.IsImageCapabilityLossError(status, body))
}
