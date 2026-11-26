package upstream

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tidwall/gjson"
	"go.uber.org/zap"

	"github.com/TokenFlux/TokenRouter/internal/infra/telemetry/logging"
	"github.com/TokenFlux/TokenRouter/internal/pkg/logredact"
	geminiwire "github.com/TokenFlux/TokenRouter/internal/protocol/gemini"
)

const (
	geminiStreamTailLimit  = 5 * time.Second
	geminiStreamReadBuffer = 32 * 1024
)

// GeminiStreamOptions 保存 Gemini 事件的大小、空闲超时和候选数量。
type GeminiStreamOptions struct {
	// Keepalive 在同一个消费协程中发送下游保活，避免与输出并发写入。
	Keepalive         func()
	KeepaliveInterval time.Duration
	IdleTimeout       time.Duration
	MaxFrameBytes     int
	Candidates        int
	Unwrap            func([]byte) ([]byte, error)
}

// GeminiStreamEvent 表示已完成解析的 Gemini 事件。
type GeminiStreamEvent struct {
	Data     []byte
	Terminal bool
}

// GeminiStreamResult 保存生成完成状态和跨帧合并的用量。
type GeminiStreamResult struct {
	Completed      bool
	UsageJSON      []byte
	TailIncomplete bool
}

// GeminiStreamError 表示上游状态码或流式协议错误。
type GeminiStreamError struct {
	Status  int
	Kind    string
	Message string
	Body    []byte
	Cause   error
}

// geminiStreamBody 跟踪实际读取活动并保证响应体只关闭一次。
type geminiStreamBody struct {
	io.ReadCloser
	last atomic.Int64
	once sync.Once
	err  error
}

// geminiStreamFrame 在读取协程与消费者之间传递完整事件。
type geminiStreamFrame struct {
	event string
	data  []byte
	err   error
}

// geminiStreamState 保存候选完成标记和本次调用的用量字段。
type geminiStreamState struct {
	options  GeminiStreamOptions
	finished map[int]struct{}
	content  bool
	usage    map[string]json.RawMessage
	result   GeminiStreamResult
}

// Error 返回流式错误的类别和说明。
func (e *GeminiStreamError) Error() string {
	return fmt.Sprintf("gemini %s: %s", e.Kind, e.Message)
}

// Unwrap 返回底层读取或取消错误。
func (e *GeminiStreamError) Unwrap() error { return e.Cause }

// GeminiCandidateCount 从最终请求体读取候选数量。
func GeminiCandidateCount(body []byte) int {
	for _, prefix := range []string{"", "request."} {
		for _, config := range []string{"generationConfig.", "generation_config."} {
			for _, key := range []string{"candidateCount", "candidate_count"} {
				value := gjson.GetBytes(body, prefix+config+key)
				if value.Type == gjson.Number && value.Int() > 0 {
					return int(value.Int())
				}
			}
		}
	}
	return 1
}

// ReadGeminiStream 按完整 SSE 事件读取 Gemini 响应，并在候选完成后收集最多五秒的尾部用量。
// @project-doc docs/interfaces/gemini_upstream.md#gemini_stream_lifecycle
func ReadGeminiStream(ctx context.Context, resp *http.Response, options GeminiStreamOptions, consume func(GeminiStreamEvent) error) (GeminiStreamResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if options.MaxFrameBytes <= 0 {
		options.MaxFrameBytes = DefaultSSELineLimit
	}
	if options.Candidates <= 0 {
		options.Candidates = 1
	}
	state := geminiStreamState{options: options, finished: make(map[int]struct{})}
	if resp == nil || resp.Body == nil {
		return state.result, newGeminiStreamError("stream_empty", "Upstream returned an empty response", nil)
	}
	body := &geminiStreamBody{ReadCloser: resp.Body}
	body.last.Store(time.Now().UnixNano())
	resp.Body = body
	if err := ctx.Err(); err != nil {
		_ = body.Close()
		return state.result, err
	}
	frames := make(chan geminiStreamFrame, 1)
	stop := make(chan struct{})
	joined := make(chan struct{})
	go func() {
		defer close(joined)
		readGeminiFrames(body, options.MaxFrameBytes, frames, stop)
	}()
	defer func() {
		close(stop)
		_ = body.Close()
		<-joined
	}()
	var idle *time.Timer
	var idleC <-chan time.Time
	if options.IdleTimeout > 0 {
		idle = time.NewTimer(options.IdleTimeout)
		idleC = idle.C
		defer idle.Stop()
	}
	var tail *time.Timer
	var keepalive *time.Ticker
	var keepaliveC <-chan time.Time
	if options.Keepalive != nil && options.KeepaliveInterval > 0 {
		keepalive = time.NewTicker(options.KeepaliveInterval)
		keepaliveC = keepalive.C
		defer keepalive.Stop()
	}
	var tailC <-chan time.Time
	defer func() {
		if tail != nil {
			tail.Stop()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return state.result, ctx.Err()
		case <-keepaliveC:
			if !state.result.Completed {
				options.Keepalive()
			}
		case <-tailC:
			return state.incompleteTail(ctx, errors.New("usage tail exceeded five seconds")), nil
		case <-idleC:
			remaining := options.IdleTimeout - time.Since(time.Unix(0, body.last.Load()))
			if remaining > 0 {
				idle.Reset(remaining)
				continue
			}
			if state.result.Completed {
				return state.incompleteTail(ctx, errors.New("usage tail became idle")), nil
			}
			return state.result, newGeminiStreamError("stream_timeout", "Upstream stream became idle", nil)
		case frame := <-frames:
			if frame.err != nil {
				if ctx.Err() != nil {
					return state.result, ctx.Err()
				}
				if errors.Is(frame.err, context.Canceled) {
					return state.result, frame.err
				}
				if errors.Is(frame.err, io.EOF) {
					if state.result.Completed {
						return state.result, nil
					}
					if !state.content {
						return state.result, newGeminiStreamError("stream_empty", "Upstream stream contained no completed response", nil)
					}
					return state.result, newGeminiStreamError("stream_incomplete", "Upstream stream ended before completion", nil)
				}
				if _, ok := errors.AsType[*GeminiStreamError](frame.err); ok {
					return state.result, frame.err
				}
				if state.result.Completed {
					return state.incompleteTail(ctx, frame.err), nil
				}
				return state.result, newGeminiStreamError("stream_read_error", "Failed to read upstream stream", frame.err)
			}
			wasCompleted := state.result.Completed
			event, done, err := state.accept(frame)
			if err != nil {
				return state.result, err
			}
			if len(event.Data) > 0 || event.Terminal {
				if err := consume(event); err != nil {
					return state.result, err
				}
			}
			if done {
				return state.result, nil
			}
			if !wasCompleted && state.result.Completed {
				tail = time.NewTimer(geminiStreamTailLimit)
				tailC = tail.C
			}
		}
	}
}

// GeminiStreamResult 保存生成完成状态和跨帧合并的用量。
// incompleteTail 记录尾部用量收集超时或读取中断。
func (s *geminiStreamState) incompleteTail(ctx context.Context, cause error) GeminiStreamResult {
	s.result.TailIncomplete = true
	logging.FromContext(ctx).Warn("gemini.stream_tail_incomplete", zap.String("error", logredact.SanitizeUpstreamQueries(cause.Error())))
	return s.result
}

// accept 校验完整事件、合并用量并推进候选完成状态。
func (s *geminiStreamState) accept(frame geminiStreamFrame) (GeminiStreamEvent, bool, error) {
	data := bytes.TrimSpace(frame.data)
	if bytes.Equal(data, []byte("[DONE]")) {
		if !s.result.Completed && !s.content {
			return GeminiStreamEvent{}, false, newGeminiStreamError("stream_empty", "Upstream ended without candidate content", nil)
		}
		event := GeminiStreamEvent{Terminal: !s.result.Completed}
		s.result.Completed = true
		return event, true, nil
	}
	business := gjson.GetBytes(data, "candidates").Exists() || gjson.GetBytes(data, "response").Exists() || gjson.GetBytes(data, "error").Exists() || gjson.GetBytes(data, "promptFeedback").Exists()
	if !business && frame.event != "error" && IsSSEHeartbeatEvent(frame.event, string(data)) {
		return GeminiStreamEvent{}, false, nil
	}
	if !json.Valid(data) {
		return GeminiStreamEvent{}, false, newGeminiStreamError("stream_invalid_json", "Upstream stream contained invalid JSON", nil)
	}
	outerError := gjson.GetBytes(data, "error")
	if s.options.Unwrap != nil && (!outerError.Exists() || outerError.Type == gjson.Null) {
		var err error
		data, err = s.options.Unwrap(data)
		if err != nil || !json.Valid(data) {
			return GeminiStreamEvent{}, false, newGeminiStreamError("stream_invalid_json", "Invalid wrapped upstream response", err)
		}
	}
	root := gjson.ParseBytes(data)
	if !root.IsObject() {
		return GeminiStreamEvent{}, false, newGeminiStreamError("stream_invalid_json", "Upstream stream payload must be an object", nil)
	}
	fields := root.Map()
	candidates := root.Get("candidates")
	emptyCandidates := candidates.Type == gjson.Null || (candidates.IsArray() && len(candidates.Array()) == 0)
	if len(fields) == 0 || (len(fields) == 1 && candidates.Exists() && emptyCandidates) {
		return GeminiStreamEvent{}, false, nil
	}
	if candidates.Exists() && candidates.Type != gjson.Null && !candidates.IsArray() {
		return GeminiStreamEvent{}, false, newGeminiStreamError("stream_invalid_json", "Invalid upstream candidate list", nil)
	}
	errorValue := root.Get("error")
	if frame.event == "error" || (errorValue.Exists() && errorValue.Type != gjson.Null) {
		status := int(errorValue.Get("code").Int())
		if status < 400 || status > 599 {
			status = http.StatusBadGateway
		}
		message := logredact.SanitizeUpstreamQueries(ExtractErrorMessage(data))
		if message == "" {
			message = "Upstream stream failed"
		}
		return GeminiStreamEvent{}, false, &GeminiStreamError{Status: status, Kind: "upstream_error", Message: message, Body: bytes.Clone(data)}
	}
	observed := geminiwire.ObservePayload(data)
	if usage := root.Get("usageMetadata"); usage.IsObject() {
		if s.usage == nil {
			s.usage = make(map[string]json.RawMessage)
		}
		usage.ForEach(func(key, value gjson.Result) bool {
			if value.Type != gjson.Null {
				s.usage[key.String()] = json.RawMessage(value.Raw)
			}
			return true
		})
		s.result.UsageJSON, _ = json.Marshal(map[string]any{"usageMetadata": s.usage})
	}
	s.content = s.content || observed.Semantic
	candidates.ForEach(func(index, candidate gjson.Result) bool {
		value := candidate.Get("finishReason")
		finish := strings.TrimSpace(value.String())
		if value.Type != gjson.String || finish == "" || finish == "FINISH_REASON_UNSPECIFIED" {
			return true
		}
		candidateIndex := int(index.Int())
		if value := candidate.Get("index"); value.Type == gjson.Number {
			candidateIndex = int(value.Int())
		}
		if candidateIndex >= 0 {
			s.finished[candidateIndex] = struct{}{}
		}
		return true
	})
	blocked := root.Get("promptFeedback.blockReason")
	completed := len(s.finished) >= s.options.Candidates || (blocked.Type == gjson.String && blocked.String() != "" && blocked.String() != "BLOCK_REASON_UNSPECIFIED")
	event := GeminiStreamEvent{Terminal: !s.result.Completed && completed}
	s.result.Completed = s.result.Completed || completed
	var compact bytes.Buffer
	if err := json.Compact(&compact, data); err != nil {
		return GeminiStreamEvent{}, false, newGeminiStreamError("stream_invalid_json", "Invalid upstream response", err)
	}
	event.Data = compact.Bytes()
	return event, false, nil
}

// newGeminiStreamError 构造本地流式读取错误。
func newGeminiStreamError(kind, message string, cause error) *GeminiStreamError {
	return &GeminiStreamError{Status: http.StatusBadGateway, Kind: kind, Message: message, Cause: cause}
}

// Read 记录读取活动的时间，供空闲超时检查使用。
func (b *geminiStreamBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.last.Store(time.Now().UnixNano())
	}
	return n, err
}

// Close 关闭响应体并保留第一次关闭的结果。
func (b *geminiStreamBody) Close() error {
	b.once.Do(func() { b.err = b.ReadCloser.Close() })
	return b.err
}

// readGeminiFrames 按空行分隔 SSE 事件，并合并多行 data 字段。
func readGeminiFrames(body io.Reader, limit int, frames chan<- geminiStreamFrame, stop <-chan struct{}) {
	reader := bufio.NewReaderSize(body, geminiStreamReadBuffer)
	var line, data []byte
	event := ""
	frameBytes := 0
	send := func(frame geminiStreamFrame) bool {
		select {
		case frames <- frame:
			return true
		case <-stop:
			return false
		}
	}
	emit := func() bool {
		if len(data) == 0 && event != "error" {
			return true
		}
		return send(geminiStreamFrame{event: event, data: bytes.Clone(data)})
	}
	for {
		fragment, err := reader.ReadSlice('\n')
		frameBytes += len(fragment)
		if frameBytes > limit {
			send(geminiStreamFrame{err: newGeminiStreamError("stream_frame_too_large", "Upstream SSE frame exceeds the configured limit", nil)})
			return
		}
		line = append(line, fragment...)
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if err != nil && !errors.Is(err, io.EOF) {
			send(geminiStreamFrame{err: err})
			return
		}
		value := bytes.TrimSuffix(bytes.TrimSuffix(line, []byte("\n")), []byte("\r"))
		if len(value) == 0 {
			if !emit() {
				return
			}
			data = data[:0]
			event = ""
			frameBytes = 0
		} else if value[0] != ':' {
			field, content, _ := bytes.Cut(value, []byte(":"))
			content = bytes.TrimPrefix(content, []byte(" "))
			switch string(field) {
			case "data":
				if len(data) > 0 {
					data = append(data, '\n')
				}
				data = append(data, content...)
			case "event":
				event = string(content)
			}
		}
		line = line[:0]
		if errors.Is(err, io.EOF) {
			if !emit() {
				return
			}
			send(geminiStreamFrame{err: io.EOF})
			return
		}
	}
}
