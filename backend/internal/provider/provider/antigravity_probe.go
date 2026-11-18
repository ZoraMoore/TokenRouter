package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/infra/telemetry/logging"
	"github.com/TokenFlux/TokenRouter/internal/pkg/logredact"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/upstream/antigravity"
)

// AntigravityProbe 是指定提供商测试的闭合入口，复用实际平台重试与应用尝试屏障。
type AntigravityProbe struct {
	Tokens *provider.AntigravityTokenSource
	Retry  *AntigravityRetry
	Enter  func() (func(), error)
}

func (s *AntigravityProbe) Execute(ctx context.Context, value *provider.Record, request provider.PreparedTestRequest) (*antigravity.TestConnectionResult, error) {
	if s.Tokens == nil {
		return nil, errors.New("antigravity token provider not configured")
	}
	token, err := s.Tokens.GetAccessToken(ctx, value)
	if err != nil {
		return nil, fmt.Errorf("获取 access_token 失败: %w", err)
	}
	project, err := provider.ResolveAntigravityProjectID(value, antigravity.ErrProjectIDRequired)
	if err != nil {
		return nil, err
	}
	model := MapAntigravityModel(value, request.Model)
	if model == "" {
		return nil, fmt.Errorf("model %s not in whitelist", request.Model)
	}
	prompt := "."
	if strings.TrimSpace(request.Prompt) != "" {
		prompt = strings.TrimSpace(request.Prompt)
	}
	proxy := ""
	if value.ProxyID != nil && value.Proxy != nil {
		proxy = value.Proxy.URL()
	}
	if request.FullAnswer {
		return s.answer(ctx, value, request, project, model, prompt, token, proxy)
	}
	var body []byte
	if strings.HasPrefix(request.Model, "gemini-") {
		body, err = antigravity.BuildGeminiTestRequest(project, model, prompt)
	} else {
		body, err = antigravity.BuildClaudeTestRequest(project, model, prompt)
	}
	if err != nil {
		return nil, fmt.Errorf("构建请求失败: %w", err)
	}
	prefix := fmt.Sprintf("[antigravity-Test] provider=%d(%s)", value.ID, value.Name)
	input := AntigravityRetryRequest{
		Context: ctx, Provider: value, ModelStore: s.Retry.Health.Store, Prefix: prefix, ProxyURL: proxy, AccessToken: token, Action: "streamGenerateContent", Body: body, RequestedModel: request.Model,
		HandleError: func(status int, _ http.Header, body []byte) {
			logging.LegacyPrintf("service.antigravity_gateway", "%s test_handle_error status=%d model=%s provider=%d body=%s", prefix, status, request.Model, value.ID, logredact.TruncateLine(body, 200))
		},
	}
	if request.Automatic {
		input.UserAgent = request.UserAgent
	}
	adapter, attempt := s.Retry.Bind(input)
	return antigravity.Probe(ctx, attempt, adapter.Options, model, s.Retry.BodyLimit, s.Enter)
}

// answer 在提供商配置的地址发起一次生成，收到 STOP 后返回该模型的可见文字。
func (s *AntigravityProbe) answer(ctx context.Context, value *provider.Record, request provider.PreparedTestRequest, project, model, prompt, token, proxy string) (*antigravity.TestConnectionResult, error) {
	if s.Retry == nil || s.Retry.BaseURL == nil || s.Retry.Do == nil {
		return nil, errAnswerUpstream
	}
	if s.Enter != nil {
		done, err := s.Enter()
		if err != nil {
			return nil, errAnswerUpstream
		}
		defer done()
	}
	body, err := antigravity.BuildAnswerRequest(project, model, prompt, pelicanMaxTokens)
	if err != nil {
		return nil, errAnswerUpstream
	}
	req, err := antigravity.NewAPIRequestWithURL(ctx, s.Retry.BaseURL(value), "streamGenerateContent", token, body)
	if err != nil {
		return nil, errAnswerUpstream
	}
	if agent := strings.TrimSpace(request.UserAgent); agent != "" {
		req.Header.Set("User-Agent", agent)
	}
	resp, err := s.Retry.Do(req, proxy, value.ID, value.Concurrency)
	if err != nil || resp == nil || resp.Body == nil {
		return nil, errAnswerUpstream
	}
	defer func() { _ = resp.Body.Close() }()
	result := &antigravity.TestConnectionResult{MappedModel: model, ReasoningEffort: sentAnswerEffort(answerGemini, body)}
	result.Text, err = completedTestAnswer(ctx, resp, answerGemini, testAnswerLimits(request.SVGAnswer, request.IQMode))
	return result, err
}
