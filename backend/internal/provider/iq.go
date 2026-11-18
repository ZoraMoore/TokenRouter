package provider

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
)

const (
	iqQuestionTimeout = 240 * time.Second
	iqModelTrace      = "modeltrace"
)

var (
	ErrIQInvalid     = errors.New("测智参数无效")
	ErrIQUnsupported = errors.New("此账号或模型不支持所选测试")
	ErrIQBusy        = errors.New("此账号正在执行其他测智请求")
	ErrIQNotFound    = errors.New("提供商不存在")
	ErrIQUnavailable = errors.New("暂时无法读取提供商")
)

// IQTestRequest 指定管理员选择的模型、推理参数和题面。
type IQTestRequest struct {
	ModelID         string `json:"model_id"`
	ReasoningEffort string `json:"reasoning_effort"`
	QuestionKind    string `json:"question_kind"`
	Prompt          string `json:"prompt"`
	ExpectedAnswer  string `json:"expected_answer"`
	RoundID         string `json:"round_id"`
}

// IQQuestionResult 记录单题发出的题面、模型参数以及可复核的回答。
type IQQuestionResult struct {
	QuestionKind    string    `json:"question_kind"`
	RoundID         string    `json:"round_id"`
	RequestedModel  string    `json:"requested_model"`
	MappedModel     string    `json:"mapped_model"`
	Protocol        string    `json:"protocol,omitempty"`
	ReasoningEffort string    `json:"reasoning_effort"`
	Prompt          string    `json:"prompt"`
	Criteria        []string  `json:"criteria"`
	Answer          string    `json:"answer"`
	RawAnswer       string    `json:"raw_answer,omitempty"`
	Status          string    `json:"status"`
	DetailCode      string    `json:"detail_code,omitempty"`
	StartedAt       time.Time `json:"started_at"`
	LatencyMs       int64     `json:"latency_ms"`
}

// IQQualityResult 汇总本轮推理题的实际作答情况。
type IQQualityResult struct {
	RequestedModel  string             `json:"requested_model"`
	ReasoningEffort string             `json:"reasoning_effort"`
	Verdict         string             `json:"verdict"`
	Passed          int                `json:"passed"`
	Total           int                `json:"total"`
	Samples         []IQQuestionResult `json:"samples"`
	StartedAt       time.Time          `json:"started_at"`
	LatencyMs       int64              `json:"latency_ms"`
}

type iqActivity struct {
	questions int
	trace     bool
}

// PrepareIQQuestion 生成一个可在多个请求间复用的题目轮次。
func PrepareIQQuestion(req IQTestRequest) (IQQuestionResult, error) {
	if !validIQRound(req.RoundID) || len(req.Prompt) > 8192 || len(req.ExpectedAnswer) > 1024 {
		return IQQuestionResult{}, ErrIQInvalid
	}
	if req.RoundID == "" {
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			return IQQuestionResult{}, err
		}
		req.RoundID = "manual:" + hex.EncodeToString(id[:])
	}
	if req.QuestionKind == "" {
		req.QuestionKind = "pelican"
	}
	expected := strings.TrimSpace(req.ExpectedAnswer)
	var evidence PelicanEvidence
	if req.QuestionKind == "custom_text" && expected == "" {
		prompt := strings.TrimSpace(req.Prompt)
		if !validPelicanQuestionText(prompt, 8192) {
			return IQQuestionResult{}, ErrIQInvalid
		}
		evidence = PelicanEvidence{Prompt: prompt, Criteria: []string{"对照题面人工查看回答。"}}
	} else {
		evidence = pelicanQuestion(req.QuestionKind, req.RoundID, PelicanQuestionConfig{Prompt: req.Prompt, ExpectedAnswer: expected})
	}
	if evidence.Prompt == "" {
		return IQQuestionResult{}, ErrIQInvalid
	}
	return IQQuestionResult{QuestionKind: req.QuestionKind, RoundID: req.RoundID, RequestedModel: strings.TrimSpace(req.ModelID), Prompt: evidence.Prompt, Criteria: evidence.Criteria}, nil
}

func validIQRound(round string) bool {
	if len(round) > 100 {
		return false
	}
	for _, r := range round {
		alphanumeric := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if !alphanumeric && !strings.ContainsRune("-_:.", r) {
			return false
		}
	}
	return true
}

func validIQRequest(id int64, req IQTestRequest) bool {
	model := strings.TrimSpace(req.ModelID)
	if id <= 0 || model == "" || len(req.ModelID) > 512 || !utf8.ValidString(req.ModelID) || strings.IndexFunc(req.ModelID, unicode.IsControl) >= 0 {
		return false
	}
	switch req.ReasoningEffort {
	case "", "none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra":
		return true
	default:
		return false
	}
}

// acquireIQ 将同账号的题目和质量检测限制在八个并发请求，指纹采样每次单独跑一轮。
func (s *TestService) acquireIQ(id int64, trace bool) (func(), error) {
	s.iqMu.Lock()
	defer s.iqMu.Unlock()
	if s.iqActive == nil {
		s.iqActive = make(map[int64]iqActivity)
	}
	entry := s.iqActive[id]
	if (trace && entry.trace) || (!trace && entry.questions >= 8) {
		return nil, ErrIQBusy
	}
	if trace {
		entry.trace = true
	} else {
		entry.questions++
	}
	s.iqActive[id] = entry
	return sync.OnceFunc(func() {
		s.iqMu.Lock()
		defer s.iqMu.Unlock()
		active := s.iqActive[id]
		if trace {
			active.trace = false
		} else {
			active.questions--
		}
		if active.questions == 0 && !active.trace {
			delete(s.iqActive, id)
		} else {
			s.iqActive[id] = active
		}
	}), nil
}

func (s *TestService) iqNow() time.Time {
	if s.options.Now != nil {
		return s.options.Now()
	}
	return time.Now()
}

func (s *TestService) iqTarget(ctx context.Context, id int64, req IQTestRequest) (TestTarget, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	target, err := s.loader.LoadTestTarget(ctx, TestRequest{ProviderID: id, Model: strings.TrimSpace(req.ModelID)})
	if err != nil || target == nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, ErrProviderNotFound) {
			return nil, ErrIQNotFound
		}
		return nil, ErrIQUnavailable
	}
	return target, nil
}

// RunIQQuestion 指定账号完成一次人工题目，每次请求独立计时。
func (s *TestService) RunIQQuestion(ctx context.Context, id int64, req IQTestRequest) (IQQuestionResult, error) {
	if !validIQRequest(id, req) {
		return IQQuestionResult{}, ErrIQInvalid
	}
	result, err := PrepareIQQuestion(req)
	if err != nil {
		return result, err
	}
	release, err := s.acquireIQ(id, false)
	if err != nil {
		return IQQuestionResult{}, err
	}
	defer release()
	return s.iqQuestion(ctx, id, req, result)
}

func (s *TestService) iqQuestion(ctx context.Context, id int64, req IQTestRequest, result IQQuestionResult) (IQQuestionResult, error) {
	result.StartedAt = s.iqNow()
	req.QuestionKind = result.QuestionKind
	answerCtx, cancel := context.WithTimeout(ctx, PelicanQuestionTimeout(req.QuestionKind))
	defer cancel()
	target, err := s.iqTarget(answerCtx, id, req)
	if err != nil {
		if ctx.Err() == nil && errors.Is(err, context.DeadlineExceeded) {
			result.Status, result.DetailCode = "inconclusive", "timeout"
			result.LatencyMs = s.iqNow().Sub(result.StartedAt).Milliseconds()
			return result, nil
		}
		result.Status, result.DetailCode = "inconclusive", "account_unavailable"
		result.LatencyMs = s.iqNow().Sub(result.StartedAt).Milliseconds()
		return result, err
	}
	sink, detail := s.iqAnswer(answerCtx, target, id, req, result.Prompt, "question", pelicanAnswerLimit(req.QuestionKind))
	if ctx.Err() != nil {
		return IQQuestionResult{}, ctx.Err()
	}
	result.MappedModel, result.ReasoningEffort, result.Protocol = sink.model, sink.effort, sink.protocol
	result.LatencyMs = s.iqNow().Sub(result.StartedAt).Milliseconds()
	result.Status, result.DetailCode = "inconclusive", detail
	if detail != "" {
		return result, nil
	}
	raw := sink.text.String()
	if result.QuestionKind == "custom_text" && strings.TrimSpace(req.ExpectedAnswer) == "" {
		result.Status, result.Answer = "pending_review", raw
	} else {
		graded := PelicanEvidence{Status: "inconclusive"}
		graded.grade(result.QuestionKind, raw, req.ExpectedAnswer)
		result.Status, result.DetailCode, result.Answer = graded.Status, graded.DetailCode, graded.Answer
		result.RawAnswer = graded.RawAnswer
	}
	return result, nil
}

// iqAnswer 收集上游完成后的文字。测试事件中的错误正文可能含有凭据。
func (s *TestService) iqAnswer(ctx context.Context, target TestTarget, id int64, req IQTestRequest, prompt, mode string, limit int) (*pelicanAnswerSink, string) {
	kind := ProviderTestTypeText
	sink := &pelicanAnswerSink{limit: limit}
	protocol, ok := iqProtocol(target.Information())
	if !ok {
		return sink, "unsupported"
	}
	run := TestRequest{
		ProviderID: id, Protocol: protocol, Model: strings.TrimSpace(req.ModelID), Prompt: prompt, Type: &kind,
		Automatic: true, FullAnswer: true, SVGAnswer: isPelicanDrawing(req.QuestionKind), ReasoningEffort: req.ReasoningEffort, IQMode: mode,
	}
	err := s.testTarget(ctx, target, run, sink)
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded):
		return sink, "timeout"
	case errors.Is(err, ErrTestAnswerEffort):
		return sink, "unsupported_effort"
	case errors.Is(err, ErrTestAnswerLimit):
		return sink, "output_limit"
	case errors.Is(err, ErrIQUnsupported):
		return sink, "unsupported"
	case ctx.Err() != nil || errors.Is(err, ErrTestAnswerIncomplete):
		return sink, "incomplete"
	case err != nil || sink.failed:
		return sink, "upstream_error"
	case !sink.completed:
		return sink, "incomplete"
	}
	answer := sink.text.String()
	if strings.TrimSpace(answer) == "" {
		return sink, "empty_output"
	}
	if !utf8.ValidString(answer) || strings.ContainsRune(answer, '\x00') {
		return sink, "incomplete"
	}
	return sink, ""
}

// RunIQQuality 用三道独立题目的参考答案核对当前账号的输出。
func (s *TestService) RunIQQuality(ctx context.Context, id int64, req IQTestRequest) (IQQualityResult, error) {
	if !validIQRequest(id, req) || req.QuestionKind != "" || req.Prompt != "" || req.ExpectedAnswer != "" {
		return IQQualityResult{}, ErrIQInvalid
	}
	base, err := PrepareIQQuestion(IQTestRequest{RoundID: req.RoundID})
	if err != nil {
		return IQQualityResult{}, err
	}
	release, err := s.acquireIQ(id, false)
	if err != nil {
		return IQQualityResult{}, err
	}
	defer release()
	digest := sha256.Sum256([]byte(base.RoundID))
	a, b := 4+int(digest[0]%9), 5+int(digest[1]%8)
	c := 2 + int(digest[2]%4)
	questions := []struct{ kind, prompt, expected string }{
		{"candy", "", "21"},
		{"custom_text", fmt.Sprintf("仓库有 %d 箱货，每箱 %d 件。取走 %d 件，再把余下的货物加倍。现在有多少件？只输出最终整数。", a, b, c), strconv.Itoa(2 * (a*b - c))},
		{"custom_text", fmt.Sprintf("一列数从 %d 开始，每项比前一项多 %d。第 %d 项是多少？只输出最终整数。", a, c, b), strconv.Itoa(a + (b-1)*c)},
	}
	result := IQQualityResult{RequestedModel: strings.TrimSpace(req.ModelID), Total: len(questions), StartedAt: s.iqNow(), Verdict: "passed"}
	mismatched := false
	incomplete := false
	for _, q := range questions {
		req.QuestionKind, req.Prompt, req.ExpectedAnswer, req.RoundID = q.kind, q.prompt, q.expected, base.RoundID
		prepared, err := PrepareIQQuestion(req)
		if err != nil {
			return IQQualityResult{}, err
		}
		sample, err := s.iqQuestion(ctx, id, req, prepared)
		// 首题读账号失败时返回 HTTP 错误，后续失败则保留前面已经拿到的回答。
		if err != nil && (ctx.Err() != nil || len(result.Samples) == 0) {
			return IQQualityResult{}, err
		}
		result.ReasoningEffort = sharedIQEffort(result.ReasoningEffort, sample.ReasoningEffort, len(result.Samples))
		result.Samples = append(result.Samples, sample)
		switch sample.Status {
		case "passed":
			result.Passed++
		case "suspected":
			mismatched = true
		default:
			incomplete = true
		}
	}
	if mismatched {
		result.Verdict = "mismatch"
	}
	if incomplete {
		result.Verdict = "inconclusive"
	}
	result.LatencyMs = s.iqNow().Sub(result.StartedAt).Milliseconds()
	return result, nil
}

// iqProtocol 遵守账号启用的文字协议，双协议账号沿用自己的协议首选项。
func iqProtocol(info TestTargetInfo) (string, bool) {
	if info.EnabledProtocols == nil {
		return "", true
	}
	if len(info.EnabledProtocols) == 0 {
		return "", false
	}
	switch info.Platform {
	case PlatformAnthropic:
		return "", slices.Contains(info.EnabledProtocols, capability.ProtocolAnthropicMessages)
	case PlatformGemini, PlatformAntigravity:
		return "", slices.Contains(info.EnabledProtocols, capability.ProtocolGeminiGenerateContent)
	case PlatformQoder:
		return "", slices.Contains(info.EnabledProtocols, capability.ProtocolQoderChat)
	case PlatformGrok:
		return "", slices.Contains(info.EnabledProtocols, capability.ProtocolOpenAIResponses)
	case PlatformOpenAI:
		// 下方从两种原生文字端点中选取该账号允许的协议。
	default:
		return "", true
	}
	responses := slices.Contains(info.EnabledProtocols, TestProtocolID(APIProtocolResponses))
	chat := info.Type == ProviderTypeAPIKey && slices.Contains(info.EnabledProtocols, TestProtocolID(APIProtocolChatCompletions))
	switch {
	case responses && chat:
		return "", true
	case responses:
		return APIProtocolResponses, true
	case chat:
		return APIProtocolChatCompletions, true
	default:
		return "", false
	}
}

// sharedIQEffort 只有每道题都实际发送同一档位时才展示整轮档位。
func sharedIQEffort(current, sample string, previous int) string {
	if previous == 0 || current == sample {
		return sample
	}
	return ""
}

// iqTextModel 过滤别名指向的图像、音频和向量模型。
func iqTextModel(model string) bool {
	model = strings.ToLower(model)
	if i := strings.LastIndexByte(model, '/'); i >= 0 {
		model = model[i+1:]
	}
	for _, prefix := range []string{"gpt-image", "chatgpt-image", "dall-e", "sora-", "tts-", "whisper-", "text-embedding", "omni-moderation", "gpt-audio", "gpt-realtime", "gpt-4o-audio", "gpt-4o-realtime", "gpt-4o-transcribe", "gpt-4o-mini-audio", "gpt-4o-mini-realtime", "gpt-4o-mini-transcribe", "gpt-4o-mini-tts", "text-moderation"} {
		if strings.HasPrefix(model, prefix) {
			return false
		}
	}
	return model != ""
}
