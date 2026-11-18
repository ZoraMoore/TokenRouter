package provider

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

type iqFixture struct {
	info    TestTargetInfo
	loadErr error
	execute func(context.Context, PreparedTestRequest, TestEventSink) error
}

func (f *iqFixture) LoadTestTarget(context.Context, TestRequest) (TestTarget, error) {
	return f, f.loadErr
}
func (f *iqFixture) Information() TestTargetInfo { return f.info }
func (f *iqFixture) Execute(ctx context.Context, req PreparedTestRequest, out TestEventSink) error {
	return f.execute(ctx, req, out)
}

func TestIQQuestionAndQualityEvidence(t *testing.T) {
	run := 0
	fixture := &iqFixture{info: TestTargetInfo{ProviderSnapshot: ProviderSnapshot{ID: 7, Platform: PlatformOpenAI, Type: ProviderTypeAPIKey}}}
	fixture.execute = func(ctx context.Context, req PreparedTestRequest, out TestEventSink) error {
		require.EqualValues(t, 7, req.ProviderID)
		require.Equal(t, "alias", req.Model)
		require.True(t, req.FullAnswer)
		run++
		require.NoError(t, out.Emit(ctx, TestEvent{Type: "test_start", Model: "mapped"}))
		require.NoError(t, out.Emit(ctx, TestEvent{Type: "test_request", Model: "mapped", Protocol: "responses", ReasoningEffort: "high"}))
		require.NoError(t, out.Emit(ctx, TestEvent{Type: "content", Text: "21"}))
		return out.Emit(ctx, TestEvent{Type: "test_complete", Success: true})
	}
	service := NewTestService(fixture, TestOptions{})
	req := IQTestRequest{ModelID: "alias", ReasoningEffort: "high", QuestionKind: "candy"}
	result, err := service.RunIQQuestion(t.Context(), 7, req)
	require.NoError(t, err)
	require.Equal(t, "passed", result.Status)
	require.Equal(t, "mapped", result.MappedModel)
	require.Equal(t, "high", result.ReasoningEffort)
	require.NotEmpty(t, result.RoundID)
	quality, err := service.RunIQQuality(t.Context(), 7, IQTestRequest{ModelID: "alias"})
	require.NoError(t, err)
	require.GreaterOrEqual(t, quality.Total, 2)
	require.Len(t, quality.Samples, quality.Total)
	require.Equal(t, "mismatch", quality.Verdict)
	require.Equal(t, 1+quality.Total, run)
}

// TestIQQuestionKeepsRawDrawing 手动测智也返回提取前的 HTML，便于下载复查。
func TestIQQuestionKeepsRawDrawing(t *testing.T) {
	const raw = `<style>.wheel{stroke:red}</style><svg><circle r="4"/></svg><script>window.frame=1</script>`
	fixture := &iqFixture{info: TestTargetInfo{ProviderSnapshot: ProviderSnapshot{ID: 7, Platform: PlatformOpenAI, Type: ProviderTypeAPIKey}}}
	fixture.execute = func(ctx context.Context, _ PreparedTestRequest, out TestEventSink) error {
		require.NoError(t, out.Emit(ctx, TestEvent{Type: "content", Text: raw}))
		return out.Emit(ctx, TestEvent{Type: "test_complete", Success: true})
	}
	service := NewTestService(fixture, TestOptions{})
	result, err := service.RunIQQuestion(t.Context(), 7, IQTestRequest{ModelID: "drawing", QuestionKind: "pelican"})
	require.NoError(t, err)
	require.Equal(t, raw, result.RawAnswer)
	require.Equal(t, "pending_review", result.Status)
}

func TestIQQuestionDeadlineByKind(t *testing.T) {
	for _, tt := range []struct {
		kind string
		wait time.Duration
	}{
		{"", 10 * time.Minute},
		{"custom_svg", 10 * time.Minute},
		{"candy", 4 * time.Minute},
		{"custom_text", 4 * time.Minute},
	} {
		t.Run(tt.kind, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				fixture := &iqFixture{execute: func(ctx context.Context, _ PreparedTestRequest, sink TestEventSink) error {
					deadline, ok := ctx.Deadline()
					require.True(t, ok)
					require.Equal(t, tt.wait, time.Until(deadline))
					return sink.Emit(ctx, TestEvent{Type: "test_complete", Success: true})
				}}
				service := NewTestService(fixture, TestOptions{})
				_, err := service.RunIQQuestion(t.Context(), 1, IQTestRequest{ModelID: "selected", QuestionKind: tt.kind, Prompt: "画一个正方形。"})
				require.NoError(t, err)
			})
		})
	}
}

func TestIQTraceRequiresCompletionAndStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	runs := 0
	fixture := &iqFixture{info: TestTargetInfo{ProviderSnapshot: ProviderSnapshot{ID: 1, Platform: PlatformOpenAI, Type: ProviderTypeAPIKey}}}
	fixture.execute = func(ctx context.Context, req PreparedTestRequest, out TestEventSink) error {
		runs++
		require.NotContains(t, req.Prompt, "SVG")
		require.NoError(t, out.Emit(ctx, TestEvent{Type: "content", Text: strings.Repeat("1 5 2 ", 110)}))
		return errors.New("sensitive-token")
	}
	s := NewTestService(fixture, TestOptions{})
	var events []string
	err := s.RunModelTrace(ctx, 1, IQTestRequest{ModelID: "gpt-5.4"}, func(name string, data any) error {
		events = append(events, name)
		if name == "modeltrace_sample" {
			v, ok := data.(ModelTraceSample)
			require.True(t, ok)
			require.False(t, v.Accepted)
			require.Empty(t, v.Text)
			require.NotContains(t, v.Error, "sensitive-token")
			cancel()
		}
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, []string{"modeltrace_start", "modeltrace_sample"}, events)
	require.Equal(t, 1, runs)
}

func TestIQConcurrentLimitsAndOpenQuestions(t *testing.T) {
	s := NewTestService(&iqFixture{execute: func(ctx context.Context, _ PreparedTestRequest, out TestEventSink) error {
		if err := out.Emit(ctx, TestEvent{Type: "content", Text: "  一首诗。  "}); err != nil {
			return err
		}
		return out.Emit(ctx, TestEvent{Type: "test_complete", Success: true})
	}}, TestOptions{})
	var releases []func()
	for range 8 {
		release, err := s.acquireIQ(1, false)
		require.NoError(t, err)
		releases = append(releases, release)
	}
	req := IQTestRequest{ModelID: "m", QuestionKind: "custom_text", Prompt: "请作诗。"}
	_, err := s.RunIQQuestion(t.Context(), 1, req)
	require.ErrorIs(t, err, ErrIQBusy)
	for _, release := range releases {
		release()
	}
	out, err := s.RunIQQuestion(t.Context(), 1, req)
	require.NoError(t, err)
	require.Equal(t, "pending_review", out.Status)
	require.Equal(t, "  一首诗。  ", out.Answer)
	trace, err := s.acquireIQ(1, true)
	require.NoError(t, err)
	_, err = s.acquireIQ(1, true)
	require.ErrorIs(t, err, ErrIQBusy)
	trace()
	require.Empty(t, s.iqActive)
}

func TestIQTargetErrorsAndConfirmedTransport(t *testing.T) {
	for _, tt := range []struct{ source, want error }{
		{ErrProviderNotFound, ErrIQNotFound}, {errors.New("sensitive-db-detail"), ErrIQUnavailable},
	} {
		service := NewTestService(&iqFixture{loadErr: tt.source}, TestOptions{})
		_, err := service.RunIQQuestion(t.Context(), 1, IQTestRequest{ModelID: "model", QuestionKind: "candy"})
		require.ErrorIs(t, err, tt.want)
		require.NotContains(t, err.Error(), "sensitive-db-detail")
	}
	fixture := &iqFixture{info: TestTargetInfo{ProviderSnapshot: ProviderSnapshot{Platform: PlatformOpenAI, Type: ProviderTypeAPIKey}}}
	fixture.execute = func(ctx context.Context, _ PreparedTestRequest, out TestEventSink) error {
		if err := out.Emit(ctx, TestEvent{Type: "test_start", Model: "prepared-only"}); err != nil {
			return err
		}
		return errors.New("private-transport-diagnostic")
	}
	service := NewTestService(fixture, TestOptions{})
	question, err := service.RunIQQuestion(t.Context(), 1, IQTestRequest{ModelID: "alias", QuestionKind: "candy", ReasoningEffort: "high"})
	require.NoError(t, err)
	require.Empty(t, question.MappedModel)
	require.Empty(t, question.Protocol)
	require.Empty(t, question.ReasoningEffort)
	quality, err := service.RunIQQuality(t.Context(), 1, IQTestRequest{ModelID: "alias", ReasoningEffort: "high"})
	require.NoError(t, err)
	require.Equal(t, "inconclusive", quality.Verdict)
	require.Empty(t, quality.ReasoningEffort)
	require.Len(t, quality.Samples, 3)
	require.NoError(t, service.RunModelTrace(t.Context(), 1, IQTestRequest{ModelID: "alias", ReasoningEffort: "high"}, func(name string, data any) error {
		if name == "modeltrace_sample" {
			sample, ok := data.(ModelTraceSample)
			require.True(t, ok)
			require.Empty(t, sample.MappedModel)
			require.Empty(t, sample.Environment.Protocol)
			require.Empty(t, sample.Environment.SystemPrompt)
			require.Empty(t, sample.ReasoningEffort)
		}
		if name == "modeltrace_complete" {
			result, ok := data.(ModelTraceResult)
			require.True(t, ok)
			require.Empty(t, result.ReasoningEffort)
			require.Equal(t, "insufficient", result.Status)
			require.Nil(t, result.Analysis)
		}
		return nil
	}))
}

func TestIQQualityDistinguishesIncompleteFromMismatch(t *testing.T) {
	count := 0
	fixture := &iqFixture{execute: func(ctx context.Context, _ PreparedTestRequest, out TestEventSink) error {
		count++
		if count == 3 {
			return ErrTestAnswerIncomplete
		}
		if err := out.Emit(ctx, TestEvent{Type: "content", Text: "10"}); err != nil {
			return err
		}
		return out.Emit(ctx, TestEvent{Type: "test_complete", Success: true})
	}}
	service := NewTestService(fixture, TestOptions{})
	result, err := service.RunIQQuality(t.Context(), 1, IQTestRequest{ModelID: "alias", RoundID: "fixed"})
	require.NoError(t, err)
	require.Equal(t, "inconclusive", result.Verdict)
	require.Equal(t, "suspected", result.Samples[0].Status)
	require.Equal(t, "incomplete", result.Samples[2].DetailCode)
	require.Empty(t, result.Samples[2].Answer)
}

// TestIQCancellationDuringRequest 检查管理员断开连接时终止当前上游请求并释放名额。
func TestIQCancellationDuringRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := make(chan struct{})
	s := NewTestService(&iqFixture{execute: func(ctx context.Context, _ PreparedTestRequest, _ TestEventSink) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}}, TestOptions{})
	done := make(chan error, 1)
	go func() { _, err := s.RunIQQuality(ctx, 1, IQTestRequest{ModelID: "model"}); done <- err }()
	<-started
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	require.Empty(t, s.iqActive)
}

// TestIQModelTraceSampleCounts 复核部分完成、拒答和事件写出失败时保留的样本。
func TestIQModelTraceSampleCounts(t *testing.T) {
	total := 0
	fixture := &iqFixture{info: TestTargetInfo{ProviderSnapshot: ProviderSnapshot{ID: 1, Platform: PlatformAnthropic, Type: ProviderTypeAPIKey}}}
	fixture.execute = func(ctx context.Context, _ PreparedTestRequest, sink TestEventSink) error {
		total++
		if total == 1 {
			return ErrTestAnswerIncomplete
		}
		text := "无法作答"
		if total == 2 {
			text = strings.Repeat("6 7 11 ", 110)
		}
		if err := sink.Emit(ctx, TestEvent{Type: "content", Text: text}); err != nil {
			return err
		}
		return sink.Emit(ctx, TestEvent{Type: "test_complete", Success: true})
	}
	service := NewTestService(fixture, TestOptions{})
	req := IQTestRequest{ModelID: "model"}
	require.NoError(t, service.RunModelTrace(t.Context(), 1, req, func(event string, data any) error {
		if event == "modeltrace_complete" {
			out, ok := data.(ModelTraceResult)
			require.True(t, ok)
			require.Equal(t, "partial", out.Status)
			require.Equal(t, 1, out.Analysis.UsedOutputs)
			require.Equal(t, "incomplete", out.Samples[0].Error)
			require.Equal(t, "too_few_numbers", out.Samples[2].Error)
		}
		return nil
	}))
	writerErr := errors.New("断开")
	require.ErrorIs(t, service.RunModelTrace(t.Context(), 1, req, func(string, any) error { return writerErr }), writerErr)
	require.Equal(t, 3, total)
	require.Empty(t, service.iqActive)
}
