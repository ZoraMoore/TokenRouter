package openai

import (
	"testing"

	"github.com/stretchr/testify/require"

	protocolopenai "github.com/TokenFlux/TokenRouter/internal/protocol/openai"
)

func TestOpenAICompatTerminalResponseSynthesizesBareError(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"type":"error","code":"upstream_error","error":{"message":"provider failed"}}`)
	event := &protocolopenai.ResponsesStreamEvent{Type: "error", Code: "upstream_error"}
	response := CompatTerminalResponse(event, payload)
	require.NotNil(t, response)
	require.Equal(t, "failed", response.Status)
	require.Equal(t, "upstream_error", response.Error.Code)
	require.Equal(t, "provider failed", response.Error.Message)
}
