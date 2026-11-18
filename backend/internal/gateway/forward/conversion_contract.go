package forward

import (
	"context"
	"encoding/json"
)

// ConversionInput 保存本次平台类型，执行时通过凭据读取函数取得授权信息。
type ConversionInput struct{ OAuth bool }

// ErrorDecision 保存健康策略的处理结果。
type ErrorDecision struct{ Generic, Failover, RetrySameProvider bool }

// ConversionPorts 提供网络请求和提供商检查的单步操作，转换流程决定调用及错误处理顺序。
type ConversionPorts interface {
	NormalizeResponses([]byte) ([]byte, bool, error)
	ResolveModel(context.Context, string) string
	Effort([]byte, bool, ...string) *string
	ThinkingFallback(*string, []byte, string) *string
	ModelNotice(string, string, string, bool)
	Mimic(context.Context, []byte, json.RawMessage, string) []byte
	CacheLimit([]byte) []byte
	Credential(context.Context) error
	Build(context.Context, []byte, string, bool, bool) ([]byte, error)
	Send(context.Context) (Response, error)
	ReadErrorBody() ([]byte, error)
	ErrorMessage([]byte) string
	Health(context.Context, int, []byte, string) ErrorDecision
	FailoverNotice(int, string)
	FailoverError(int, []byte, bool) error
	Output() Output
}

// MapStatus 保留上游五百类状态对客户端的映射。
func MapStatus(status int) int {
	if status >= 500 {
		return 502
	}
	return status
}
