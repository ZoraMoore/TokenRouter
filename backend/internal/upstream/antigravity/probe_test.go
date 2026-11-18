package antigravity

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBuildAnswerRequestSharesPromptWithLowCostProbe(t *testing.T) {
	for _, tt := range []struct {
		model string
		probe func(string, string, ...string) ([]byte, error)
	}{
		{model: "gemini-3-pro", probe: BuildGeminiTestRequest},
		{model: "claude-sonnet-4-5", probe: BuildClaudeTestRequest},
	} {
		t.Run(tt.model, func(t *testing.T) {
			const prompt = "生成一个内联 SVG 动画。"
			probe, err := tt.probe("project", tt.model, prompt)
			require.NoError(t, err)
			answer, err := BuildAnswerRequest("project", tt.model, prompt, 32000)
			require.NoError(t, err)
			for _, payload := range [][]byte{probe, answer} {
				require.Equal(t, tt.model, gjson.GetBytes(payload, "model").String())
				require.Equal(t, prompt, gjson.GetBytes(payload, "request.contents.0.parts.0.text").String())
			}
			require.EqualValues(t, 1, gjson.GetBytes(probe, "request.generationConfig.maxOutputTokens").Int())
			require.EqualValues(t, 32000, gjson.GetBytes(answer, "request.generationConfig.maxOutputTokens").Int())
		})
	}
}
