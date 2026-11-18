package provider

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type pelicanTestTarget func(context.Context, PreparedTestRequest, TestEventSink) error

func (f pelicanTestTarget) LoadTestTarget(context.Context, TestRequest) (TestTarget, error) {
	return f, nil
}

func (pelicanTestTarget) Information() TestTargetInfo {
	return TestTargetInfo{ProviderSnapshot: ProviderSnapshot{Platform: PlatformOpenAI}}
}

func (f pelicanTestTarget) Execute(ctx context.Context, request PreparedTestRequest, sink TestEventSink) error {
	return f(ctx, request, sink)
}

func TestRunPelicanRequiresCompletedAnswer(t *testing.T) {
	const drawing = `<svg xmlns="http://www.w3.org/2000/svg"><circle r="7"/></svg>`
	const animation = `<svg xmlns="http://www.w3.org/2000/svg"><defs><circle id="wheel" r="7"/></defs><style>.road{animation:move 2s infinite}@keyframes move{to{opacity:0}}</style><use class="road" href="#wheel"><animate attributeName="x" from="1" to="10" dur="2s" repeatCount="indefinite"/></use></svg>`
	const sensitive = "https://fixture.invalid/private?token=should-never-be-saved"
	for _, tt := range []struct {
		name, kind, text, status, code, answer string
		complete, fail                         bool
	}{
		{name: "上游错误丢弃已经收到的文字", kind: "candy", text: "21", complete: true, fail: true, status: "inconclusive", code: "upstream_error"},
		{name: "断流后已有答案仍不能判分", kind: "candy", text: "21", status: "inconclusive", code: "incomplete"},
		{name: "完成但没有回答", kind: "candy", text: "  ", complete: true, status: "inconclusive", code: "empty_output"},
		{name: "超限的 SVG", kind: "pelican", text: strings.Repeat("x", (2<<20)+1), complete: true, status: "inconclusive", code: "output_limit"},
		{name: "截断的 SVG", kind: "pelican", text: "<svg><path/>", complete: true, status: "inconclusive", code: "invalid_artwork"},
		{name: "只有 HTML", kind: "pelican", text: "<html><canvas></canvas></html>", complete: true, status: "inconclusive", code: "invalid_artwork"},
		{name: "SVG 没有可绘制元素", kind: "pelican", text: "<svg/>", complete: true, status: "inconclusive", code: "invalid_artwork"},
		{name: "仅有未引用的图形定义", kind: "pelican", text: `<svg><defs><circle id="wheel" r="7"/></defs></svg>`, complete: true, status: "inconclusive", code: "invalid_artwork"},
		{name: "图像属性重复会使 XML 解析失败", kind: "pelican", text: `<svg width="10" width="20"><circle r="7"/></svg>`, complete: true, status: "inconclusive", code: "invalid_artwork"},
		{name: "HTML 中的 SVG 单独预览", kind: "pelican", text: "<html><body>" + drawing + "</body></html>", complete: true, status: "pending_review", answer: drawing},
		{name: "CSS 和 SMIL 动画可供复核", kind: "pelican", text: animation, complete: true, status: "pending_review", answer: animation},
		{name: "HTML 内联 SVG 补上图像命名空间", kind: "pelican", text: `<svg><circle r="7"/></svg>`, complete: true, status: "pending_review", answer: drawing},
		{name: "内联图像补上 xlink 定义", kind: "pelican", text: `<svg><use xlink:href="#wheel"/></svg>`, complete: true, status: "pending_review", answer: `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"><use xlink:href="#wheel"/></svg>`},
		{name: "图像含未声明的 XML 前缀", kind: "pelican", text: `<svg><circle unknown:radius="7"/></svg>`, complete: true, status: "inconclusive", code: "invalid_artwork"},
		{name: "脚本文字中的伪标签", kind: "pelican", text: "<html><script>const illustration = '" + drawing + "';</script></html>", complete: true, status: "inconclusive", code: "invalid_artwork"},
		{name: "完整但无法解析的 SVG", kind: "pelican", text: "<svg><g></svg>", complete: true, status: "inconclusive", code: "invalid_artwork"},
		{name: "外链图像", kind: "pelican", text: `<svg><image href="https://fixture.invalid/a"/></svg>`, complete: true, status: "inconclusive", code: "invalid_artwork"},
		{name: "短的错误数值", kind: "candy", text: "答案是 22 个糖果。", complete: true, status: "suspected", code: "answer_mismatch", answer: "答案是 22 个糖果。"},
		{name: "无数字的上游提示", kind: "candy", text: "temporarily unavailable " + sensitive, complete: true, status: "inconclusive", code: "incomplete"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var actualPrompt string
			service := NewTestService(pelicanTestTarget(func(ctx context.Context, request PreparedTestRequest, sink TestEventSink) error {
				require.Equal(t, int64(42), request.ProviderID)
				require.Equal(t, "selected-image-name", request.Model)
				require.Equal(t, ProviderTestTypeText, request.TestType)
				require.True(t, request.ExplicitType)
				actualPrompt = request.Prompt
				if err := sink.Emit(ctx, TestEvent{Type: "content", Text: tt.text}); err != nil {
					require.Error(t, ctx.Err())
					return err
				}
				if tt.complete {
					require.NoError(t, sink.Emit(ctx, TestEvent{Type: "test_complete", Success: true}))
				}
				if tt.fail {
					require.NoError(t, sink.Emit(ctx, TestEvent{Type: "error", Error: sensitive}))
					return errors.New(sensitive)
				}
				return nil
			}), TestOptions{})
			ev := service.RunPelican(t.Context(), 42, "selected-image-name", tt.kind, "1", "")
			require.Equal(t, tt.status, ev.Status)
			require.Equal(t, tt.code, ev.DetailCode)
			require.Equal(t, tt.answer, ev.Answer)
			require.NotEmpty(t, ev.Criteria)
			require.NotEmpty(t, actualPrompt)
			require.Equal(t, actualPrompt, ev.Prompt)
		})
	}
}

// TestPelicanKeepsOriginalDrawing 保存提取前的 HTML，供复核样式、脚本和格式错误。
func TestPelicanKeepsOriginalDrawing(t *testing.T) {
	for _, drawing := range []string{`<svg><circle r="7"/></svg>`, `<svg><g></svg>`} {
		raw := " \n<!doctype html><html><head><style>.wheel{animation:spin 1s infinite}</style></head><body>" + drawing + `<script>window.frame=1;</script></body></html>` + "\n"
		evidence := PelicanEvidence{Status: "inconclusive"}
		evidence.grade("pelican", raw, "")
		require.Equal(t, raw, evidence.RawAnswer)
		if strings.Contains(drawing, "circle") {
			require.Equal(t, "pending_review", evidence.Status)
			require.NotContains(t, evidence.Answer, "<script>")
		} else {
			require.Equal(t, "invalid_artwork", evidence.DetailCode)
		}
	}
	for _, raw := range []string{"<svg>\x00</svg>", string([]byte{0xff}), strings.Repeat("x", PelicanDrawingLimit+1)} {
		evidence := PelicanEvidence{Status: "inconclusive"}
		evidence.grade("pelican", raw, "")
		require.Empty(t, evidence.RawAnswer)
	}
}

func TestRunPelicanCancelledBeforeOrDuringRequest(t *testing.T) {
	for _, alreadyCancelled := range []bool{true, false} {
		ctx, cancel := context.WithCancel(t.Context())
		if alreadyCancelled {
			cancel()
		}
		called := false
		service := NewTestService(pelicanTestTarget(func(ctx context.Context, _ PreparedTestRequest, sink TestEventSink) error {
			called = true
			require.NoError(t, sink.Emit(ctx, TestEvent{Type: "content", Text: "21"}))
			cancel()
			return ctx.Err()
		}), TestOptions{})
		evidence := service.RunPelican(ctx, 42, "selected", "candy", "1", "")
		cancel()
		require.Equal(t, !alreadyCancelled, called)
		require.Equal(t, "incomplete", evidence.DetailCode)
		require.Empty(t, evidence.Answer)
	}
}

func TestRunPelicanUsesRoundQuestionAndItsRubric(t *testing.T) {
	service := NewTestService(pelicanTestTarget(func(context.Context, PreparedTestRequest, TestEventSink) error { return nil }), TestOptions{})
	for round, description := range map[string][]string{"1": {"鹈鹕", "喉囊", "路标"}, "2": {"企鹅", "短喙", "冰块"}, "4": {"兔子", "长耳", "草丛"}, "7": {"水獭", "长尾", "石块"}} {
		evidence := service.RunPelican(t.Context(), 1, "model", "pelican", round, "")
		require.Contains(t, evidence.Prompt, description[0])
		require.Contains(t, evidence.Prompt, "CSS")
		require.Contains(t, evidence.Prompt, "SMIL")
		require.Len(t, evidence.Criteria, 4)
		require.Contains(t, evidence.Criteria[0], description[1])
		require.Contains(t, evidence.Criteria[3], description[2])
		evidence.Criteria[0] = "已编辑"
		next := service.RunPelican(t.Context(), 2, "another", "pelican", round, "")
		require.Equal(t, evidence.Prompt, next.Prompt)
		require.Contains(t, next.Criteria[0], description[1])
	}
}

func TestRunPelicanCandyFinalAnswer(t *testing.T) {
	for _, answer := range []string{"21", "21个", "21 颗糖果。", "答案是 21 个糖果。", "最终答案: **21**", "至少需要21个", "最少需要取出21个糖果。", "二十一颗糖果", "２１个", "The answer is 21.", "21 candies", "20", "121", "210", "21.5", "-21", "21或22", "不是21", "答案不是21个", "21个不够", "错误答案21", "2 1", "2*1", "21/22"} {
		t.Run(answer, func(t *testing.T) {
			service := NewTestService(pelicanTestTarget(func(ctx context.Context, _ PreparedTestRequest, sink TestEventSink) error {
				if err := sink.Emit(ctx, TestEvent{Type: "content", Text: answer}); err != nil {
					return err
				}
				return sink.Emit(ctx, TestEvent{Type: "test_complete", Success: true})
			}), TestOptions{})
			ev := service.RunPelican(t.Context(), 1, "model", "candy", "round", "")
			require.Contains(t, ev.Prompt, "圆形 7 9 8")
			require.Contains(t, ev.Prompt, "五角星形 7 6 4")
			switch answer {
			case "21", "21个", "21 颗糖果。", "答案是 21 个糖果。", "最终答案: **21**", "至少需要21个", "最少需要取出21个糖果。", "二十一颗糖果", "２１个", "The answer is 21.", "21 candies":
				require.Equal(t, "passed", ev.Status)
				require.Equal(t, answer, ev.Answer)
			default:
				require.NotEqual(t, "passed", ev.Status)
			}
		})
	}
}

func TestRunConfiguredPelicanGradesCustomQuestions(t *testing.T) {
	const drawing = `<svg xmlns="http://www.w3.org/2000/svg"><circle r="4"/></svg>`
	for _, tt := range []struct {
		name, kind, expected, output, status, detail string
	}{
		{"忽略首尾空白后精确匹配", "custom_text", "  AbC\n42  ", "\tAbC\n42\n", "passed", ""},
		{"保留大小写差异和完整回答", "custom_text", "AbC\n42", " abc\n42 \n", "suspected", "answer_mismatch"},
		{"保留回答内部换行", "custom_text", "AbC\n42", "AbC 42", "suspected", "answer_mismatch"},
		{"保留长文本回答", "custom_text", "42", strings.Repeat("作答\n", 400), "suspected", "answer_mismatch"},
		{"已完成的空文本", "custom_text", "42", " \n ", "inconclusive", "empty_output"},
		{"含空字符的文本无法存入结果", "custom_text", "42", "4\x002", "inconclusive", "incomplete"},
		{"文本超过采集上限", "custom_text", "42", strings.Repeat("x", PelicanAnswerLimit+1), "inconclusive", "output_limit"},
		{"管理员复核单独预览的图", "custom_svg", "", drawing, "pending_review", ""},
		{"截断的自定义图", "custom_svg", "", "<svg><path/>", "inconclusive", "invalid_artwork"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			const prompt = "按题面作答并保留换行。"
			service := NewTestService(pelicanTestTarget(func(ctx context.Context, request PreparedTestRequest, sink TestEventSink) error {
				require.Contains(t, request.Prompt, prompt)
				if tt.kind == "custom_text" {
					require.Equal(t, prompt, request.Prompt)
				} else {
					require.Contains(t, request.Prompt, "SVG")
					require.Contains(t, request.Prompt, "脚本")
				}
				if err := sink.Emit(ctx, TestEvent{Type: "content", Text: tt.output}); err != nil {
					return err
				}
				return sink.Emit(ctx, TestEvent{Type: "test_complete", Success: true})
			}), TestOptions{})
			ev := service.RunConfiguredPelican(t.Context(), 1, "model", tt.kind, "round", "", PelicanQuestionConfig{Prompt: prompt, ExpectedAnswer: tt.expected})
			require.Equal(t, tt.status, ev.Status)
			require.Equal(t, tt.detail, ev.DetailCode)
			require.NotEmpty(t, ev.Criteria)
			if tt.status == "inconclusive" {
				require.Empty(t, ev.Answer)
			} else {
				require.Equal(t, tt.output, ev.Answer)
			}
			require.Empty(t, ev.ReasoningEffort)
		})
	}
}

func TestRunConfiguredPelicanUsesEditedBuiltInQuestions(t *testing.T) {
	const artwork = `<svg xmlns="http://www.w3.org/2000/svg"><circle r="3"/></svg>`
	const editedDrawing = "用 SVG 画出一个蓝色圆圈。"
	const editedCandy = "苹果味与桃子味糖果共有几颗？"
	for _, tt := range []struct {
		name, kind, prompt, reference, output, status, detail string
	}{
		{"更换绘图题面", "pelican", editedDrawing, "", artwork, "pending_review", ""},
		{"编辑绘图题后仍检查成品", "pelican", editedDrawing, "", `<svg><image href="https://fixture.invalid/image"/></svg>`, "inconclusive", "invalid_artwork"},
		{"保留默认糖果答案", "candy", editedCandy, "", "二十一颗糖果", "passed", ""},
		{"显式默认仍兼容糖果单位", "candy", editedCandy, " 21 ", "最少需要取出21个糖果。", "passed", ""},
		{"标准答案可编辑为单个整数", "candy", editedCandy, "23", " 23\n", "passed", ""},
		{"非默认答案只接受原文", "candy", editedCandy, "23", "答案是 23 颗糖果。", "suspected", "answer_mismatch"},
		{"只覆盖答案也能使用原题", "candy", "", "Blue\n42", "\tBlue\n42\n", "passed", ""},
		{"更换答案后按完整输出比较", "candy", editedCandy, "红色", strings.Repeat("x", PelicanAnswerLimit), "suspected", "answer_mismatch"},
		{"超过采集上限", "candy", editedCandy, "红色", strings.Repeat("x", PelicanAnswerLimit+1), "inconclusive", "output_limit"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var sent string
			service := NewTestService(pelicanTestTarget(func(ctx context.Context, req PreparedTestRequest, sink TestEventSink) error {
				sent = req.Prompt
				if err := sink.Emit(ctx, TestEvent{Type: "content", Text: tt.output}); err != nil {
					return err
				}
				return sink.Emit(ctx, TestEvent{Type: "test_complete", Success: true})
			}), TestOptions{})
			ev := service.RunConfiguredPelican(t.Context(), 1, "model", tt.kind, "2", "", PelicanQuestionConfig{Prompt: tt.prompt, ExpectedAnswer: tt.reference})
			if tt.prompt != "" {
				require.Equal(t, tt.prompt, sent)
			} else {
				require.Equal(t, candyPrompt, sent)
			}
			require.Equal(t, sent, ev.Prompt)
			require.Equal(t, tt.status, ev.Status)
			require.Equal(t, tt.detail, ev.DetailCode)
			require.NotEmpty(t, ev.Criteria)
			if tt.kind == "pelican" {
				require.NotContains(t, strings.Join(ev.Criteria, "\n"), "短喙")
			} else if tt.reference != "" {
				require.Contains(t, strings.Join(ev.Criteria, "\n"), strings.TrimSpace(tt.reference))
			}
			if ev.Status == "inconclusive" {
				require.Empty(t, ev.Answer)
			} else {
				require.Equal(t, tt.output, ev.Answer)
			}
		})
	}
}
