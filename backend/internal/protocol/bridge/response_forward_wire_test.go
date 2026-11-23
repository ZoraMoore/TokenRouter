package bridge

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestNormalizeResponsesStreamingTerminalOutputPreservesReportedItems 检查终止事件的空 output 由流中报告的 item 重建。
// 累积器只建模一个 reasoning 和一个 message，经其重建会把多 item 回合压缩成
// 单个伪造 message。
func TestNormalizeResponsesStreamingTerminalOutputPreservesReportedItems(t *testing.T) {
	doneItems := NewResponsesStreamOutputItems()

	doneItems.Observe([]byte(`{
		"type":"response.output_item.done",
		"output_index":0,
		"item":{"id":"rs_1","type":"reasoning","summary":[{"type":"summary_text","text":"thinking"}],"encrypted_content":"opaque"}
	}`))
	doneItems.Observe([]byte(`{
		"type":"response.output_item.done",
		"output_index":1,
		"item":{"id":"msg_1","type":"message","status":"completed","phase":"final_answer","role":"assistant","content":[{"type":"output_text","text":"shipped","annotations":[],"logprobs":[]}]}
	}`))

	normalized, changed := NormalizeResponsesStreamingTerminalOutput(
		[]byte(`{"type":"response.completed","response":{"status":"completed","output":[]}}`),
		nil,
		doneItems,
		nil,
	)
	require.True(t, changed)

	output := gjson.GetBytes(normalized, "response.output")
	require.True(t, output.IsArray())
	require.Len(t, output.Array(), 2, "both reported items must survive")

	require.Equal(t, "reasoning", gjson.GetBytes(normalized, "response.output.0.type").String())
	require.Equal(t, "rs_1", gjson.GetBytes(normalized, "response.output.0.id").String())
	require.Equal(t, "opaque", gjson.GetBytes(normalized, "response.output.0.encrypted_content").String(),
		"fields the gateway does not model must survive verbatim")

	require.Equal(t, "message", gjson.GetBytes(normalized, "response.output.1.type").String())
	require.Equal(t, "msg_1", gjson.GetBytes(normalized, "response.output.1.id").String(),
		"the reported id must be reused, not regenerated")
	require.Equal(t, "completed", gjson.GetBytes(normalized, "response.output.1.status").String())
	require.Equal(t, "final_answer", gjson.GetBytes(normalized, "response.output.1.phase").String())
	require.Equal(t, "shipped", gjson.GetBytes(normalized, "response.output.1.content.0.text").String())
}

// TestResponsesStreamOutputItemsOrderByOutputIndex 检查乱序到达的 item 按 output_index 输出。
func TestResponsesStreamOutputItemsOrderByOutputIndex(t *testing.T) {
	doneItems := NewResponsesStreamOutputItems()
	doneItems.Observe([]byte(`{"type":"response.output_item.done","output_index":2,"item":{"id":"c","type":"message"}}`))
	doneItems.Observe([]byte(`{"type":"response.output_item.done","output_index":0,"item":{"id":"a","type":"reasoning"}}`))

	built, ok := doneItems.BuildOutput()
	require.True(t, ok)
	require.Equal(t, "a", gjson.GetBytes(built, "0.id").String())
	require.Equal(t, "c", gjson.GetBytes(built, "1.id").String())
}

// TestNormalizeResponsesStreamingTerminalOutputIgnoresNonDoneEvents 检查没有 done item 的流使用增量重建。
func TestNormalizeResponsesStreamingTerminalOutputIgnoresNonDoneEvents(t *testing.T) {
	doneItems := NewResponsesStreamOutputItems()
	doneItems.Observe([]byte(`{"type":"response.output_item.added","output_index":0,"item":{"id":"msg_1","type":"message"}}`))
	doneItems.Observe([]byte(`{"type":"response.output_text.delta","output_index":0,"delta":"hi"}`))
	require.False(t, doneItems.HasItems())

	raw := []byte(`{"type":"response.completed","response":{"status":"completed","output":[]}}`)
	normalized, changed := NormalizeResponsesStreamingTerminalOutput(raw, nil, doneItems, nil)
	require.False(t, changed)
	require.Equal(t, string(raw), string(normalized))
}

// TestNormalizeResponsesStreamingTerminalOutputRepairsTruncatedOutput 验证终止事件可能带有非空但被截断的 output：流报告了两个 item，终止事件只带一个，
// 且其 id 不是流中报告的 id。此时以已报告 item 为准。
func TestNormalizeResponsesStreamingTerminalOutputRepairsTruncatedOutput(t *testing.T) {
	doneItems := NewResponsesStreamOutputItems()
	doneItems.Observe([]byte(`{
		"type":"response.output_item.done","output_index":0,
		"item":{"id":"rs_real","type":"reasoning","status":"in_progress","summary":[]}
	}`))
	doneItems.Observe([]byte(`{
		"type":"response.output_item.done","output_index":1,
		"item":{"id":"msg_real","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"shipped","annotations":[],"logprobs":[]}]}
	}`))

	normalized, changed := NormalizeResponsesStreamingTerminalOutput([]byte(`{
		"type":"response.completed",
		"response":{"status":"completed","output":[{"type":"message","role":"assistant","id":"msg_fabricated","status":"completed","content":[{"type":"output_text","text":"shipped","annotations":[],"logprobs":[]}]}]}
	}`), nil, doneItems, nil)
	require.True(t, changed)

	require.Len(t, gjson.GetBytes(normalized, "response.output").Array(), 2)
	require.Equal(t, "reasoning", gjson.GetBytes(normalized, "response.output.0.type").String())
	require.Equal(t, "rs_real", gjson.GetBytes(normalized, "response.output.0.id").String())
	require.Equal(t, "msg_real", gjson.GetBytes(normalized, "response.output.1.id").String(),
		"the id the stream reported must replace the fabricated one")
}

// TestNormalizeResponsesStreamingTerminalOutputLeavesCompleteOutputAlone 验证已完整的终止 output 不应被重写。
func TestNormalizeResponsesStreamingTerminalOutputLeavesCompleteOutputAlone(t *testing.T) {
	doneItems := NewResponsesStreamOutputItems()
	doneItems.Observe([]byte(`{
		"type":"response.output_item.done","output_index":0,
		"item":{"id":"msg_real","type":"message","status":"completed"}
	}`))

	raw := []byte(`{"type":"response.completed","response":{"status":"completed","output":[{"type":"message","id":"msg_upstream","status":"completed","vendor":"keep"}]}}`)
	normalized, changed := NormalizeResponsesStreamingTerminalOutput(raw, nil, doneItems, nil)
	require.False(t, changed)
	require.Equal(t, string(raw), string(normalized))
}

func TestResponsesTerminalMergeKeepsToolIdentity(t *testing.T) {
	done := NewResponsesStreamOutputItems()
	done.Observe([]byte(`{"type":"response.output_item.done","output_index":1,"item":{"type":"function_call","id":"fc_a","name":"lookup","arguments":"{}"}}`))
	done.Observe([]byte(`{"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","id":"rs_x","encrypted_content":"opaque"}}`))
	got, changed := NormalizeResponsesStreamingTerminalOutput([]byte(`{"type":"response.completed","response":{"output":[{"type":"function_call","id":"fc_a","call_id":"call_a","vendor":42}]}}`), nil, done, nil)
	require.True(t, changed)
	require.Equal(t, "call_a", gjson.GetBytes(got, "response.output.1.call_id").String())
	require.Equal(t, "42", gjson.GetBytes(got, "response.output.1.vendor").Raw)
	require.Equal(t, "opaque", gjson.GetBytes(got, "response.output.0.encrypted_content").String())
}

func TestResponseOutputDoneInheritsOnlyMatchingAddedCallID(t *testing.T) {
	for _, tt := range []struct{ name, added, done, want string }{
		{"same", "fc_a", "fc_a", "call_a"},
		{"different", "fc_a", "fc_b", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			items := NewResponsesStreamOutputItems()
			items.Observe([]byte(`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"` + tt.added + `","name":"read","call_id":"call_a"}}`))
			items.Observe([]byte(`{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"` + tt.done + `","name":"read"}}`))
			body, ok := items.BuildOutput()
			require.True(t, ok)
			require.Equal(t, tt.want, gjson.GetBytes(body, "0.call_id").String())
		})
	}
}

func TestResponseOutputMergePreservesParallelAndConflictingIDs(t *testing.T) {
	items := NewResponsesStreamOutputItems()
	items.Observe([]byte(`{"type":"response.output_item.done","output_index":1,"item":{"type":"function_call","id":"fc_b","call_id":"call_b","name":"read"}}`))
	items.Observe([]byte(`{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"fc_a","call_id":"call_a","name":"read"}}`))
	body, changed := NormalizeResponsesStreamingTerminalOutput([]byte(`{"type":"response.completed","response":{"output":[{"type":"function_call","id":"fc_a","call_id":"final_a","name":"read"},{"type":"function_call","id":"fc_b","name":"read"}]}}`), nil, items, nil)
	require.True(t, changed)
	require.Equal(t, "final_a", gjson.GetBytes(body, "response.output.0.call_id").String())
	require.Equal(t, "call_b", gjson.GetBytes(body, "response.output.1.call_id").String())
}

func TestBufferedToolNumbersStayWithStableItems(t *testing.T) {
	const frames = "data: " + `{"type":"response.output_item.added","output_index":1,"item":{"id":"fc_a","type":"function_call","call_id":"call_a"}}` + "\n\n" +
		"data: " + `{"type":"response.output_item.done","output_index":1,"item":{"id":"fc_a","type":"function_call"}}` + "\n\n" +
		"data: " + `{"type":"response.output_item.done","output_index":0,"item":{"id":"fc_b","type":"function_call","call_id":"call_b"}}` + "\n\n"
	body := RepairBufferedResponsesOutput([]byte(`{"id":"resp_test","output":[{"type":"function_call","id":"fc_b","call_id":"final_b","vendor":1}]}`), frames)
	require.Equal(t, "final_b", gjson.GetBytes(body, "output.0.call_id").Str)
	require.Equal(t, "call_a", gjson.GetBytes(body, "output.1.call_id").Str)
	empty := RepairBufferedResponsesOutput([]byte(`{"id":"resp_test","output":[]}`), frames)
	require.Equal(t, "fc_b", gjson.GetBytes(empty, "output.0.id").Str)
	require.Equal(t, "call_a", gjson.GetBytes(empty, "output.1.call_id").Str)
}

func TestResponsesConflictingOrUnknownIndexNeverSuppliesCallID(t *testing.T) {
	items := NewResponsesStreamOutputItems()
	items.Observe([]byte(`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"a","call_id":"call_a","name":"same"}}`))
	items.Observe([]byte(`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"b","call_id":"call_b","name":"same"}}`))
	require.Nil(t, items.Observe([]byte(`{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"a","name":"same"}}`)))
	require.Nil(t, items.Observe([]byte(`{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"b","name":"same"}}`)))
	require.Positive(t, items.ConflictCount())
	items.Observe([]byte(`{"type":"response.output_item.added","item":{"type":"function_call","id":"c","call_id":"call_c"}}`))
	require.Nil(t, items.Observe([]byte(`{"type":"response.output_item.done","item":{"type":"function_call","id":"c"}}`)))
	items.Observe([]byte(`{"type":"response.output_item.done","item":{"type":"function_call","id":"c"}}`))
	raw, ok := items.BuildOutput()
	require.True(t, ok)
	parsed := gjson.ParseBytes(raw).Array()
	require.Len(t, parsed, 3)
	for _, item := range parsed {
		require.False(t, item.Get("call_id").Exists())
	}
}
