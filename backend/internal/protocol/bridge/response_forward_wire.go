package bridge

import (
	"encoding/json"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	protocolopenai "github.com/TokenFlux/TokenRouter/internal/protocol/openai"
)

func NormalizeResponsesStreamingTerminalOutput(data []byte, acc *BufferedResponseAccumulator, doneItems *ResponsesStreamOutputItems, imageOutputs []json.RawMessage) ([]byte, bool) {
	eventType := strings.TrimSpace(gjson.GetBytes(data, "type").String())
	switch eventType {
	case "response.completed", "response.done", "response.incomplete", "response.cancelled", "response.canceled":
	default:
		return data, false
	}

	output := gjson.GetBytes(data, "response.output")
	hasAccumulatedOutput := (acc != nil && acc.HasContent()) || len(imageOutputs) > 0 || doneItems.HasItems()
	outputJSON := []byte("[]")
	if reconstructed, ok := doneItems.mergeTerminalOutput(output); ok {
		outputJSON = reconstructed
	} else if output.IsArray() && (len(output.Array()) > 0 || !hasAccumulatedOutput) {
		return data, false
	} else if reconstructed, ok := BuildResponsesOutputJSON(acc, imageOutputs); ok {
		outputJSON = reconstructed
	}
	updated, err := sjson.SetRawBytes(data, "response.output", outputJSON)
	if err != nil {
		return data, false
	}
	return updated, true
}

// CollectRawResponsesOutputItemsFromSSE 按到达顺序收集 output_item.done 中的原始 item。
// 已产出结果的图片项会修正进行中状态，其余字段保留原始 JSON 字节，包括 encrypted_content、
// summary、opaque 等压缩字段（#3777 问题 2）。done 缺少 compaction 时，再从 added 收集，
// 部分上游的 compaction 完整结果仅出现在 added 中。
func CollectRawResponsesOutputItemsFromSSE(bodyText string) ([]byte, bool) {
	observed := NewResponsesStreamOutputItems()
	var addedCompaction []json.RawMessage
	protocolopenai.ForEachSSEDataPayload(bodyText, func(data []byte) {
		if normalized, changed := protocolopenai.NormalizeCompletedImageGenerationStatus(data); changed {
			data = normalized
		}
		observed.Observe(data)
		if gjson.GetBytes(data, "type").Str == "response.output_item.added" {
			item := gjson.GetBytes(data, "item")
			if protocolopenai.IsResponsesCompactionItemType(item.Get("type").Str) {
				addedCompaction = append(addedCompaction, json.RawMessage(item.Raw))
			}
		}
	})
	var items []json.RawMessage
	seen := map[string]struct{}{}
	appendItem := func(raw json.RawMessage) {
		key := gjson.GetBytes(raw, "id").Str
		if key == "" {
			key = string(raw)
		}
		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			items = append(items, raw)
		}
	}
	hasCompaction := false
	for _, item := range observed.orderedItems() {
		appendItem(item.raw)
		hasCompaction = hasCompaction || protocolopenai.IsResponsesCompactionItemType(item.slot.key.kind)
	}
	if !hasCompaction {
		for _, item := range addedCompaction {
			appendItem(item)
		}
	}
	if len(items) == 0 {
		return nil, false
	}
	outputJSON, err := json.Marshal(items)
	if err != nil {
		return nil, false
	}
	return outputJSON, true
}

// ReconstructResponseOutputFromSSE 扫描原始 SSE 正文，为 output 为空的终止
// 事件重建 JSON output 数组。优先使用 output_item.done 中的原始条目，因为
// Responses 协议将其定义为条目的最终形态；delta 累加仅覆盖文本、函数调用和
// reasoning 内容，会静默丢弃 compaction 等未知类型，进而导致 Codex remote
// compact v2 报告没有拿到唯一的 compaction 条目（#3887）。无法重建时返回
// (nil, false)。
func ReconstructResponseOutputFromSSE(bodyText string) ([]byte, bool) {
	if outputJSON, ok := CollectRawResponsesOutputItemsFromSSE(bodyText); ok {
		return outputJSON, true
	}
	acc := NewBufferedResponseAccumulator()
	imageOutputs := make([]json.RawMessage, 0, 1)
	seenImages := make(map[string]struct{})
	protocolopenai.ForEachSSEDataPayload(bodyText, func(data []byte) {
		if imageOutput, ok := protocolopenai.ExtractImageGenerationOutputFromSSEData(data, seenImages); ok {
			imageOutputs = append(imageOutputs, imageOutput)
		}
		eventType := strings.TrimSpace(gjson.GetBytes(data, "type").String())
		if protocolopenai.ResponsesStreamEventMayContributeToOutput(eventType) {
			var event protocolopenai.ResponsesStreamEvent
			if err := json.Unmarshal(data, &event); err == nil {
				acc.ProcessEvent(&event)
			}
		}
	})
	return BuildResponsesOutputJSON(acc, imageOutputs)
}

func BuildResponsesOutputJSON(acc *BufferedResponseAccumulator, imageOutputs []json.RawMessage) ([]byte, bool) {
	if (acc == nil || !acc.HasContent()) && len(imageOutputs) == 0 {
		return nil, false
	}

	var output []json.RawMessage
	if acc != nil && acc.HasContent() {
		outputJSON, err := json.Marshal(acc.BuildOutput())
		if err != nil {
			return nil, false
		}
		if err := json.Unmarshal(outputJSON, &output); err != nil {
			return nil, false
		}
	}
	output = append(output, imageOutputs...)

	outputJSON, err := json.Marshal(output)
	if err != nil {
		return nil, false
	}
	return outputJSON, true
}

// RepairBufferedResponsesOutput 使用缓存的 SSE 帧补全返回给非流式客户端的终态输出。
func RepairBufferedResponsesOutput(final []byte, body string) []byte {
	output := gjson.GetBytes(final, "output")
	var rebuilt []byte
	var ok bool
	if len(output.Array()) == 0 {
		rebuilt, ok = ReconstructResponseOutputFromSSE(body)
	} else {
		items := NewResponsesStreamOutputItems()
		protocolopenai.ForEachSSEDataPayload(body, func(data []byte) { items.Observe(data) })
		rebuilt, ok = items.mergeTerminalOutput(output)
	}
	if ok {
		if updated, err := sjson.SetRawBytes(final, "output", rebuilt); err == nil {
			return updated
		}
	}
	return final
}
