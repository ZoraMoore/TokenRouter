package provider

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	// PelicanAnswerLimit 是文本题允许保存的回答字节数。
	PelicanAnswerLimit = 256 << 10
	// PelicanDrawingLimit 是一份绘图回答和独立 SVG 各自的字节上限。
	PelicanDrawingLimit = 2 << 20
)

var (
	// ErrTestAnswerIncomplete 表示上游在完成回答前停止生成。
	ErrTestAnswerIncomplete = errors.New("上游未完成回答")
	// ErrTestAnswerLimit 表示上游响应或提取的回答超过采集上限。
	ErrTestAnswerLimit = errors.New("测智回答超出大小限制")
	// ErrTestAnswerEffort 表示提供商或所选模型不接受指定档位。
	ErrTestAnswerEffort = errors.New("提供商或模型不支持指定的推理强度")
)

// PelicanQuestionConfig 保存管理员指定的题面、参考答案及请求档位。
type PelicanQuestionConfig struct {
	Prompt, ExpectedAnswer, ReasoningEffort string
}

// PelicanEvidence 保存本次实际发送的题目、评判要求和可供管理员查看的回答。
type PelicanEvidence struct {
	Status, DetailCode, Prompt, Answer string
	Criteria                           []string
	ReasoningEffort                    string
}

type pelicanAnswerSink struct {
	text         strings.Builder
	model        string
	protocol     string
	systemPrompt string
	limit        int
	effort       string
	completed    bool
	failed       bool
}

// PelicanQuestionTimeout 给绘图预留生成动画的时间，供手动和分组测试共用。
func PelicanQuestionTimeout(kind string) time.Duration {
	if isPelicanDrawing(kind) {
		return 10 * time.Minute
	}
	return iqQuestionTimeout
}

func isPelicanDrawing(kind string) bool {
	return kind == "pelican" || kind == "custom_svg"
}

func pelicanAnswerLimit(kind string) int {
	if isPelicanDrawing(kind) {
		return PelicanDrawingLimit
	}
	return PelicanAnswerLimit
}

// RunPelican 通过指定提供商执行一题。相同轮次的 SVG 题目和评判要求相同。
func (s *TestService) RunPelican(ctx context.Context, providerID int64, model, kind, round, testProtocol string) PelicanEvidence {
	return s.RunConfiguredPelican(ctx, providerID, model, kind, round, testProtocol, PelicanQuestionConfig{})
}

// RunConfiguredPelican 将参考答案留在本地，仅向选中的模型发送题面和支持的推理参数。
func (s *TestService) RunConfiguredPelican(ctx context.Context, providerID int64, model, kind, round, testProtocol string, config PelicanQuestionConfig) PelicanEvidence {
	result := pelicanQuestion(kind, round, config)
	model = strings.TrimSpace(model)
	if result.Prompt == "" || providerID <= 0 || model == "" || len(model) > 512 || strings.IndexFunc(model, unicode.IsControl) >= 0 {
		return result
	}
	if ctx.Err() != nil {
		result.DetailCode = "incomplete"
		return result
	}
	switch config.ReasoningEffort {
	case "", "low", "medium", "high", "xhigh":
	default:
		result.DetailCode = "unsupported_effort"
		return result
	}
	testType := ProviderTestTypeText
	request := TestRequest{
		ProviderID:      providerID,
		Model:           model,
		Prompt:          result.Prompt,
		Mode:            ProviderTestModeDefault,
		Type:            &testType,
		Automatic:       true,
		FullAnswer:      true,
		SVGAnswer:       isPelicanDrawing(kind),
		Protocol:        testProtocol,
		ReasoningEffort: config.ReasoningEffort,
	}
	ctx, cancel := context.WithTimeout(ctx, PelicanQuestionTimeout(kind))
	defer cancel()
	sink := pelicanAnswerSink{limit: pelicanAnswerLimit(kind)}
	err := s.Test(ctx, request, &sink)
	result.ReasoningEffort = sink.effort
	switch {
	case errors.Is(err, ErrTestAnswerEffort):
		result.DetailCode = "unsupported_effort"
	case errors.Is(err, ErrTestAnswerLimit):
		result.DetailCode = "output_limit"
	case ctx.Err() != nil || errors.Is(err, ErrTestAnswerIncomplete):
		result.DetailCode = "incomplete"
	case err != nil || sink.failed:
		result.DetailCode = "upstream_error"
	case !sink.completed:
		result.DetailCode = "incomplete"
	default:
		result.grade(kind, sink.text.String(), config.ExpectedAnswer)
	}
	return result
}

func (e *PelicanEvidence) grade(kind, raw, expected string) {
	text := strings.TrimSpace(raw)
	expected = strings.TrimSpace(expected)
	switch {
	case text == "":
		e.DetailCode = "empty_output"
	case usesStandardCandyAnswer(kind, expected):
		e.Status, e.DetailCode = candyVerdict(text)
		if e.Status != "inconclusive" {
			e.Answer = text
		}
	case kind == "candy" || kind == "custom_text":
		if !utf8.ValidString(raw) || strings.ContainsRune(raw, '\x00') {
			e.DetailCode = "incomplete"
			return
		}
		e.Answer = raw
		e.Status, e.DetailCode = "passed", ""
		if text != expected {
			e.Status, e.DetailCode = "suspected", "answer_mismatch"
		}
	default:
		e.Answer = pelicanSVG(text)
		if e.Answer == "" {
			e.DetailCode = "invalid_artwork"
			return
		}
		e.Status = "pending_review"
		e.DetailCode = ""
	}
}

func (*pelicanAnswerSink) Begin(context.Context, bool) error { return nil }

func (s *pelicanAnswerSink) Emit(_ context.Context, event TestEvent) error {
	switch event.Type {
	case "test_request":
		s.model, s.effort = event.Model, event.ReasoningEffort
		s.protocol, s.systemPrompt = event.Protocol, event.SystemPrompt
	case "content":
		if len(event.Text) > s.limit-s.text.Len() {
			return ErrTestAnswerLimit
		}
		_, _ = s.text.WriteString(event.Text)
	case "test_complete":
		s.completed = event.Success
	case "error":
		s.failed = true
	}
	return nil
}
