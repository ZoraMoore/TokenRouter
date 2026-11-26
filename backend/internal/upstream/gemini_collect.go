package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
)

// geminiCollectedCandidate 保存一个候选的累计内容和最终元数据。
type geminiCollectedCandidate struct {
	value    map[string]any
	parts    []any
	finished bool
}

// CollectGeminiStream 按候选索引聚合响应，并写入共享读取器合并的尾部用量。
func CollectGeminiStream(ctx context.Context, resp *http.Response, options GeminiStreamOptions, observe func([]byte)) (map[string]any, GeminiStreamResult, error) {
	response := make(map[string]any)
	candidates := make(map[int]*geminiCollectedCandidate)
	result, err := ReadGeminiStream(ctx, resp, options, func(event GeminiStreamEvent) error {
		if len(event.Data) == 0 {
			return nil
		}
		if observe != nil {
			observe(event.Data)
		}
		var payload map[string]any
		decoder := json.NewDecoder(bytes.NewReader(event.Data))
		decoder.UseNumber()
		if err := decoder.Decode(&payload); err != nil {
			return err
		}
		for key, value := range payload {
			if key != "candidates" {
				response[key] = value
			}
		}
		values, _ := payload["candidates"].([]any)
		for position, value := range values {
			candidate, ok := value.(map[string]any)
			if !ok {
				continue
			}
			index := position
			if number, ok := candidate["index"].(json.Number); ok {
				if parsed, err := number.Int64(); err == nil && parsed >= 0 {
					index = int(parsed)
				}
			}
			collected := candidates[index]
			if collected == nil {
				collected = &geminiCollectedCandidate{value: make(map[string]any)}
				candidates[index] = collected
			}
			collected.add(candidate)
		}
		return nil
	})
	if len(candidates) > 0 {
		indices := make([]int, 0, len(candidates))
		for index := range candidates {
			indices = append(indices, index)
		}
		slices.Sort(indices)
		values := make([]any, 0, len(indices))
		for _, index := range indices {
			values = append(values, candidates[index].value)
		}
		response["candidates"] = values
	}
	if len(result.UsageJSON) > 0 {
		var usage map[string]json.RawMessage
		if json.Unmarshal(result.UsageJSON, &usage) == nil {
			response["usageMetadata"] = usage["usageMetadata"]
		}
	}
	return response, result, err
}

// add 合并同一个候选的内容片段和终态字段。
func (c *geminiCollectedCandidate) add(candidate map[string]any) {
	for key, value := range candidate {
		if key != "content" {
			c.value[key] = value
		}
	}
	if incoming, ok := candidate["content"].(map[string]any); ok {
		content, _ := c.value["content"].(map[string]any)
		if content == nil {
			content = make(map[string]any)
			c.value["content"] = content
		}
		for key, value := range incoming {
			if key != "parts" {
				content[key] = value
			}
		}
		if !c.finished {
			parts, _ := incoming["parts"].([]any)
			for _, part := range parts {
				if len(c.parts) > 0 && mergeGeminiCollectedText(c.parts[len(c.parts)-1], part) {
					continue
				}
				c.parts = append(c.parts, part)
			}
		}
		content["parts"] = c.parts
	}
	if finish, ok := candidate["finishReason"].(string); ok && finish != "" && finish != "FINISH_REASON_UNSPECIFIED" {
		c.finished = true
	}
}

// mergeGeminiCollectedText 在片段元数据相同时合并相邻文本。
func mergeGeminiCollectedText(previous, next any) bool {
	a, ok := previous.(map[string]any)
	if !ok {
		return false
	}
	b, ok := next.(map[string]any)
	if !ok || len(a) != len(b) {
		return false
	}
	first, ok := a["text"].(string)
	if !ok {
		return false
	}
	second, ok := b["text"].(string)
	if !ok {
		return false
	}
	for key, value := range a {
		if key != "text" && !reflect.DeepEqual(value, b[key]) {
			return false
		}
	}
	a["text"] = first + second
	return true
}
