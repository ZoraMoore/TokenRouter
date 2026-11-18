package openai

import (
	"encoding/json"
	"errors"
	"strings"
)

// LiveCallRequest 是两种下游创建协议转换后的请求，Session 保存原始 JSON。
type LiveCallRequest struct {
	SDP     string          `json:"sdp"`
	Session json.RawMessage `json:"session"`
}

// ValidateLiveCallRequest 校验 SDP 和原始 session JSON 的基本结构。
func ValidateLiveCallRequest(request *LiveCallRequest) error {
	if request == nil || strings.TrimSpace(request.SDP) == "" {
		return errors.New("sdp is required")
	}
	if len(request.Session) == 0 || !json.Valid(request.Session) {
		return errors.New("session must be valid JSON")
	}
	var sessionObject map[string]json.RawMessage
	if err := json.Unmarshal(request.Session, &sessionObject); err != nil {
		return errors.New("session must be a JSON object")
	}
	if sessionObject == nil {
		return errors.New("session must be a JSON object")
	}
	return nil
}
