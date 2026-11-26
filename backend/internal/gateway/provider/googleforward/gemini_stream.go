package googleforward

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	forwardcore "github.com/TokenFlux/TokenRouter/internal/gateway/forward"
	gatewayprovider "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	providercore "github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/upstream"
)

// streamFailure 将已识别的流式错误交给提供商策略和当前协议的错误输出。
func (s *Gemini) streamFailure(ctx context.Context, c *attempt, provider *gatewayprovider.ExecutionProvider, result upstream.AttemptResult, failure error, present func(int, []byte) error) error {
	var streamError *upstream.GeminiStreamError
	if !errors.As(failure, &streamError) || ctx.Err() != nil || result.ClientDisconnect {
		return failure
	}
	body := streamError.Body
	if len(body) == 0 {
		body, _ = json.Marshal(map[string]any{"error": map[string]any{"code": streamError.Status, "message": streamError.Message}})
	}
	decision := s.applyGeminiUpstreamErrorPolicy(ctx, provider, streamError.Status, result.UpstreamHeaders.Clone(), body, result.UpstreamModel)
	committed := true
	if state, ok := c.Sink().(interface{ OutputState() upstream.OutputHead }); ok {
		committed = state.OutputState().Committed
	}
	canSwitch := s.shouldFailoverGeminiUpstreamError(streamError.Status)
	if decision.Policy != providercore.ErrorPolicyCustomSkipped && decision.Policy != providercore.ErrorPolicyPoolBypassed {
		canSwitch = decision.ShouldFailover(gatewayprovider.ExecutionErrorPolicy(provider), streamError.Status, canSwitch)
	}
	if !committed && canSwitch {
		if retry := s.skippedErrorPolicyFailoverError(c, provider, streamError.Status, body, result.RequestID); retry != nil {
			return retry
		}
		return &forwardcore.UpstreamFailoverError{
			StatusCode: streamError.Status, ResponseBody: body,
			RetryableOnSameProvider: provider.View().IsPoolMode() && provider.View().IsPoolModeRetryableStatus(streamError.Status),
		}
	}
	if decision.Policy == providercore.ErrorPolicyCustomSkipped {
		presented := c.GeminiCustomCodeSkippedError(provider, streamError.Status, result.RequestID, body, func() {
			fallback, _ := json.Marshal(map[string]any{"error": map[string]any{"code": 500, "message": geminiCustomCodeSkippedClientMessage}})
			_ = present(http.StatusInternalServerError, fallback)
		})
		return errors.Join(failure, presented)
	}
	return errors.Join(failure, present(streamError.Status, body))
}
