package forward

import (
	"context"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/gateway/requeststate"
	"github.com/TokenFlux/TokenRouter/internal/upstream"
)

// APIKeyInput 保存直通请求的报文引用和字段，平台回写使用同一解析对象。
type APIKeyInput struct {
	Body                        []byte
	Parsed                      *requeststate.ParsedRequest
	RequestModel, OriginalModel string
	RequestStream               bool
	StartTime                   time.Time
}

// PassthroughPorts 提供与单个提供商进行协议交换的操作。
type PassthroughPorts interface {
	Begin() (func(), error)
	Credential(context.Context) error
	TokenKind() string
	ResolveProxy()
	MarkPassthrough()
	FilterSearchHistory([]byte, string) []byte
	ReadErrorBody() ([]byte, error)
	ResetErrorBody([]byte)
	Health(context.Context, string, int, map[string][]string, []byte, string) ErrorDecision
	HandleError(context.Context, string, bool) (*Result, error)
	Observe(Notice)
	FailoverError(int, []byte, bool) error
	IsFailover(error) bool
	Log(string)
	Truncate(string, int) string
	ExecutePassthrough(context.Context, *APIKeyInput, MessageHooks) (upstream.AttemptResult, error)
	ServiceTier() string
}
