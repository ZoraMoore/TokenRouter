package httpapi

import (
	"fmt"

	"github.com/TokenFlux/TokenRouter/internal/egress"
	egressprovider "github.com/TokenFlux/TokenRouter/internal/egress/provider"
	gatewayadapter "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	"github.com/TokenFlux/TokenRouter/internal/infra/httpclient"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	provideradapter "github.com/TokenFlux/TokenRouter/internal/provider/provider"
)

// OpenAIRequestOptions 只保存启动时的目标和透传配置。
type OpenAIRequestOptions struct {
	ForceCLI            bool
	AllowTimeoutHeaders bool
	URLPolicy           egress.OperatorURLPolicy
}

// OpenAIRequests 统一构造协议请求及传输参数，不选择提供商或提交完成任务。
type OpenAIRequests struct {
	Options      OpenAIRequestOptions
	Providers    gatewayadapter.ExecutionProviderStore
	Identity     *gatewayadapter.ExecutionAgentIdentity
	Credentials  *provider.OpenAIExecutionCredentials
	Transport    httpclient.UpstreamTransport
	Failure      *UpstreamTransportFailure
	Turns        *CodexTurnStateHeaders
	Profiles     *egressprovider.TLSProfiles
	Routers      *egress.TLSFingerprintRouterService
	Readers      *gatewayadapter.RuntimeReaders
	Detector     provider.ClientRestrictionDetector
	ClientPolicy *provideradapter.OpenAIProbePolicy
	GrokRoutes   gatewayadapter.GrokRoutes
}

// ValidateBaseURL 保持原错误前缀，URL 规则由 egress 唯一执行。
func (s *OpenAIRequests) ValidateBaseURL(raw string) (string, error) {
	normalized, err := s.ValidateURL(raw)
	if err != nil {
		return "", fmt.Errorf("invalid base_url: %w", err)
	}
	return normalized, nil
}

func (s *OpenAIRequests) AllowTimeoutHeaders() bool { return s != nil && s.Options.AllowTimeoutHeaders }

// ValidateURL 保留未配置时的 HTTPS 格式约束。
func (s *OpenAIRequests) ValidateURL(raw string) (string, error) {
	if s == nil {
		return egress.ValidateURLFormat(raw, false)
	}
	return s.Options.URLPolicy.Validate(raw)
}
