package operation

import (
	"errors"
	"testing"
	"time"
)

func TestOperationOutcome(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		state   State
		message string
		delay   time.Duration
	}{
		{name: "completed", state: Succeeded},
		{name: "upstream wants an operator", err: NeedsAction("需要人工授权"), state: RequiresAction, message: "需要人工授权"},
		{name: "accepted can be polled later", err: RetryAfter("等待授权", 12*time.Second), state: Queued, message: "等待授权", delay: 12 * time.Second},
		{name: "internal errors stay private", err: errors.New("credential from upstream"), state: Failed, message: "操作失败，请查看服务端日志"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			state, message, delay := outcome(tt.err)
			if state != tt.state || message != tt.message || delay != tt.delay {
				t.Fatalf("got %q, %q, %s", state, message, delay)
			}
		})
	}
}
