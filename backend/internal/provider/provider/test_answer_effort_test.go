package provider

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSentAnswerEffortReadsProtocolFields(t *testing.T) {
	for _, tt := range []struct {
		protocol   answerProtocol
		body, want string
	}{
		{answerResponses, `{"model":"fixture-high"}`, "default"},
		{answerResponses, `{"reasoning":{"effort":"unknown"}}`, ""},
		{answerChat, `{"reasoning_effort":"xhigh"}`, "xhigh"},
		{answerClaude, `{"thinking":{"type":"enabled","budget_tokens":3000}}`, ""},
		{answerBedrock, `{"output_config":{"effort":"high"}}`, "high"},
		{answerGemini, `{"request":{"generationConfig":{"thinkingConfig":{"thinkingLevel":"LOW"}}}}`, "low"},
		{answerGemini, `{"generationConfig":{"thinkingConfig":{"thinkingBudget":0}}}`, "none"},
		{answerGemini, `{"generationConfig":{"thinkingConfig":{"thinkingBudget":4096}}}`, ""},
		{answerQoder, `{"parameters":{"reasoning_effort":"none"}}`, "none"},
		{answerQoder, `{"model_config":{"is_reasoning":true}}`, ""},
		{answerQoder, `{"model_config":{"is_reasoning":false}}`, "none"},
		{answerChat, "", ""},
	} {
		require.Equal(t, tt.want, sentAnswerEffort(tt.protocol, []byte(tt.body)), tt.body)
	}
}
