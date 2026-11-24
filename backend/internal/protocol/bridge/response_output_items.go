package bridge

import (
	"encoding/json"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type (
	outputIdentity struct {
		kind string
		id   string
	}
	outputSlot struct {
		index int64
		key   outputIdentity
	}
	observedOutput struct {
		slot outputSlot
		raw  json.RawMessage
	}
	toolNumber struct {
		index    int64
		value    string
		conflict bool
	}

	// ResponsesStreamOutputItems 在单次响应内收集工具身份和各项的最终内容。
	ResponsesStreamOutputItems struct {
		items     []observedOutput
		positions map[outputIdentity]int
		calls     map[outputIdentity]toolNumber
		slots     map[int64]outputIdentity
		conflicts int
	}
)

func NewResponsesStreamOutputItems() *ResponsesStreamOutputItems {
	return &ResponsesStreamOutputItems{}
}

func identityOf(item gjson.Result) outputIdentity {
	return outputIdentity{kind: item.Get("type").Str, id: item.Get("id").Str}
}

func (key outputIdentity) tool() bool {
	return key.id != "" && (key.kind == "function_call" || key.kind == "custom_tool_call" || key.kind == "mcp_tool_call" || key.kind == "tool_call")
}

func outputIndex(data gjson.Result) int64 {
	number := data.Get("output_index")
	if number.Type == gjson.Number {
		if n, err := strconv.ParseUint(number.Raw, 10, 32); err == nil {
			return int64(n)
		}
	}
	return -1
}

// Observe 记录工具的 added 和 done 事件，返回需要替换的 done 帧，其他情况返回 nil。
func (r *ResponsesStreamOutputItems) Observe(data []byte) []byte {
	kind := gjson.GetBytes(data, "type").Str
	if r == nil || (kind != "response.output_item.added" && kind != "response.output_item.done") {
		return nil
	}
	event := gjson.ParseBytes(data)
	item := event.Get("item")
	if !item.IsObject() {
		return nil
	}
	slot := outputSlot{index: outputIndex(event), key: identityOf(item)}
	var repaired []byte
	if slot.key.tool() {
		id := r.observeNumber(slot, item.Get("call_id"))
		if kind == "response.output_item.done" && id != "" {
			if patched, err := sjson.SetBytes(data, "item.call_id", id); err == nil {
				repaired = patched
				item = gjson.GetBytes(patched, "item")
			}
		}
	}
	if kind == "response.output_item.done" {
		if r.positions == nil {
			r.positions = make(map[outputIdentity]int)
		}
		raw := json.RawMessage(item.Raw)
		if pos, ok := r.positions[slot.key]; ok && slot.key.id != "" {
			r.items[pos].raw = raw
		} else {
			r.positions[slot.key] = len(r.items)
			r.items = append(r.items, observedOutput{slot: slot, raw: raw})
		}
	}
	return repaired
}

// observeNumber 在位置和工具身份一致时，返回可用于补齐空字段的编号。
func (r *ResponsesStreamOutputItems) observeNumber(slot outputSlot, id gjson.Result) string {
	if r.calls == nil {
		r.calls = make(map[outputIdentity]toolNumber)
		r.slots = make(map[int64]outputIdentity)
	}
	known, seen := r.calls[slot.key]
	if slot.index >= 0 {
		if old, exists := r.slots[slot.index]; exists && old != slot.key {
			r.markConflict(old)
			r.markConflict(slot.key)
			return ""
		}
		r.slots[slot.index] = slot.key
	}
	if seen && known.conflict {
		return ""
	}
	value := strings.TrimSpace(id.Str)
	if seen && (known.index != slot.index || (value != "" && known.value != "" && id.Str != known.value)) {
		r.markConflict(slot.key)
		return ""
	}
	if value != "" {
		r.calls[slot.key] = toolNumber{index: slot.index, value: id.Str}
		return ""
	}
	if !seen {
		r.calls[slot.key] = toolNumber{index: slot.index}
	}
	if slot.index >= 0 && (id.Type == gjson.Null || id.Type == gjson.String) {
		return known.value
	}
	return ""
}

func (r *ResponsesStreamOutputItems) markConflict(key outputIdentity) {
	known := r.calls[key]
	if !known.conflict {
		known.conflict = true
		r.calls[key] = known
		r.conflicts++
	}
}

func (r *ResponsesStreamOutputItems) HasItems() bool { return r.Count() > 0 }
func (r *ResponsesStreamOutputItems) Count() int {
	if r == nil {
		return 0
	}
	return len(r.items)
}

func (r *ResponsesStreamOutputItems) orderedItems() []observedOutput {
	ordered := slices.Clone(r.items)
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := ordered[i].slot.index, ordered[j].slot.index
		return a >= 0 && (b < 0 || a < b)
	})
	return ordered
}

func (r *ResponsesStreamOutputItems) BuildOutput() ([]byte, bool) {
	if !r.HasItems() {
		return nil, false
	}
	return r.mergeTerminalOutput(gjson.Result{})
}

// mergeTerminalOutput 按稳定身份合并部分终态，有效终态中的字段具有优先权。
func (r *ResponsesStreamOutputItems) mergeTerminalOutput(output gjson.Result) ([]byte, bool) {
	if r == nil || (len(r.items) == 0 && len(r.calls) == 0) {
		return nil, false
	}
	final := output.Array()
	if len(r.calls) == 0 && len(final) >= len(r.items) {
		return nil, false
	}
	byID := make(map[outputIdentity]int, len(final))
	fields := make([]json.RawMessage, 0, len(final))
	changed := false
	for i, item := range final {
		key := identityOf(item)
		if _, seen := byID[key]; seen {
			byID[key] = -1
		} else {
			byID[key] = i
		}
		raw := json.RawMessage(item.Raw)
		fields = append(fields, raw)
	}
	for key, pos := range byID {
		call := r.calls[key]
		if pos < 0 || call.value == "" || call.conflict {
			continue
		}
		id := final[pos].Get("call_id")
		if id.Type != gjson.Null && (id.Type != gjson.String || strings.TrimSpace(id.Str) != "") {
			if id.Str != "" && id.Str != call.value {
				r.markConflict(key)
			}
			continue
		}
		raw, err := sjson.SetBytes(fields[pos], "call_id", call.value)
		if err == nil {
			fields[pos] = raw
			changed = true
		}
	}
	if len(final) >= len(r.items) {
		if !changed {
			return nil, false
		}
		data, err := json.Marshal(fields)
		return data, err == nil
	}
	seen := make(map[outputIdentity]int, len(r.items))
	for _, done := range r.items {
		seen[done.slot.key]++
	}
	merged := make([]json.RawMessage, 0, len(r.items))
	used := make([]bool, len(final))
	// 某些中转会在截断终态中用新 id 重复已经下发的完整文本。
	texts := make(map[string]int)
	for _, done := range r.orderedItems() {
		raw := done.raw
		key := done.slot.key
		if key.kind == "message" {
			texts[messageFingerprint(raw)]++
		}
		if pos, ok := byID[key]; ok && pos >= 0 && key.id != "" && seen[key] == 1 {
			used[pos] = true
			raw = mergeOutputFields(fields[pos], raw, r.calls[key].conflict)
		}
		merged = append(merged, raw)
	}
	for i, item := range final {
		if used[i] {
			continue
		}
		if item.Get("type").Str == "message" {
			sig := messageFingerprint(fields[i])
			if texts[sig] > 0 {
				texts[sig]--
				continue
			}
		}
		merged = append(merged, fields[i])
	}
	data, err := json.Marshal(merged)
	return data, err == nil
}

// mergeOutputFields 补齐终态工具项的扩展字段，冲突编号由终态决定。
func mergeOutputFields(final, done json.RawMessage, conflict bool) json.RawMessage {
	var result, other map[string]json.RawMessage
	if json.Unmarshal(final, &result) != nil || json.Unmarshal(done, &other) != nil {
		return final
	}
	for key, value := range other {
		if _, ok := result[key]; !ok && (key != "call_id" || !conflict) {
			result[key] = value
		}
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return final
	}
	return raw
}

// messageFingerprint 比较截断终态中重建为新 id 的消息，扩展字段仍参与比较。
func messageFingerprint(raw json.RawMessage) string {
	var msg map[string]json.RawMessage
	if json.Unmarshal(raw, &msg) != nil {
		return string(raw)
	}
	delete(msg, "id")
	key, _ := json.Marshal(msg)
	return string(key)
}

// ConflictCount 统计本次响应中观察到的工具编号或输出位置冲突。
func (r *ResponsesStreamOutputItems) ConflictCount() int { return r.conflicts }
