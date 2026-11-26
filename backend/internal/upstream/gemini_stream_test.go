package upstream

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

// TestReadGeminiStreamValidation 覆盖上游 Gemini 事件和用量的回归场景。
func TestReadGeminiStreamValidation(t *testing.T) {
	content := `{"candidates":[{"index":0,"content":{"parts":[{"text":"hello"}]}}]}`
	terminal := `{"candidates":[{"index":0,"finishReason":"STOP"}],"unknown":{"retained":true}}`
	for _, tc := range []struct {
		name     string
		body     string
		kind     string
		status   int
		payloads int
	}{
		{"ordinary", "data: " + content + "\n\ndata: " + terminal + "\n\n", "", 0, 2},
		{"crlf fields", ": note\r\nevent: message\r\nid: ignored\r\nretry: 10\r\ndata: " + terminal + "\r\n\r\n", "", 0, 1},
		{"multiline", "data: {\n data: ignored\ndata: \"candidates\":[{\"finishReason\":\"STOP\"}]}\n\n", "", 0, 1},
		{"final delimiter absent", "data: " + terminal, "", 0, 1},
		{"blocked", "data: {\"promptFeedback\":{\"blockReason\":\"SAFETY\"}}\n\n", "", 0, 1},
		{"done after content", "data: " + content + "\n\ndata: [DONE]\n\n", "", 0, 1},
		{"empty", "", "stream_empty", 502, 0},
		{"empty candidates", "data: {\"candidates\":[]}\n\ndata: " + terminal + "\n\n", "", 0, 1},
		{"null candidates only", "data: {\"candidates\":null}\n\n", "stream_empty", 502, 0},
		{"heartbeats only", ": keepalive\n\nevent: ping\ndata: {\"type\":\"ping\"}\n\n\n\n", "stream_empty", 502, 0},
		{"usage only done", "data: {\"usageMetadata\":{\"promptTokenCount\":1}}\n\ndata: [DONE]\n\n", "stream_empty", 502, 1},
		{"missing finish", "data: " + content + "\n\n", "stream_incomplete", 502, 1},
		{"invalid json", "data: {broken}\n\n", "stream_invalid_json", 502, 0},
		{"error", "data: {\"error\":{\"code\":429,\"message\":\"quota\"}}\n\n", "upstream_error", 429, 0},
		{"error after terminal", "data: " + terminal + "\n\ndata: {\"error\":{\"code\":503,\"message\":\"failed\"}}\n\n", "upstream_error", 503, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := &http.Response{Body: io.NopCloser(strings.NewReader(tc.body))}
			payloads, terminals := 0, 0
			result, err := ReadGeminiStream(context.Background(), resp, GeminiStreamOptions{}, func(event GeminiStreamEvent) error {
				if len(event.Data) > 0 {
					payloads++
				}
				if event.Terminal {
					terminals++
				}
				return nil
			})
			require.Equal(t, tc.payloads, payloads)
			if tc.kind != "" {
				var failure *GeminiStreamError
				require.ErrorAs(t, err, &failure)
				require.Equal(t, tc.kind, failure.Kind)
				require.Equal(t, tc.status, failure.Status)
				return
			}
			require.NoError(t, err)
			require.True(t, result.Completed)
			require.Equal(t, 1, terminals)
		})
	}
}

// TestReadGeminiStreamCandidates 覆盖上游 Gemini 事件和用量的回归场景。
func TestReadGeminiStreamCandidates(t *testing.T) {
	body := "data: {\"candidates\":[{\"index\":0,\"finishReason\":\"STOP\"}]}\n\n" +
		"data: {\"candidates\":[{\"index\":0,\"finishReason\":\"STOP\"}]}\n\n" +
		"data: {\"candidates\":[{\"index\":1,\"finishReason\":\"MAX_TOKENS\"}]}\n\n" +
		"data: {\"usageMetadata\":{\"totalTokenCount\":12}}\n\n"
	var terminals []bool
	result, err := ReadGeminiStream(context.Background(), &http.Response{Body: io.NopCloser(strings.NewReader(body))}, GeminiStreamOptions{Candidates: 2}, func(event GeminiStreamEvent) error {
		terminals = append(terminals, event.Terminal)
		return nil
	})
	require.NoError(t, err)
	require.True(t, result.Completed)
	require.Equal(t, []bool{false, false, true, false}, terminals)
}

// TestReadGeminiStreamFrameLimit 覆盖上游 Gemini 事件和用量的回归场景。
func TestReadGeminiStreamFrameLimit(t *testing.T) {
	_, err := ReadGeminiStream(context.Background(), &http.Response{Body: io.NopCloser(strings.NewReader(":" + strings.Repeat("x", 100) + "\n\n"))}, GeminiStreamOptions{MaxFrameBytes: 32}, func(GeminiStreamEvent) error { return nil })
	var failure *GeminiStreamError
	require.ErrorAs(t, err, &failure)
	require.Equal(t, "stream_frame_too_large", failure.Kind)
}

// TestReadGeminiStreamDeadlines 覆盖上游 Gemini 事件和用量的回归场景。
func TestReadGeminiStreamDeadlines(t *testing.T) {
	for _, mode := range []string{"idle", "tail", "cancel", "heartbeat renewal"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				reader, writer := io.Pipe()
				defer func() { _ = writer.Close() }()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				go func() {
					if mode == "cancel" {
						time.Sleep(time.Second)
						cancel()
						return
					}
					if mode == "heartbeat renewal" {
						for range 3 {
							time.Sleep(90 * time.Second)
							_, _ = io.WriteString(writer, ": alive\n\n")
						}
					}
					if mode != "idle" {
						_, _ = io.WriteString(writer, "data: {\"candidates\":[{\"finishReason\":\"STOP\"}]}\n\n")
					}
					if mode == "heartbeat renewal" {
						_ = writer.Close()
					}
				}()
				start := time.Now()
				result, err := ReadGeminiStream(ctx, &http.Response{Body: reader}, GeminiStreamOptions{IdleTimeout: 180 * time.Second}, func(GeminiStreamEvent) error { return nil })
				switch mode {
				case "idle":
					var failure *GeminiStreamError
					require.ErrorAs(t, err, &failure)
					require.Equal(t, "stream_timeout", failure.Kind)
					require.Equal(t, 180*time.Second, time.Since(start))
				case "cancel":
					require.ErrorIs(t, err, context.Canceled)
				case "tail":
					require.NoError(t, err)
					require.True(t, result.Completed)
					require.True(t, result.TailIncomplete)
					require.Equal(t, 5*time.Second, time.Since(start))
				case "heartbeat renewal":
					require.NoError(t, err)
					require.True(t, result.Completed)
					require.Equal(t, 270*time.Second, time.Since(start))
				}
			})
		})
	}
}

// TestReadGeminiStreamConsumerFailure 覆盖上游 Gemini 事件和用量的回归场景。
func TestReadGeminiStreamConsumerFailure(t *testing.T) {
	want := errors.New("client write failed")
	body := "data: {\"candidates\":[{\"finishReason\":\"STOP\"}]}\n\n"
	_, err := ReadGeminiStream(context.Background(), &http.Response{Body: io.NopCloser(strings.NewReader(body))}, GeminiStreamOptions{}, func(GeminiStreamEvent) error { return want })
	require.ErrorIs(t, err, want)
}

// TestGeminiDownstreamHeartbeatDoesNotRenewUpstreamIdle 检查下游保活不会掩盖上游超时。
func TestGeminiDownstreamHeartbeatDoesNotRenewUpstreamIdle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader, writer := io.Pipe()
		defer func() { _ = writer.Close() }()
		beats := 0
		started := time.Now()
		result, err := ReadGeminiStream(t.Context(), &http.Response{Body: reader}, GeminiStreamOptions{
			IdleTimeout: 3 * time.Second, KeepaliveInterval: time.Second, Keepalive: func() { beats++ },
		}, func(GeminiStreamEvent) error { return nil })
		var failure *GeminiStreamError
		require.ErrorAs(t, err, &failure)
		require.Equal(t, "stream_timeout", failure.Kind)
		require.False(t, result.Completed)
		require.GreaterOrEqual(t, beats, 2)
		require.Equal(t, 3*time.Second, time.Since(started))
	})
}
