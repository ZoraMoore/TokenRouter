package openai

import (
	"strconv"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/protocol/openai"
)

// CompactionTestPayload 构造 V2 流式 Responses 请求，最后一个 input 为 compaction_trigger。
func CompactionTestPayload(model string, isOAuth bool) map[string]any {
	payload := map[string]any{
		"model":        strings.TrimSpace(model),
		"instructions": "You are a helpful coding assistant.",
		"input": []any{
			map[string]any{
				"type":    "message",
				"role":    "user",
				"content": "Respond with OK.",
			},
			map[string]any{"type": "compaction_trigger"},
		},
		"stream": true,
	}
	if isOAuth {
		// ChatGPT 内部 Responses API 与正常 OAuth 转发保持相同的 store 约束。
		payload["store"] = false
	}
	return payload
}

// LegacyCompactionTestPayload 构造旧端点的 unary 请求，供管理员测试兼容性。
// V2 能力需要通过 CompactionTestPayload 测试。
func LegacyCompactionTestPayload(model string) map[string]any {
	return map[string]any{
		"model":        strings.TrimSpace(model),
		"instructions": "You are a helpful coding assistant.",
		"input": []any{
			map[string]any{
				"type":    "message",
				"role":    "user",
				"content": "Respond with OK.",
			},
		},
	}
}

// CompactionTestHasOutput 确认原生 V2 响应确实给出了 compaction
// item。单纯 2xx 可能表示中间链路吞掉 trigger，不能将其报告为测试成功。
func CompactionTestHasOutput(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	bodyText := string(body)
	if _, found := openai.FindRawCompactionItemFromSSE(bodyText); found {
		return true
	}
	if finalResponse, ok := openai.ExtractCodexFinalResponse(bodyText); ok && openai.ResponsesOutputHasCompactionItem(finalResponse) {
		return true
	}
	return openai.ResponsesOutputHasCompactionItem(body)
}

func CompactionTestSessionID(providerID int64) string {
	// 保留既有会话标识，避免重命名内部函数改变上游对测试请求的处理。
	if providerID <= 0 {
		return DeriveStableUUIDv4("tokenrouter:openai-native-compaction-v2-probe:anonymous")
	}
	return DeriveStableUUIDv4("tokenrouter:openai-native-compaction-v2-probe:" + strconv.FormatInt(providerID, 10))
}

// LegacyCompactionTestSessionID 保持旧端点既有会话格式，避免兼容性测试本身改变
// legacy 上游对请求形状的识别。
func LegacyCompactionTestSessionID(providerID int64) string {
	if providerID <= 0 {
		return "probe_compact"
	}
	return "probe_compact_" + strconv.FormatInt(providerID, 10)
}
