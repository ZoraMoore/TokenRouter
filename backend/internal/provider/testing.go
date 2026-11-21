package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
)

const (
	TestRouteClaude TestRoute = iota
	TestRouteCNAdaptive
	TestRouteCNResponses
	TestRouteCNChat
	TestRouteCNAnthropic
	TestRouteOpenAI
	TestRouteGemini
	TestRouteGrok
	TestRouteAntigravity
	TestRouteQoder

	ProviderTestTypeText          = "text"
	ProviderTestTypeImage         = "image"
	ProviderTestModeDefault       = "default"
	ProviderTestModeCompact       = "compact"
	ProviderTestModeLegacyCompact = "legacy_compact"

	scheduledAuthTimeout = 45 * time.Second
)

var errScheduledAuthFailed = errors.New("提供商认证复检失败")

// TestEvent represents a SSE event for provider testing.
type TestEvent struct {
	Type            string `json:"type"`
	Text            string `json:"text,omitempty"`
	Model           string `json:"model,omitempty"`
	Status          string `json:"status,omitempty"`
	Code            string `json:"code,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	Protocol        string `json:"protocol,omitempty"`
	SystemPrompt    string `json:"system_prompt,omitempty"`
	ImageURL        string `json:"image_url,omitempty"`
	MimeType        string `json:"mime_type,omitempty"`
	Data            any    `json:"data,omitempty"`
	Success         bool   `json:"success,omitempty"`
	Error           string `json:"error,omitempty"`
	// HTTPStatus 记录后台用例读取到的上游状态码。
	HTTPStatus int `json:"-"`
}

// TestRequest 表达测试意图；Type 为 nil 时保留历史模型名推断，客户端元数据不包含凭据。
type TestRequest struct {
	ProviderID            int64
	Model, Prompt, Mode   string
	Type                  *string
	Protocol              string
	UserAgent, Originator string
	Automatic             bool
	// FullAnswer 要求选中的上游完成一次整题作答。
	FullAnswer      bool
	ReasoningEffort string
	// SVGAnswer 为文字协议返回的绘图作品选择采集上限。
	SVGAnswer bool
	// IQMode 区分管理员发起的题目和数字指纹采样。
	IQMode string
	// AuthObservation 为定向认证复检选择文字模型，读取上游端点的首个 HTTP 响应。
	AuthObservation bool
}
type TestRoute uint8

type PreparedTestRequest struct {
	TestRequest
	TestType     string
	ExplicitType bool
	Route        TestRoute
}
type TestTargetInfo struct {
	ProviderSnapshot
	APIProtocol string
}

// TestTarget 提供单次测试执行和脱敏的提供商快照。
type TestTarget interface {
	Information() TestTargetInfo
	Execute(context.Context, PreparedTestRequest, TestEventSink) error
}
type TestLoader interface {
	LoadTestTarget(context.Context, TestRequest) (TestTarget, error)
}

// Begin 的 commit 区分仅准备元数据与立即提交；HTTP Header/Flush 的实现由 Adapter 拥有。
type TestEventSink interface {
	Begin(context.Context, bool) error
	Emit(context.Context, TestEvent) error
}
type TestOptions struct {
	Now        func() time.Time
	Error      func(string)
	WriteError func(error)
}
type TestService struct {
	stateProbes sync.Map
	loader      TestLoader
	options     TestOptions
	iqMu        sync.Mutex
	iqActive    map[int64]iqActivity
}

// 首次输出失败立即取消执行上下文，后续写出复用相同错误。
type testGuardedSink struct {
	mu     sync.Mutex
	next   TestEventSink
	cancel context.CancelFunc
	err    error
}

// 后台直接收集事件，通过 JSON 编码处理非法 UTF-8，编码失败的 Data 被丢弃。
type testResultSink struct {
	mu    sync.Mutex
	texts []string
	err   string
}

// authObservationSink 保存上游状态和完成信号。
type authObservationSink struct {
	status   int
	complete bool
}

// NormalizeProviderTestType 统一管理端传入的测试类型，空值由调用方按兼容方式处理。
func NormalizeProviderTestType(testType string) string {
	switch strings.ToLower(strings.TrimSpace(testType)) {
	case ProviderTestTypeImage:
		return ProviderTestTypeImage
	case ProviderTestTypeText:
		return ProviderTestTypeText
	default:
		return ProviderTestTypeText
	}
}

// ProviderTestTypeFromArgs 返回类型以及是否由调用方明确指定。
// 省略类型时按模型名判断，管理端请求传入 text 或 image。
func ProviderTestTypeFromArgs(testTypes ...string) (string, bool) {
	if len(testTypes) == 0 || strings.TrimSpace(testTypes[0]) == "" {
		return ProviderTestTypeText, false
	}
	return NormalizeProviderTestType(testTypes[0]), true
}

// ResolveProviderTestModeAndType 兼容少量旧调用把 text/image 放在 mode 字段的情况。
func ResolveProviderTestModeAndType(mode string, testTypes ...string) (string, string, bool) {
	// 兼容新调用方把参数顺序写成 testType、mode 的形式。
	if len(testTypes) > 0 {
		rawMode := strings.ToLower(strings.TrimSpace(mode))
		rawTypeOrMode := strings.ToLower(strings.TrimSpace(testTypes[0]))
		if (rawMode == ProviderTestTypeText || rawMode == ProviderTestTypeImage) &&
			(rawTypeOrMode == ProviderTestModeDefault || rawTypeOrMode == ProviderTestModeCompact || rawTypeOrMode == ProviderTestModeLegacyCompact) {
			return NormalizeProviderTestMode(testTypes[0]), NormalizeProviderTestType(mode), true
		}
	}
	testType, explicit := ProviderTestTypeFromArgs(testTypes...)
	normalizedMode := NormalizeProviderTestMode(mode)
	if !explicit {
		switch strings.ToLower(strings.TrimSpace(mode)) {
		case ProviderTestTypeText, ProviderTestTypeImage:
			return ProviderTestModeDefault, NormalizeProviderTestType(mode), true
		}
	}
	if !explicit {
		testType = ""
	}
	return normalizedMode, testType, explicit
}

func NormalizeProviderTestMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ProviderTestModeCompact:
		return ProviderTestModeCompact
	case ProviderTestModeLegacyCompact:
		return ProviderTestModeLegacyCompact
	default:
		return ProviderTestModeDefault
	}
}

func NewTestService(loader TestLoader, options TestOptions) *TestService {
	return &TestService{loader: loader, options: options}
}

func (s *TestService) Test(ctx context.Context, request TestRequest, sink TestEventSink) error {
	return s.testTarget(ctx, nil, request, sink)
}

// ObserveScheduledAuth 使用指定账号的当前凭据，读取上游 HTTP 状态和完整响应信号。
// @project-doc docs/operations/provider_maintenance.md#maintenance_schedules
func (s *TestService) ObserveScheduledAuth(ctx context.Context, id int64, model string) (status int, complete bool, err error) {
	model = strings.TrimSpace(model)
	if id <= 0 || (model != "" && !ValidStateProbeModelName(model)) {
		return 0, false, ErrIQInvalid
	}

	ctx, cancel := context.WithTimeout(ctx, scheduledAuthTimeout)
	defer cancel()
	target, err := s.iqTarget(ctx, id, IQTestRequest{ModelID: model})
	if err != nil {
		return 0, false, err
	}
	info := target.Information()
	if info.ID != id {
		return 0, false, ErrIQUnavailable
	}
	if !supportsStateProbeSnapshot(info.ProviderSnapshot) ||
		(info.Status != StatusActive && info.Status != StatusError) ||
		(info.ExpiresAt != nil && !s.iqNow().Before(*info.ExpiresAt)) {
		return 0, false, ErrIQUnsupported
	}

	testType := ProviderTestTypeText
	request := TestRequest{
		ProviderID: id, Model: model, Prompt: "只回复 OK。", Type: &testType, Protocol: APIProtocolResponses,
		Automatic: true, FullAnswer: true, AuthObservation: true,
	}
	var sink authObservationSink
	err = s.testTarget(ctx, target, request, &sink)
	switch {
	case ctx.Err() != nil:
		return sink.status, false, ctx.Err()
	case sink.status != 0 && sink.status != 200:
		return sink.status, false, nil
	case errors.Is(err, ErrTestAnswerIncomplete):
		return sink.status, false, ErrTestAnswerIncomplete
	case errors.Is(err, ErrTestAnswerLimit):
		return sink.status, false, ErrTestAnswerLimit
	case errors.Is(err, ErrIQUnsupported):
		return sink.status, false, ErrIQUnsupported
	case err != nil:
		return sink.status, false, errScheduledAuthFailed
	case sink.status != 200 || !sink.complete:
		return sink.status, false, ErrTestAnswerIncomplete
	default:
		return sink.status, true, nil
	}
}

// testTarget 让手动检测在刚校验过的账号快照上执行一次。
func (s *TestService) testTarget(ctx context.Context, target TestTarget, request TestRequest, sink TestEventSink) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	guarded := &testGuardedSink{next: sink, cancel: cancel}
	err := s.execute(runCtx, target, request, guarded)
	if writeErr := guarded.failure(); writeErr != nil && !errors.Is(err, writeErr) {
		return errors.Join(err, writeErr)
	}
	return err
}

func (s *TestService) execute(ctx context.Context, target TestTarget, request TestRequest, sink TestEventSink) error {
	if request.Protocol != "" && TestProtocolID(request.Protocol) == "" {
		return s.fail(ctx, sink, "Invalid test protocol")
	}
	var types []string
	if request.Type != nil {
		types = []string{*request.Type}
	}
	mode, kind, explicit := ResolveProviderTestModeAndType(request.Mode, types...)
	if explicit && kind == ProviderTestTypeImage {
		mode = ProviderTestModeDefault
	}
	if target == nil {
		var err error
		target, err = s.loader.LoadTestTarget(ctx, request)
		if err != nil {
			return s.fail(ctx, sink, "Provider not found")
		}
	}
	info := target.Information()
	claudeIQ := (request.SVGAnswer || request.IQMode != "") && info.Platform == PlatformAnthropic && (info.Type == ProviderTypeAPIKey || info.Type == ProviderTypeOAuth || info.Type == ProviderTypeSetupToken)
	if request.FullAnswer && request.ReasoningEffort != "" && info.Platform != PlatformOpenAI && !claudeIQ {
		return ErrTestAnswerEffort
	}
	if explicit && kind == ProviderTestTypeImage && info.Platform != PlatformOpenAI && info.Platform != PlatformGemini && info.Platform != PlatformGrok {
		return s.fail(ctx, sink, fmt.Sprintf("Image tests are not supported for platform %s", info.Platform))
	}
	if request.Protocol != "" && kind != ProviderTestTypeImage {
		disabled := request.FullAnswer && !slices.Contains(info.EnabledProtocols, TestProtocolID(request.Protocol))
		if disabled || !testProtocolAllowed(info, request.Protocol) {
			return s.fail(ctx, sink, fmt.Sprintf("Test protocol %s is not supported for this provider", request.Protocol))
		}
	}
	prepared := PreparedTestRequest{TestRequest: request, TestType: kind, ExplicitType: explicit}
	prepared.Mode = mode
	imageRequest := kind == ProviderTestTypeImage || (!explicit && (strings.Contains(strings.ToLower(request.Model), "image") || strings.Contains(strings.ToLower(request.Model), "imagen")))
	if mode == ProviderTestModeDefault && !imageRequest && strings.TrimSpace(prepared.Prompt) == "" {
		prepared.Prompt = DefaultProviderTestPrompt()
	}
	switch info.Platform {
	case PlatformKimi, PlatformZhipu, PlatformDeepseek:
		switch info.APIProtocol {
		case APIProtocolAdaptive:
			prepared.Route = TestRouteCNAdaptive
		case APIProtocolResponses:
			prepared.Route = TestRouteCNResponses
		case APIProtocolChatCompletions:
			prepared.Route = TestRouteCNChat
		default:
			prepared.Route = TestRouteCNAnthropic
		}
	case PlatformOpenAI:
		prepared.Route = TestRouteOpenAI
	case PlatformGemini:
		prepared.Route = TestRouteGemini
	case PlatformGrok:
		prepared.Route = TestRouteGrok
	case PlatformAntigravity:
		prepared.Route = TestRouteAntigravity
	case PlatformQoder:
		prepared.Route = TestRouteQoder
	default:
		prepared.Route = TestRouteClaude
	}
	return target.Execute(ctx, prepared, sink)
}

// TestProtocolID 把管理端传入的测试协议转换为原生协议 ID，未知值返回空字符串。
func TestProtocolID(protocol string) capability.ProtocolID {
	switch protocol {
	case APIProtocolAnthropic:
		return capability.ProtocolAnthropicMessages
	case APIProtocolResponses:
		return capability.ProtocolOpenAIResponses
	case APIProtocolChatCompletions:
		return capability.ProtocolOpenAIChatCompletions
	default:
		return ""
	}
}

// testProtocolAllowed 判断本次文字测试能否直连所选协议。
// OpenAI API Key 可在 Responses 与 Chat 之间任选，OAuth 只有 Responses；
// 国产平台可测试已启用的协议，其他平台使用各自固定的测试端点。
func testProtocolAllowed(info TestTargetInfo, protocol string) bool {
	switch info.Platform {
	case PlatformOpenAI:
		if info.Type == ProviderTypeAPIKey {
			return protocol != APIProtocolAnthropic
		}
		return protocol == APIProtocolResponses
	case PlatformKimi, PlatformZhipu, PlatformDeepseek:
		return slices.Contains(info.EnabledProtocols, TestProtocolID(protocol))
	default:
		return false
	}
}

func (s *TestService) fail(ctx context.Context, sink TestEventSink, message string) error {
	if s.options.Error != nil {
		s.options.Error(message)
	}
	if err := sink.Emit(ctx, TestEvent{Type: "error", Error: message}); err != nil && s.options.WriteError != nil {
		s.options.WriteError(err)
	}
	return errors.New(message)
}

func (s *testGuardedSink) apply(write func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	if err := write(); err != nil {
		s.err = err
		s.cancel()
	}
	return s.err
}

func (s *testGuardedSink) Begin(ctx context.Context, commit bool) error {
	return s.apply(func() error { return s.next.Begin(ctx, commit) })
}

func (s *testGuardedSink) Emit(ctx context.Context, event TestEvent) error {
	return s.apply(func() error { return s.next.Emit(ctx, event) })
}
func (s *testGuardedSink) failure() error { s.mu.Lock(); defer s.mu.Unlock(); return s.err }
func (s *TestService) RunTestBackground(ctx context.Context, id int64, model string) (*ScheduledTestResult, error) {
	return s.RunTestBackgroundWithPromptAndUserAgent(ctx, id, model, "", "")
}

func (s *TestService) RunTestBackgroundWithPromptAndUserAgent(ctx context.Context, id int64, model, prompt, userAgent string) (*ScheduledTestResult, error) {
	started := s.options.Now()
	sink := &testResultSink{}
	testErr := s.Test(ctx, TestRequest{ProviderID: id, Model: model, Prompt: prompt, Mode: ProviderTestModeDefault, Automatic: true, UserAgent: userAgent}, sink)
	finished := s.options.Now()
	text, message := sink.result()
	status := "success"
	if testErr != nil || message != "" {
		status = "failed"
		if message == "" && testErr != nil {
			message = testErr.Error()
		}
	}
	return &ScheduledTestResult{Status: status, ResponseText: text, ErrorMessage: message, LatencyMs: finished.Sub(started).Milliseconds(), StartedAt: started, FinishedAt: finished}, nil
}

func (*testResultSink) Begin(context.Context, bool) error { return nil }
func (s *testResultSink) Emit(_ context.Context, event TestEvent) error {
	if event.Type != "content" && event.Type != "error" {
		return nil
	}
	raw, err := json.Marshal(event)
	if err != nil {
		return nil
	}
	var wire TestEvent
	if json.Unmarshal(raw, &wire) != nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch wire.Type {
	case "content":
		if wire.Text != "" {
			s.texts = append(s.texts, wire.Text)
		}
	case "error":
		s.err = wire.Error
	}
	return nil
}

func (s *testResultSink) result() (string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Join(s.texts, ""), s.err
}

func (*authObservationSink) Begin(context.Context, bool) error { return nil }

func (s *authObservationSink) Emit(_ context.Context, event TestEvent) error {
	switch event.Type {
	case "test_request":
		s.status = event.HTTPStatus
	case "test_complete":
		s.complete = event.Success
	}
	return nil
}
