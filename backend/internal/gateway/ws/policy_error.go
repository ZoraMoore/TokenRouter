package ws

import "fmt"

// GenericPolicyError 表示已开启自定义错误码，但当前状态未匹配配置，HTTP 入站返回统一 500。
type GenericPolicyError struct {
	upstreamStatus int
}

func (e *GenericPolicyError) Error() string {
	if e == nil || e.upstreamStatus == 0 {
		return "upstream websocket error not in custom error codes"
	}
	return fmt.Sprintf("upstream websocket status %d not in custom error codes", e.upstreamStatus)
}

// NewGenericPolicyError 保留未命中自定义状态码时的内部错误，不向客户端泄露上游正文。
func NewGenericPolicyError(status int) error { return &GenericPolicyError{upstreamStatus: status} }
