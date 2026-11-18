package provider

import (
	"context"
	"strings"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/provider/modeltrace"
)

// ModelTraceEnvironment 说明实际协议和账号测试请求中的系统提示来源。
type ModelTraceEnvironment struct {
	Protocol     string `json:"protocol"`
	SystemPrompt string `json:"system_prompt"`
	OAuth        bool   `json:"oauth"`
}

// ModelTraceSample 记录一道指纹题的输出和本地归因。
type ModelTraceSample struct {
	Challenge       modeltrace.Challenge  `json:"challenge"`
	Text            string                `json:"text"`
	MappedModel     string                `json:"mapped_model"`
	ReasoningEffort string                `json:"reasoning_effort"`
	Environment     ModelTraceEnvironment `json:"environment"`
	ParsedNumbers   int                   `json:"parsed_numbers"`
	MinimumNumbers  int                   `json:"minimum_numbers"`
	Accepted        bool                  `json:"accepted"`
	Error           string                `json:"error,omitempty"`
	Analysis        *modeltrace.Analysis  `json:"analysis,omitempty"`
	StartedAt       time.Time             `json:"started_at"`
	LatencyMs       int64                 `json:"latency_ms"`
}

// ModelTraceResult 按有效回答数量选取校准参数，保留各题与整轮的归因。
type ModelTraceResult struct {
	RequestedModel  string               `json:"requested_model"`
	ReasoningEffort string               `json:"reasoning_effort"`
	Bank            modeltrace.Metadata  `json:"bank"`
	Samples         []ModelTraceSample   `json:"samples"`
	Analysis        *modeltrace.Analysis `json:"analysis,omitempty"`
	Status          string               `json:"status"`
	Disagreement    bool                 `json:"disagreement"`
	CreatedAt       time.Time            `json:"created_at"`
}

// RunModelTrace 顺序执行三道独立的指纹题。取消或事件接收失败会停止本轮。
func (s *TestService) RunModelTrace(ctx context.Context, id int64, req IQTestRequest, emit func(string, any) error) error {
	if !validIQRequest(id, req) || req.QuestionKind != "" || req.Prompt != "" || req.ExpectedAnswer != "" || req.RoundID != "" || emit == nil {
		return ErrIQInvalid
	}
	target, err := s.iqTarget(ctx, id, req)
	if err != nil {
		return err
	}
	if _, err = traceEnvironment(target, strings.TrimSpace(req.ModelID)); err != nil {
		return err
	}
	release, err := s.acquireIQ(id, true)
	if err != nil {
		return err
	}
	defer release()
	challenges, err := modeltrace.Challenges()
	if err != nil {
		return err
	}
	bank, err := modeltrace.BankMetadata()
	if err != nil {
		return err
	}
	result := ModelTraceResult{RequestedModel: strings.TrimSpace(req.ModelID), Bank: bank, Status: "insufficient", CreatedAt: s.iqNow()}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err = emit("modeltrace_start", struct {
		Challenges []modeltrace.Challenge `json:"challenges"`
		Bank       modeltrace.Metadata    `json:"bank"`
	}{challenges, bank}); err != nil {
		return err
	}
	var outputs []modeltrace.Output
	var firstPrediction string
	for _, challenge := range challenges {
		if err = ctx.Err(); err != nil {
			return err
		}
		sampleCtx, cancel := context.WithTimeout(ctx, iqQuestionTimeout)
		sample := s.traceSample(sampleCtx, id, req, challenge)
		cancel()
		if err = ctx.Err(); err != nil {
			return err
		}
		result.ReasoningEffort = sharedIQEffort(result.ReasoningEffort, sample.ReasoningEffort, len(result.Samples))
		result.Samples = append(result.Samples, sample)
		if sample.Accepted {
			outputs = append(outputs, modeltrace.Output{Text: sample.Text, ExpectedCount: challenge.ExpectedCount})
			if firstPrediction == "" {
				firstPrediction = sample.Analysis.Prediction
			}
			result.Disagreement = result.Disagreement || sample.Analysis.Prediction != firstPrediction
		}
		if err = emit("modeltrace_sample", sample); err != nil {
			return err
		}
	}
	if len(outputs) > 0 {
		result.Analysis, err = modeltrace.Analyze(outputs)
		if err != nil {
			return err
		}
		result.Status = "partial"
		if len(outputs) == len(challenges) {
			result.Status = "complete"
		}
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return emit("modeltrace_complete", result)
}

func traceEnvironment(target TestTarget, model string) (ModelTraceEnvironment, error) {
	info := target.Information()
	oauth := info.Type == ProviderTypeOAuth || info.Type == ProviderTypeSetupToken
	if (!oauth && info.Type != ProviderTypeAPIKey) || info.ParentProviderID != nil || info.AuthMode != "" {
		return ModelTraceEnvironment{}, ErrIQUnsupported
	}
	if info.Platform != PlatformOpenAI && info.Platform != PlatformAnthropic {
		return ModelTraceEnvironment{}, ErrIQUnsupported
	}
	if _, ok := iqProtocol(info); !ok {
		return ModelTraceEnvironment{}, ErrIQUnsupported
	}
	if native, ok := target.(interface{ IQMappedModel(string) string }); ok {
		model = native.IQMappedModel(model)
	}
	if !iqTextModel(model) {
		return ModelTraceEnvironment{}, ErrIQUnsupported
	}
	env := ModelTraceEnvironment{OAuth: oauth}
	return env, nil
}

func (s *TestService) traceSample(ctx context.Context, id int64, req IQTestRequest, challenge modeltrace.Challenge) (sample ModelTraceSample) {
	sample = ModelTraceSample{Challenge: challenge, StartedAt: s.iqNow(), MinimumNumbers: modeltrace.MinimumNumbers(challenge.ExpectedCount)}
	defer func() { sample.LatencyMs = s.iqNow().Sub(sample.StartedAt).Milliseconds() }()
	target, err := s.iqTarget(ctx, id, req)
	if err != nil {
		sample.Error = "account_unavailable"
		if ctx.Err() == context.DeadlineExceeded {
			sample.Error = "timeout"
		}
		return sample
	}
	sample.Environment, err = traceEnvironment(target, strings.TrimSpace(req.ModelID))
	if err != nil {
		sample.Error = "unsupported"
		return sample
	}
	answer, detail := s.iqAnswer(ctx, target, id, req, challenge.Prompt, iqModelTrace, 64<<10)
	sample.MappedModel, sample.ReasoningEffort = answer.model, answer.effort
	sample.Environment.Protocol, sample.Environment.SystemPrompt = answer.protocol, answer.systemPrompt
	if detail != "" {
		sample.Error = detail
		return sample
	}
	sample.Text = answer.text.String()
	sample.ParsedNumbers = len(modeltrace.ParseNumbers(sample.Text))
	if sample.ParsedNumbers < sample.MinimumNumbers {
		sample.Error = "too_few_numbers"
		return sample
	}
	sample.Analysis, err = modeltrace.Analyze([]modeltrace.Output{{Text: sample.Text, ExpectedCount: challenge.ExpectedCount}})
	if err != nil || sample.Analysis == nil {
		sample.Error = "analysis_failed"
		return sample
	}
	sample.Accepted = true
	return sample
}
