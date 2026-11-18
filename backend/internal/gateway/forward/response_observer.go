package forward

import (
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/protocol"
	"github.com/TokenFlux/TokenRouter/internal/protocol/openai"
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
	if o == nil {
		return
	}
	eventType = openai.EffectiveOpenAISSEEventType(payload, eventType)
	if eventType == "" {
		eventType = openai.OpenAIChatCompletionServiceTierEventType(payload)
	}
	o.model.ObserveOpenAI(payload, eventType)
	terminal := isUpstreamResponseModelTerminalEvent(eventType)
	// Responses 的早期事件会回显请求档位。终态可能只声明实际档位，省略模型。
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
	return openai.ObservedServiceTier(raw)
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

// Reset 清除上一次尝试的模型与档位，避免将失败响应的声明用于下一次结算。
func (o *ResponseObserver) Reset() {
	if o != nil {
		*o = ResponseObserver{}
	}
}
