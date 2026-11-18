package openai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	wire "github.com/TokenFlux/TokenRouter/internal/protocol/openai"
)

// compactionBrokenReader 模拟上游在事件之间断开。
type compactionBrokenReader struct{}

// compactionTrackedBody 记录外层取消读取时的关闭操作。
type compactionTrackedBody struct {
	io.Reader
	closed bool
}

func (compactionBrokenReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func (b *compactionTrackedBody) Close() error {
	b.closed = true
	return nil
}

func TestNormalizeNativeCompactionResponsePreservesOpaqueItem(t *testing.T) {
	item := `{"id":"cmp_opaque","type":"compaction","encrypted_content":"cipher\\nfixture","future":{"number":9007199254740993,"values":[1,2,3]}}`
	for _, source := range []string{"terminal", "done", "added"} {
		t.Run(source, func(t *testing.T) {
			output := "[" + item + "]"
			body := ""
			if source != "terminal" {
				body = fmt.Sprintf("event: response.output_item.%s\ndata: {\"output_index\":0,\"item\":%s}\n\n", source, item)
				output = "[]"
			}
			body += `event: response.completed` + "\n" + `data: {"type":"response.completed","response":{"id":"resp_test","status":"completed","output":` + output + `,"usage":{"output_tokens":123}}}` + "\n\n"
			var observations []NativeCompactionDiagnostics
			resp := normalizeNativeCompactionResponse(context.Background(), &http.Response{Body: io.NopCloser(strings.NewReader(body))}, StreamOptions{
				NativeCompactionV2: true,
				NativeCompactionResult: func(value NativeCompactionDiagnostics) {
					observations = append(observations, value)
				},
			})
			out, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Len(t, observations, 1)
			require.Equal(t, source, observations[0].Source)
			require.Empty(t, observations[0].Reason)
			require.Contains(t, string(out), "event: response.created")
			var received []string
			wire.ForEachSSEDataPayload(string(out), func(payload []byte) {
				if gjson.GetBytes(payload, "type").String() == "response.output_item.done" {
					received = append(received, gjson.GetBytes(payload, "item").Raw)
				}
			})
			require.Len(t, received, 1)
			require.Equal(t, "9007199254740993", gjson.Get(received[0], "future.number").Raw)
			require.Equal(t, gjson.Get(item, "encrypted_content").Str, gjson.Get(received[0], "encrypted_content").Str)
		})
	}
}

func TestNormalizeNativeCompactionResponseRetainsOtherTerminalItems(t *testing.T) {
	body := `data: {"type":"response.completed","response":{"id":"resp_test","status":"completed","output":[{"type":"message","role":"user","content":[]},{"type":"compaction","encrypted_content":"cipher"},{"type":"message","role":"user","content":[]}]}}` + "\n\n"
	resp := normalizeNativeCompactionResponse(context.Background(), &http.Response{Body: io.NopCloser(strings.NewReader(body))}, StreamOptions{NativeCompactionV2: true})
	out, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var types []string
	wire.ForEachSSEDataPayload(string(out), func(payload []byte) {
		if gjson.GetBytes(payload, "type").String() == "response.output_item.done" {
			types = append(types, gjson.GetBytes(payload, "item.type").String())
		}
	})
	require.Equal(t, []string{"message", "compaction", "message"}, types)
}

func TestNormalizeNativeCompactionResponseMatchesTerminalWithoutID(t *testing.T) {
	body := `data: {"type":"response.output_item.done","output_index":0,"item":{"id":"cmp_a","type":"compaction","encrypted_content":"cipher"}}` + "\n\n" +
		`data: {"type":"response.completed","response":{"id":"resp_test","status":"completed","output":[{"encrypted_content":"cipher","type":"compaction"}]}}` + "\n\n"
	resp := normalizeNativeCompactionResponse(context.Background(), &http.Response{Body: io.NopCloser(strings.NewReader(body))}, StreamOptions{NativeCompactionV2: true})
	out, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(out), "response.completed")
	require.NotContains(t, string(out), "response.failed")
}

func TestNormalizeNativeCompactionResponseRejectsConflictingItems(t *testing.T) {
	body := `data: {"type":"response.output_item.done","output_index":0,"item":{"id":"cmp_a","type":"compaction","encrypted_content":"secret-one"}}` + "\n\n" +
		`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"cmp_b","type":"compaction","encrypted_content":"secret-two"}}` + "\n\n"
	resp := normalizeNativeCompactionResponse(context.Background(), &http.Response{Body: io.NopCloser(strings.NewReader(body))}, StreamOptions{NativeCompactionV2: true})
	out, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(out), "conflicting_item_index")
	require.NotContains(t, string(out), "secret-one")
	require.NotContains(t, string(out), "secret-two")
	require.NotContains(t, string(out), "response.completed")
}

func TestNormalizeNativeCompactionResponseCancellationAndReadFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	source := &compactionTrackedBody{Reader: strings.NewReader("")}
	resp := normalizeNativeCompactionResponse(ctx, &http.Response{Body: source}, StreamOptions{NativeCompactionV2: true})
	_, err := io.ReadAll(resp.Body)
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, resp.Body.Close())
	require.True(t, source.closed)

	progress := `data: {"type":"response.in_progress","response":{"id":"resp_test","usage":{"input_tokens":23}}}` + "\n\n"
	resp = normalizeNativeCompactionResponse(context.Background(), &http.Response{Body: io.NopCloser(io.MultiReader(strings.NewReader(progress), compactionBrokenReader{}))}, StreamOptions{NativeCompactionV2: true})
	out, err := io.ReadAll(resp.Body)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.Contains(t, string(out), `"input_tokens":23`)
	require.NotContains(t, string(out), "response.completed")
}

func TestNormalizeNativeCompactionResponseBoundedBuffer(t *testing.T) {
	item := `{"id":"cmp_large","type":"compaction","encrypted_content":"` + strings.Repeat("x", 800) + `"}`
	body := `data: {"type":"response.output_item.added","output_index":0,"item":` + item + `}` + "\n\n" +
		`data: {"type":"response.output_item.done","output_index":0,"item":` + item + `}` + "\n\n"
	resp := normalizeNativeCompactionResponse(context.Background(), &http.Response{Body: io.NopCloser(strings.NewReader(body))}, StreamOptions{NativeCompactionV2: true, MaxLineSize: 1024})
	out, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(out), "buffer_limit")
	require.NotContains(t, string(out), strings.Repeat("x", 800))
}

func TestNormalizeNativeCompactionResponsePreservesUpstreamFailure(t *testing.T) {
	for _, body := range []string{
		`data: {"type":"response.failed","response":{"id":"resp_test","status":"failed","error":{"code":"token_revoked","message":"Token revoked","status_code":401}}}` + "\n\n",
		`data: {"type":"error","error":{"code":"token_revoked","message":"Token revoked"}}` + "\n\n",
	} {
		resp := normalizeNativeCompactionResponse(context.Background(), &http.Response{Body: io.NopCloser(strings.NewReader(body))}, StreamOptions{NativeCompactionV2: true})
		out, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Contains(t, string(out), "token_revoked")
		require.NotContains(t, string(out), "response.completed")
	}
}

func TestNormalizeNativeCompactionResponseOrdinaryRequest(t *testing.T) {
	body := `data: {"type":"response.completed","response":{"output":[],"usage":{"input_tokens":23}}}` + "\n\n"
	original := &http.Response{Body: io.NopCloser(strings.NewReader(body))}
	resp := normalizeNativeCompactionResponse(context.Background(), original, StreamOptions{})
	require.Same(t, original, resp)
	out, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, body, string(out))
}
