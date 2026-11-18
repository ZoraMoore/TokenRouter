package forward

import (
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/gateway/tierpolicy"
	"github.com/TokenFlux/TokenRouter/internal/protocol"
)

// ResponseObserver 分别保存模型声明与实际服务档位，不改变出站请求和计费模型。
type ResponseObserver struct {
	model protocol.ResponseModelObserver

	firstTier         string
	firstTierConflict bool
	terminalTier      string
}

func (o *ResponseObserver) Observe(model string, terminal bool) {
	if o != nil {
		o.model.Observe(model, terminal)
	}
}

func (o *ResponseObserver) ObserveOpenAI(payload []byte, eventType string) {
	model := firstValidTrimmedGJSONModel(payload, "response.model", "model")
	terminal := isUpstreamResponseModelTerminalEvent(eventType)
	// 上游只有携带 model 的事件才同时提供可信的 service_tier；无 model 的
	// 增量帧不能作为计费依据。
	if model == "" {
		return
	}
	o.model.ObserveOpenAI(payload, eventType)
	// Responses 的非终止事件通常只是回显请求档位，只有终止事件和无类型的
	// Chat Completions/非流式 JSON 才能作为实际处理档位的证据。
	if !terminal && strings.TrimSpace(eventType) != "" {
		return
	}
	tier := NormalizeObservedOpenAIServiceTier(firstValidTrimmedGJSONModel(payload, "response.service_tier", "service_tier"))
	o.ObserveServiceTier(tier, terminal)
}

func (o *ResponseObserver) ObserveAnthropic(payload []byte) {
	model := firstValidTrimmedGJSONModel(payload, "message.model", "model")
	if model != "" {
		o.Observe(model, false)
	}
	tier := normalizeObservedAnthropicSpeed(firstValidTrimmedGJSONModel(payload, "message.usage.speed", "usage.speed"))
	o.ObserveServiceTier(tier, false)
}

// ObserveServiceTier 记录上游声明的档位，终态优先；非终态声明相互冲突时清除该观测。
func (o *ResponseObserver) ObserveServiceTier(tier string, terminal bool) {
	if o == nil || tier == "" {
		return
	}
	if terminal {
		o.terminalTier = tier
		return
	}
	if o.firstTier == "" {
		o.firstTier = tier
		return
	}
	if o.firstTier != tier {
		o.firstTierConflict = true
	}
}

// ServiceTier 返回无歧义的上游实际服务档位；没有声明或声明冲突时返回空。
func (o *ResponseObserver) ServiceTier() string {
	if o == nil {
		return ""
	}
	if o.terminalTier != "" {
		return o.terminalTier
	}
	if o.firstTierConflict {
		return ""
	}
	return o.firstTier
}

// NormalizeObservedOpenAIServiceTier 仅接受上游已知档位，保留 fast 的标准化规则。
func NormalizeObservedOpenAIServiceTier(raw string) string {
	switch value := strings.ToLower(strings.TrimSpace(raw)); value {
	case "priority", "fast":
		return tierpolicy.OpenAIFastTierPriority
	case "default", "flex", "scale":
		return value
	default:
		return ""
	}
}

func normalizeObservedAnthropicSpeed(raw string) string {
	switch value := strings.ToLower(strings.TrimSpace(raw)); value {
	case "fast", "standard":
		return value
	default:
		return ""
	}
}

func (o *ResponseObserver) Model() string {
	if o == nil {
		return ""
	}
	return o.model.Model()
}

func firstValidTrimmedGJSONModel(payload []byte, paths ...string) string {
	return protocol.ResponseModelString(payload, paths...)
}

func isUpstreamResponseModelTerminalEvent(event string) bool {
	return protocol.IsResponseModelTerminalEvent(event)
}

// ResetModel 在再次发送上游请求前清空模型声明，保留既有服务档位处理。
func (o *ResponseObserver) ResetModel() {
	if o != nil {
		o.model = protocol.ResponseModelObserver{}
	}
}

// ObserveOpenAIModel 仅补充模型声明，供原本不采集服务档位的透传路径使用。
func (o *ResponseObserver) ObserveOpenAIModel(payload []byte, event string) {
	if o != nil {
		o.model.ObserveOpenAI(payload, event)
	}
}
