package selection

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gatewayprovider "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	providercore "github.com/TokenFlux/TokenRouter/internal/provider"
)

// ResolveOpenAIWSRoutingModelForProvider 为已选定的 WebSocket 提供商逐轮解析并校验分组映射模型。
// 长连接不能在后续 turn 重新调度提供商，因此模型不再适配当前提供商时直接拒绝该帧。
func (s *Compatible) ResolveOpenAIWSRoutingModelForProvider(
	ctx context.Context,
	groupID *int64,
	provider *gatewayprovider.ExecutionProvider,
	requestedModel string,
	requiredCapability providercore.OpenAIEndpointCapability,
) (string, error) {
	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel == "" {
		return "", errors.New("websocket request model is empty")
	}
	if s.CheckGroupModelRestriction(ctx, groupID, requestedModel) {
		return "", fmt.Errorf("model %s is restricted by group model policy", requestedModel)
	}

	routingModel := strings.TrimSpace(s.resolveGroupRoutingModel(ctx, groupID, requestedModel))
	if routingModel == "" {
		routingModel = requestedModel
	}
	if provider == nil || !gatewayprovider.
		CompatibleProviderEligible(
			ctx,
			provider,
			provider.Record.Platform,
			routingModel,
			false,
			requiredCapability,
		) {
		return "", fmt.Errorf("model %s is not supported by the selected websocket provider", requestedModel)
	}
	if s.isOpenAIProviderRequestRuntimeBlocked(provider, routingModel) {
		return "", fmt.Errorf("model %s is temporarily unavailable on the selected websocket provider", requestedModel)
	}
	if groupID != nil && s.NeedsUpstreamGroupRestriction(ctx, groupID) &&
		s.UpstreamRoutingModelRestricted(ctx, *groupID, provider, routingModel, false) {
		return "", fmt.Errorf("model %s is restricted after provider mapping", requestedModel)
	}
	return routingModel, nil
}
