package openai

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	wire "github.com/TokenFlux/TokenRouter/internal/protocol/openai"
)

// NativeCompactionDiagnostics 记录压缩项的来源和数量，供请求日志定位协议错误。
type NativeCompactionDiagnostics struct {
	Source, Reason                  string
	AddedItems, DoneItems, EndItems int
}

// nativeCompactionBody 在成功终态确认后生成完整的压缩完成事件。
// 等待期间输出 SSE 注释，使外层读取器继续观测上游活动。
type nativeCompactionBody struct {
	ctx        context.Context
	source     io.ReadCloser
	scanner    *wire.SSEJSONDocumentScanner
	parser     wire.OpenAICompatSSEFrameParser
	lifecycle  wire.ResponseLifecycleNormalizer
	added      map[int]json.RawMessage
	done       map[int]json.RawMessage
	usage      json.RawMessage
	bareError  []byte
	responseID string
	pending    []byte
	ended      bool
	created    bool
	bytes      int
	limit      int
	sequence   int64
	diagnostic NativeCompactionDiagnostics
	observe    func(NativeCompactionDiagnostics)
}

// normalizeNativeCompactionResponse 为已标记的 V2 请求安装结果校验器。
func normalizeNativeCompactionResponse(ctx context.Context, resp *http.Response, options StreamOptions) *http.Response {
	if !options.NativeCompactionV2 {
		return resp
	}
	scanner := bufio.NewScanner(resp.Body)
	limit := options.MaxLineSize
	if limit <= 0 {
		limit = OpenAIFirstOutputStageMaxBytes
	}
	// 压缩结果在校验前暂存，使用首输出阶段的容量上限。
	limit = min(limit, OpenAIFirstOutputStageMaxBytes)
	scanner.Buffer(make([]byte, min(64*1024, limit)), limit)
	copyResponse := *resp
	copyResponse.Body = &nativeCompactionBody{
		ctx: ctx, source: resp.Body, scanner: wire.NewSSEJSONDocumentScanner(scanner),
		added: make(map[int]json.RawMessage), done: make(map[int]json.RawMessage),
		limit: limit, observe: options.NativeCompactionResult,
	}
	return &copyResponse
}

// Read 逐帧处理上游事件，完成校验后输出压缩结果。
func (r *nativeCompactionBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for len(r.pending) == 0 {
		if err := r.ctx.Err(); err != nil {
			return 0, err
		}
		if r.ended {
			return 0, io.EOF
		}
		if r.scanner.Scan() {
			line := r.scanner.Text()
			if frame, ok := r.parser.AddLine(line); ok {
				r.pending = r.consume(frame)
			} else if strings.HasPrefix(line, ":") {
				r.pending = []byte(": native compaction pending\n\n")
			}
			continue
		}
		if err := r.scanner.Err(); err != nil {
			r.report("read_error")
			return 0, err
		}
		if frame, ok := r.parser.Finish(); ok {
			r.pending = r.consume(frame)
		} else {
			r.pending = r.fail("missing_completed")
		}
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

// Close 关闭上游响应，使取消和超时可以中断底层读取。
func (r *nativeCompactionBody) Close() error { return r.source.Close() }

// consume 保存结果项和用量，并按上游终态选择完成或失败事件。
func (r *nativeCompactionBody) consume(frame wire.OpenAICompatSSEFrame) []byte {
	if strings.TrimSpace(frame.Data) == "[DONE]" {
		return r.fail("missing_completed")
	}
	data := []byte(wire.OpenAICompatPayloadWithEventType(frame.Data, frame.EventType))
	if !gjson.ValidBytes(data) {
		return r.fail("invalid_json")
	}
	data, event, _ := r.lifecycle.Normalize(data, frame.EventType)
	if id := wire.ExtractOpenAIResponseIDFromJSONBytes(data); id != "" {
		r.responseID = id
	}
	for _, path := range []string{"response.usage", "usage"} {
		if value := gjson.GetBytes(data, path); value.IsObject() {
			r.usage = json.RawMessage(value.Raw)
			break
		}
	}
	switch event {
	case "response.output_item.added", "response.output_item.done":
		item := gjson.GetBytes(data, "item")
		if item.IsObject() {
			items := r.added
			if event == "response.output_item.done" {
				items = r.done
			}
			index := int(gjson.GetBytes(data, "output_index").Int())
			if previous, exists := items[index]; exists &&
				gjson.GetBytes(previous, "id").String() != item.Get("id").String() {
				return r.fail("conflicting_item_index")
			}
			r.bytes += len(item.Raw) - len(items[index])
			if r.bytes > r.limit {
				return r.fail("buffer_limit")
			}
			items[index] = json.RawMessage(item.Raw)
		}
	case "response.completed":
		if status := gjson.GetBytes(data, "response.status").String(); status != "" && status != "completed" {
			return r.fail("incomplete_terminal")
		}
		return r.complete(data)
	case "response.failed":
		r.ended = true
		r.report("upstream_failed")
		return r.event(data, event)
	case "response.incomplete", "response.cancelled", "response.canceled", "response.done":
		return r.fail("incomplete_terminal")
	case "error":
		// 独立错误可能被后续成功终态覆盖，终态之前暂存其诊断。
		r.bareError = data
	case "response.created", "response.in_progress":
		data, _ = sjson.SetRawBytes(data, "response.output", []byte("[]"))
		if event == "response.created" {
			if r.created {
				return []byte(": native compaction pending\n\n")
			}
			r.created = true
		}
		prefix := r.ensureCreated(data)
		return append(prefix, r.event(data, event)...)
	}
	// 用量在失败前也会出现，交给外层读取器保存供结算使用。
	if len(r.usage) > 0 {
		data, _ := json.Marshal(map[string]any{
			"type":     "response.in_progress",
			"response": map[string]any{"id": r.responseID, "status": "in_progress", "output": []any{}, "usage": r.usage},
		})
		prefix := r.ensureCreated(data)
		return append(prefix, r.event(data, "response.in_progress")...)
	}
	return []byte(": native compaction pending\n\n")
}

// complete 优先采用终态结果，用完成事件和 added 事件补齐压缩项。
func (r *nativeCompactionBody) complete(data []byte) []byte {
	output := gjson.GetBytes(data, "response.output")
	if output.Exists() && !output.IsArray() {
		return r.fail("invalid_output")
	}
	var items []json.RawMessage
	seen := make(map[string]struct{})
	appendItem := func(raw json.RawMessage) {
		item := gjson.ParseBytes(raw)
		key := item.Get("id").String()
		if key == "" {
			key = string(raw)
		}
		if _, exists := seen[key]; exists {
			return
		}
		// 部分终态省略 item ID，以同一份密文匹配已完成的压缩项。
		if wire.IsResponsesCompactionItemType(item.Get("type").String()) {
			cipher := item.Get("encrypted_content")
			for _, previous := range items {
				known := gjson.ParseBytes(previous)
				if (item.Get("id").String() == "" || known.Get("id").String() == "") &&
					wire.IsResponsesCompactionItemType(known.Get("type").String()) &&
					cipher.Type == gjson.String && cipher.Str != "" && known.Get("encrypted_content").Str == cipher.Str {
					return
				}
			}
		}
		seen[key] = struct{}{}
		items = append(items, raw)
	}
	for _, item := range output.Array() {
		if !item.IsObject() {
			return r.fail("invalid_item")
		}
		items = append(items, json.RawMessage(item.Raw))
		key := item.Get("id").String()
		if key == "" {
			key = item.Raw
		}
		seen[key] = struct{}{}
	}
	r.diagnostic.EndItems = countCompactionItems(items)
	if r.diagnostic.EndItems > 0 {
		r.diagnostic.Source = "terminal"
	}
	for _, index := range sortedCompactionIndexes(r.done) {
		appendItem(r.done[index])
	}
	if r.diagnostic.Source == "" && countCompactionItems(items) > 0 {
		r.diagnostic.Source = "done"
	}
	if countCompactionItems(items) == 0 {
		for _, index := range sortedCompactionIndexes(r.added) {
			item := gjson.ParseBytes(r.added[index])
			if wire.IsResponsesCompactionItemType(item.Get("type").String()) {
				appendItem(r.added[index])
			}
		}
		if countCompactionItems(items) > 0 {
			r.diagnostic.Source = "added"
		}
	}
	if count := countCompactionItems(items); count != 1 {
		return r.fail("compaction_count")
	}
	for index, raw := range items {
		item := gjson.ParseBytes(raw)
		if !wire.IsResponsesCompactionItemType(item.Get("type").String()) {
			continue
		}
		cipher := item.Get("encrypted_content")
		if cipher.Type != gjson.String || strings.TrimSpace(cipher.Str) == "" {
			return r.fail("missing_encrypted_content")
		}
		if status := item.Get("status").String(); status != "" && status != "completed" {
			return r.fail("incomplete_item")
		}
		items[index], _ = sjson.SetBytes(raw, "type", "compaction")
	}
	encoded, _ := json.Marshal(items)
	data, _ = sjson.SetRawBytes(data, "response.output", encoded)
	if !gjson.GetBytes(data, "response.usage").Exists() && len(r.usage) > 0 {
		data, _ = sjson.SetRawBytes(data, "response.usage", r.usage)
	}
	result := r.ensureCreated(data)
	for index, item := range items {
		for _, event := range []string{"response.output_item.added", "response.output_item.done"} {
			payload, _ := json.Marshal(map[string]any{"type": event, "output_index": index, "item": item})
			result = append(result, r.event(payload, event)...)
		}
	}
	result = append(result, r.event(data, "response.completed")...)
	r.ended = true
	r.report("")
	return result
}

// ensureCreated 为省略起始事件的上游补齐空的进行中响应。
func (r *nativeCompactionBody) ensureCreated(data []byte) []byte {
	if r.created {
		return nil
	}
	created, _ := sjson.SetBytes(data, "type", "response.created")
	created, _ = sjson.SetBytes(created, "response.status", "in_progress")
	created, _ = sjson.SetRawBytes(created, "response.output", []byte("[]"))
	created, _ = sjson.DeleteBytes(created, "response.usage")
	r.created = true
	return r.event(created, "response.created")
}

// fail 将无效结果转换为包含已观测用量的失败终态。
func (r *nativeCompactionBody) fail(reason string) []byte {
	r.ended = true
	r.report(reason)
	message := fmt.Sprintf("Native compaction V2 returned invalid output (reason=%s; added=%d; done=%d; terminal=%d)",
		reason, r.diagnostic.AddedItems, r.diagnostic.DoneItems, r.diagnostic.EndItems)
	errorBody := map[string]any{"type": "server_error", "code": "invalid_compaction_output", "message": message, "status_code": http.StatusBadGateway}
	if len(r.bareError) > 0 {
		if raw := gjson.GetBytes(r.bareError, "error"); raw.IsObject() {
			errorBody = make(map[string]any)
			_ = json.Unmarshal([]byte(raw.Raw), &errorBody)
		} else {
			errorBody["code"] = gjson.GetBytes(r.bareError, "code").String()
			errorBody["message"] = ExtractOpenAISSEErrorMessage(r.bareError)
		}
		errorBody["status_code"] = OpenAIStreamFailureStatus(r.bareError, ExtractOpenAISSEErrorMessage(r.bareError))
	}
	response := map[string]any{"id": r.responseID, "status": "failed", "output": []any{}, "error": errorBody}
	if len(r.usage) > 0 {
		response["usage"] = r.usage
	}
	data, _ := json.Marshal(map[string]any{"type": "response.failed", "response": response})
	return r.event(data, "response.failed")
}

// event 为下发事件分配递增序号。
func (r *nativeCompactionBody) event(data []byte, event string) []byte {
	data, _ = sjson.SetBytes(data, "sequence_number", r.sequence)
	r.sequence++
	return []byte("event: " + event + "\ndata: " + string(data) + "\n\n")
}

// report 汇总各阶段的压缩项数量和处理结果。
func (r *nativeCompactionBody) report(reason string) {
	r.diagnostic.Reason = reason
	for _, items := range []struct {
		values map[int]json.RawMessage
		count  *int
	}{{r.added, &r.diagnostic.AddedItems}, {r.done, &r.diagnostic.DoneItems}} {
		*items.count = 0
		for _, item := range items.values {
			if wire.IsResponsesCompactionItemType(gjson.GetBytes(item, "type").String()) {
				(*items.count)++
			}
		}
	}
	if r.observe != nil {
		r.observe(r.diagnostic)
	}
}

// sortedCompactionIndexes 按上游 output_index 排列结果项。
func sortedCompactionIndexes(items map[int]json.RawMessage) []int {
	indexes := make([]int, 0, len(items))
	for index := range items {
		indexes = append(indexes, index)
	}
	slices.Sort(indexes)
	return indexes
}

// countCompactionItems 统计两种压缩项名称对应的结果数量。
func countCompactionItems(items []json.RawMessage) int {
	count := 0
	for _, item := range items {
		if wire.IsResponsesCompactionItemType(gjson.GetBytes(item, "type").String()) {
			count++
		}
	}
	return count
}
