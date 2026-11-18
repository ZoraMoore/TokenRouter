package openai

import (
	"encoding/json"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ResponseLifecycleNormalizer 修复单次 HTTP Responses 流的生命周期事件，保留响应中的未知字段。
// 重复成功终态只标记为不应下发，调用方仍须读取其中的用量和观测信息。
// @project-doc docs/interfaces/openai_upstream.md#openai_protocol_dispatch
type ResponseLifecycleNormalizer struct {
	nextSequence int64
	completedID  string
}

// IsResponseLifecycleEvent 识别携带完整 Response 对象的生命周期事件。
func IsResponseLifecycleEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "response.created", "response.in_progress", "response.completed", "response.done",
		"response.failed", "response.incomplete", "response.cancelled", "response.canceled":
		return true
	default:
		return false
	}
}

// Normalize 补齐裸 Response 的事件包装，并将旧 done 事件按实际状态转为标准终态。
// 返回值依次为载荷、事件类型和重复成功终态标记；畸形 JSON 与未知事件保持原样。
func (n *ResponseLifecycleNormalizer) Normalize(data []byte, eventType string) ([]byte, string, bool) {
	eventType = EffectiveOpenAISSEEventType(data, eventType)
	sequence := n.nextSequence
	if value := gjson.GetBytes(data, "sequence_number"); value.Type == gjson.Number && value.Int() >= sequence {
		sequence = value.Int()
	}
	n.nextSequence = sequence + 1
	if !IsResponseLifecycleEvent(eventType) || !gjson.ValidBytes(data) {
		return data, eventType, false
	}

	response := gjson.GetBytes(data, "response")
	changed := false
	if !response.IsObject() {
		// 裸对象带有 Response 身份字段时才补上成功响应包装。
		if gjson.GetBytes(data, "object").String() != "response" || gjson.GetBytes(data, "id").String() == "" {
			return data, eventType, false
		}
		response = gjson.ParseBytes(data)
		wrapped, err := json.Marshal(struct {
			Type     string          `json:"type"`
			Response json.RawMessage `json:"response"`
		}{Type: eventType, Response: data})
		if err != nil {
			return data, eventType, false
		}
		data = wrapped
		changed = true
	}
	if eventType == "response.done" {
		status := response.Get("status").String()
		if status == "" {
			// 缺少状态时只根据明确的失败信息补齐，不能把空状态当成成功。
			switch {
			case response.Get("error").Exists() && response.Get("error").Type != gjson.Null:
				status = "failed"
			case response.Get("incomplete_details").Exists() && response.Get("incomplete_details").Type != gjson.Null:
				status = "incomplete"
			default:
				return data, eventType, false
			}
			data, _ = sjson.SetBytes(data, "response.status", status)
			changed = true
		}
		switch status {
		case "completed":
			eventType = "response.completed"
		case "failed":
			eventType = "response.failed"
		case "incomplete":
			eventType = "response.incomplete"
		case "cancelled", "canceled":
			eventType = "response." + status
		default:
			// 未知或进行中的状态不能推断为成功。
			return data, eventType, false
		}
	}
	if gjson.GetBytes(data, "type").String() != eventType {
		data, _ = sjson.SetBytes(data, "type", eventType)
		changed = true
	}
	if changed && !gjson.GetBytes(data, "sequence_number").Exists() {
		data, _ = sjson.SetBytes(data, "sequence_number", sequence)
	}

	id := response.Get("id").String()
	duplicate := eventType == "response.completed" && id != "" && n.completedID == id
	if eventType == "response.completed" && id != "" {
		n.completedID = id
	}
	return data, eventType, duplicate
}
