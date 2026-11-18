package provider

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPelicanQuestionDefaultsMatchTheQuestionsSent(t *testing.T) {
	defaults := PelicanQuestionDefaults()
	require.Len(t, defaults, 2)
	data, err := json.Marshal(defaults)
	require.NoError(t, err)
	var rows []map[string]string
	require.NoError(t, json.Unmarshal(data, &rows))
	for _, row := range rows {
		require.Len(t, row, 4)
		for _, key := range []string{"question_kind", "question_name", "question_prompt", "expected_answer"} {
			require.Contains(t, row, key)
		}
	}
	svg, candy := defaults[0], defaults[1]
	require.Equal(t, "pelican", svg.QuestionKind)
	require.Equal(t, "鹈鹕 SVG", svg.QuestionName)
	require.Empty(t, svg.ExpectedAnswer)
	for _, fragment := range []string{"鹈鹕骑自行车", "喉囊", "脚踏", "路标", "内联 SVG", "CSS", "SMIL"} {
		require.Contains(t, svg.QuestionPrompt, fragment)
	}
	for _, round := range []string{"1", "2", "4", "7"} {
		question := pelicanQuestion("pelican", round, PelicanQuestionConfig{Prompt: svg.QuestionPrompt})
		require.Equal(t, svg.QuestionPrompt, question.Prompt)
		require.Len(t, question.Criteria, 4)
		require.Contains(t, question.Criteria[0], "喉囊")
		require.Contains(t, question.Criteria[3], "路标")
	}
	require.Equal(t, "candy", candy.QuestionKind)
	require.Equal(t, "糖果题", candy.QuestionName)
	require.Equal(t, candyPrompt, candy.QuestionPrompt)
	require.Equal(t, "21", candy.ExpectedAnswer)

	defaults[0].QuestionPrompt = "已编辑"
	require.Equal(t, svg.QuestionPrompt, PelicanQuestionDefaults()[0].QuestionPrompt)
}

func TestPelicanQuestionRejectsInvalidOverrides(t *testing.T) {
	for _, tt := range []struct {
		kind, prompt, expected string
	}{
		{"pelican", strings.Repeat("x", 8193), ""},
		{"pelican", "绘\x00图", ""},
		{"pelican", "画图", "不适用于 SVG"},
		{"candy", "\xff", "21"},
		{"candy", "题目", strings.Repeat("x", 1025)},
	} {
		q := pelicanQuestion(tt.kind, "round", PelicanQuestionConfig{Prompt: tt.prompt, ExpectedAnswer: tt.expected})
		require.Empty(t, q.Prompt)
		require.Equal(t, "inconclusive", q.Status)
	}
}
