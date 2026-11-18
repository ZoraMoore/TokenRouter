package protocol

import (
	"strings"

	"github.com/tidwall/gjson"
)

// ResponseModelObserver 保存单次上游尝试或 WS turn 的原始模型声明，供观测使用。
type ResponseModelObserver struct {
	first    string
	terminal string
}

// Observe 记录有效模型；终态声明优先于首次声明。
func (o *ResponseModelObserver) Observe(model string, terminal bool) {
	if o == nil {
		return
	}
	model = NormalizeResponseModel(model)
	if model == "" {
		return
	}
	if terminal {
		o.terminal = model
	} else if o.first == "" {
		o.first = model
	}
}

// NormalizeResponseModel 限制观测字段长度，保留模型版本与供应商前缀。
func NormalizeResponseModel(model string) string {
	model = strings.TrimSpace(model)
	runes := []rune(model)
	if len(runes) > 200 {
		return string(runes[:200])
	}
	return model
}

// ObserveOpenAI 仅读取协议元数据；Chat chunk 始终保留首次声明。
func (o *ResponseModelObserver) ObserveOpenAI(payload []byte, event string) {
	terminal := IsResponseModelTerminalEvent(event) && !gjson.GetBytes(payload, "choices").IsArray()
	o.Observe(ResponseModelString(payload, "response.model", "model"), terminal)
}

// ObserveAnthropic 读取完整 Message 或 message_start 的模型声明。
func (o *ResponseModelObserver) ObserveAnthropic(payload []byte) {
	o.Observe(ResponseModelString(payload, "message.model", "model"), false)
}

// ObserveGemini 保留最后一次原始模型版本，兼容 Google 内部响应封装。
func (o *ResponseModelObserver) ObserveGemini(payload []byte) {
	o.Observe(ResponseModelString(payload, "modelVersion", "response.modelVersion", "response.response.modelVersion"), true)
}

// Model 返回已观察到的模型；未声明时返回空，不使用请求模型补齐。
func (o *ResponseModelObserver) Model() string {
	if o == nil {
		return ""
	}
	if o.terminal != "" {
		return o.terminal
	}
	return o.first
}

// ResponseModelString 只接受有效 JSON 中指定位置的非空字符串。
func ResponseModelString(payload []byte, paths ...string) string {
	for _, path := range paths {
		value := gjson.GetBytes(payload, path)
		if value.Type == gjson.String && strings.TrimSpace(value.Str) != "" {
			if !gjson.ValidBytes(payload) {
				return ""
			}
			return strings.TrimSpace(value.Str)
		}
	}
	return ""
}

// IsResponseModelTerminalEvent 判断 Responses 的终态模型声明。
func IsResponseModelTerminalEvent(event string) bool {
	switch strings.TrimSpace(event) {
	case "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
		return true
	default:
		return false
	}
}
