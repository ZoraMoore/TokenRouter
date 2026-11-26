package antigravity

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/TokenFlux/TokenRouter/internal/protocol/bridge"
	"github.com/TokenFlux/TokenRouter/internal/upstream"
)

type cacheClientSink struct {
	bytes.Buffer // captured client output bytes
}

func (s *cacheClientSink) Begin(upstream.OutputHead) error { return nil }

func (s *cacheClientSink) Emit(event upstream.OutputEvent) error {
	_, err := s.Write(event.Data)
	return err
}

func TestAntigravityCacheTailAcrossClientProtocols(t *testing.T) {
	for _, quota := range []struct{ cache, output int }{{60, 10}, {100, 0}} {
		body := fmt.Sprintf("data: "+`{"responseId":"outer_cache","response":{"candidates":[{"content":{"parts":[{"text":"OK"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":100}}}`+"\n\n"+
			"data: "+`{"response":{"usageMetadata":{"cachedContentTokenCount":%d,"candidatesTokenCount":%d}}}`+"\n\ndata: [DONE]\n\n", quota.cache, quota.output)

		for _, protocol := range []string{"messages", "chat", "responses"} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%t/cache=%d/output=%d", protocol, stream, quota.cache, quota.output), func(t *testing.T) {
					response := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
					sink := &cacheClientSink{}
					output := upstream.NewOutputContext(sink)
					adapter := ResponseAdapter{Options: ResponseOptions{
						ReverseTools:       func(data []byte) []byte { return data },
						Failover:           func([]byte) error { return errors.New("unexpected failover") },
						MapCollectionError: func(err error) error { return err },
						IsFailover:         func(error) bool { return false }, MarkCommitted: func() {},
					}}
					var result *StreamResult
					var err error
					switch protocol {
					case "messages":
						if stream {
							result, err = adapter.HandleClaudeStreamingResponse(output, response, time.Now(), "model")
						} else {
							result, err = adapter.HandleClaudeStreamToNonStreaming(output, response, time.Now(), "model")
						}
					case "chat":
						if stream {
							result, err = adapter.HandleChatCompletionsStreamingFromAntigravity(output, response, time.Now(), "model", true)
						} else {
							result, err = adapter.HandleChatCompletionsNonStreamingFromAntigravity(output, response, time.Now(), "model")
						}
					case "responses":
						if stream {
							result, err = adapter.HandleResponsesStreamingFromAntigravity(output, response, time.Now(), "model", bridge.ResponsesClientToolMapping{})
						} else {
							result, err = adapter.HandleResponsesNonStreamingFromAntigravity(output, response, time.Now(), "model", bridge.ResponsesClientToolMapping{})
						}
					}
					require.NoError(t, err)
					require.Equal(t, 100-quota.cache, result.Usage.InputTokens)
					require.Equal(t, quota.cache, result.Usage.CacheReadInputTokens)
					require.Equal(t, quota.output, result.Usage.OutputTokens)
					visible := gjson.Get(sink.String(), "usage")
					if stream {
						for line := range strings.SplitSeq(sink.String(), "\n") {
							if !strings.HasPrefix(line, "data: ") {
								continue
							}
							payload := strings.TrimPrefix(line, "data: ")
							if u := gjson.Get(payload, "usage"); u.IsObject() {
								visible = u
							}
							if u := gjson.Get(payload, "response.usage"); u.IsObject() {
								visible = u
							}
						}
					}
					require.True(t, visible.IsObject(), sink.String())
					switch protocol {
					case "messages":
						require.Contains(t, sink.String(), `"id":"outer_cache"`)
						require.Equal(t, int64(100-quota.cache), visible.Get("input_tokens").Int())
						require.Equal(t, int64(quota.cache), visible.Get("cache_read_input_tokens").Int())
						if stream {
							require.Equal(t, 1, strings.Count(sink.String(), `"type":"message_stop"`))
						}
					case "chat":
						require.Equal(t, int64(100), visible.Get("prompt_tokens").Int())
						require.Equal(t, int64(quota.cache), visible.Get("prompt_tokens_details.cached_tokens").Int())
					case "responses":
						require.Equal(t, int64(100), visible.Get("input_tokens").Int())
						require.Equal(t, int64(quota.cache), visible.Get("input_tokens_details.cached_tokens").Int())
					}
				})
			}
		}
	}
}

func TestAntigravityCacheTailFailureCannotEmitSuccess(t *testing.T) {
	body := "data: " + `{"response":{"candidates":[{"content":{"parts":[{"text":"OK"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":100,"cachedContentTokenCount":60}}}` + "\n\n" +
		"data: " + `{"error":{"code":500,"message":"fixture failure"}}` + "\n\n"
	sink := &cacheClientSink{}
	adapter := ResponseAdapter{}
	response := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
	result, err := adapter.HandleClaudeStreamingResponse(upstream.NewOutputContext(sink), response, time.Now(), "model")
	require.Error(t, err)
	require.Equal(t, 60, result.Usage.CacheReadInputTokens)
	require.Contains(t, sink.String(), "event: error")
	require.NotContains(t, sink.String(), `"type":"message_stop"`)
}

func TestAntigravityChatUsageOptOutRetainsAccounting(t *testing.T) {
	body := "data: " + `{"response":{"candidates":[{"content":{"parts":[{"text":"OK"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":100,"cachedContentTokenCount":60,"candidatesTokenCount":10}}}` + "\n\n"
	sink := &cacheClientSink{}
	adapter := ResponseAdapter{Options: ResponseOptions{ReverseTools: func(data []byte) []byte { return data }}}
	response := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
	result, err := adapter.HandleChatCompletionsStreamingFromAntigravity(upstream.NewOutputContext(sink), response, time.Now(), "model", false)
	require.NoError(t, err)
	require.Equal(t, 60, result.Usage.CacheReadInputTokens)
	require.NotContains(t, sink.String(), `"usage"`)
}
