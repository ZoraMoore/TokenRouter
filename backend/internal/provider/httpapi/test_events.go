package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/TokenFlux/TokenRouter/internal/provider"
)

// TestStreamWriter 将业务测试事件写入 HTTP 响应。
type TestStreamWriter interface {
	http.ResponseWriter
	http.Flusher
}
type TestEventSink struct{ writer TestStreamWriter }

func NewTestEventSink(writer TestStreamWriter) *TestEventSink { return &TestEventSink{writer: writer} }

func (s *TestEventSink) Begin(_ context.Context, commit bool) error {
	s.writer.Header().Set("Content-Type", "text/event-stream")
	s.writer.Header().Set("Cache-Control", "no-cache")
	if commit {
		s.writer.Header().Set("Connection", "keep-alive")
		s.writer.Header().Set("X-Accel-Buffering", "no")
		s.writer.Flush()
	}
	return nil
}

func (s *TestEventSink) Emit(_ context.Context, event provider.TestEvent) error {
	// 编码失败时写出空 data 行，写出失败则返回错误，由用例取消执行。
	raw, _ := json.Marshal(event)
	if _, err := fmt.Fprintf(s.writer, "data: %s\n\n", raw); err != nil {
		return err
	}
	s.writer.Flush()
	return nil
}
