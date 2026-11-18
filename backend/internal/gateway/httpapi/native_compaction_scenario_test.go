package httpapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/TokenFlux/TokenRouter/internal/gateway/forward"
	gatewayprovider "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	wire "github.com/TokenFlux/TokenRouter/internal/protocol/openai"
	providercore "github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
	"github.com/TokenFlux/TokenRouter/internal/upstream"
	"github.com/TokenFlux/TokenRouter/internal/upstream/openai"
)

// TestNativeCompactionStream 覆盖 openai_response_output.go 的响应处理、
// openai_response_passthrough_options.go 的透传装配和 upstream/openai 的流式读取。
func TestNativeCompactionStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	item := `{"id":"cmp_fixture","type":"compaction","encrypted_content":"opaque-fixture"}`
	secondItem := `{"id":"cmp_fixture_2","type":"compaction","encrypted_content":"opaque-fixture-2"}`
	event := func(kind, data string) string { return "event: " + kind + "\ndata: " + data + "\n\n" }
	created := event("response.created", `{"type":"response.created","response":{"id":"resp_fixture","status":"in_progress","output":[]}}`)
	done := func(index int, item string) string {
		return event("response.output_item.done", fmt.Sprintf(`{"type":"response.output_item.done","output_index":%d,"item":%s}`, index, item))
	}
	completed := func(output string) string {
		return event("response.completed", `{"type":"response.completed","response":{"id":"resp_fixture","status":"completed","model":"gpt-6-sol","output":`+output+`,"usage":{"input_tokens":239015,"output_tokens":4720}}}`)
	}
	cases := []struct {
		name, body string
		invalid    bool
	}{
		{"valid_done_and_terminal", created + done(0, item) + completed("["+item+"]"), false},
		{"valid_done_empty_terminal", created + done(0, item) + completed("[]"), false},
		{"terminal_only_compaction", created + completed("["+item+"]"), false},
		{"added_only_compaction", created + event("response.output_item.added", `{"type":"response.output_item.added","output_index":0,"item":`+item+`}`) + completed("[]"), false},
		{"empty_output_with_usage", created + completed("[]"), true},
		{"missing_cipher", created + completed(`[{"id":"cmp_empty","type":"compaction"}]`), true},
		{"blank_cipher", created + completed(`[{"type":"compaction","encrypted_content":"  "}]`), true},
		{"incomplete_item", created + completed(`[{"type":"compaction","status":"in_progress","encrypted_content":"partial"}]`), true},
		{"incomplete_terminal", created + event("response.incomplete", `{"type":"response.incomplete","response":{"id":"resp_fixture","status":"incomplete","output":[],"usage":{"output_tokens":4720}}}`), true},
		{"missing_terminal", created + done(0, item), true},
		{"done_marker_without_terminal", created + done(0, item) + "data: [DONE]\n\n", true},
		{"duplicate_done_event", created + done(0, item) + done(0, item) + completed("[]"), false},
		{"summary_alias", created + completed(`[{"id":"cmp_summary","type":"compaction_summary","encrypted_content":"opaque-fixture"}]`), false},
		{"two_compaction_items", created + done(0, item) + done(1, secondItem) + completed("["+item+","+secondItem+"]"), true},
	}
	for _, passthrough := range []bool{false, true} {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("passthrough_%t/%s", passthrough, tc.name), func(t *testing.T) {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/responses", strings.NewReader(`{"model":"gpt-6-sol","stream":true,"input":[{"type":"compaction_trigger"}]}`))
				MarkOpenAINativeCompactionV2(c)
				svc := newResponseOutputForTest(OpenAIResponseOptions{})
				svc.Corrector = openai.NewCodexToolCorrector()
				provider := &gatewayprovider.ExecutionProvider{Record: providercore.Record{ID: 6945, Platform: capability.PlatformOpenAI, Type: capability.ProviderTypeOAuth}}
				resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(tc.body))}
				var result *openai.StreamingResult
				var err error
				if passthrough {
					result, err = openai.ReadPassthroughStreaming(context.Background(), resp, upstream.NewOutputContext(ResponseSink{Writer: c.Writer}), svc.PassthroughOptions(context.Background(), c, provider), time.Now(), "gpt-6-sol", "gpt-6-sol")
				} else {
					result, err = svc.ReadStreamObservation(context.Background(), resp, c, provider, time.Now(), "gpt-6-sol", "gpt-6-sol", "")
				}
				doneItems, compactions, terminalItems, terminalCompactions := 0, 0, 0, 0
				lastSequence := int64(-1)
				wire.ForEachSSEDataPayload(rec.Body.String(), func(payload []byte) {
					sequence := gjson.GetBytes(payload, "sequence_number").Int()
					require.Greater(t, sequence, lastSequence)
					lastSequence = sequence
					switch gjson.GetBytes(payload, "type").String() {
					case "response.output_item.done":
						doneItems++
						if gjson.GetBytes(payload, "item.type").String() == "compaction" {
							compactions++
						}
					case "response.completed":
						out := gjson.GetBytes(payload, "response.output").Array()
						terminalItems = len(out)
						for _, it := range out {
							if it.Get("type").String() == "compaction" {
								terminalCompactions++
							}
						}
					}
				})
				outputTokens := 0
				if result != nil && result.Usage != nil {
					outputTokens = result.Usage.OutputTokens
				}
				t.Logf("RESULT error=%v http=%d done_items=%d compaction_done=%d terminal_items=%d terminal_compactions=%d output_tokens=%d", err, rec.Code, doneItems, compactions, terminalItems, terminalCompactions, outputTokens)
				if tc.invalid {
					require.Error(t, err)
					var failover *forward.UpstreamFailoverError
					require.ErrorAs(t, err, &failover)
					require.False(t, c.Writer.Written(), "失败结果应在下发输出前进入故障转移")
					require.Empty(t, rec.Body.String())
				} else if err != nil || compactions != 1 {
					t.Errorf("expected one completed compaction item, got %d (error=%v)", compactions, err)
				}
				if strings.Contains(tc.body, `"output_tokens":4720`) {
					require.Equal(t, 4720, outputTokens)
				}
			})
		}
	}
}
