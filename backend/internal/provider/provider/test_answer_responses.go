package provider

import (
	"strconv"

	"github.com/tidwall/gjson"

	"github.com/TokenFlux/TokenRouter/internal/provider"
)

// drawingResponse 记录当前消息和文字段在缓冲区的起点。上游会在每一层结束时重发已经输出的文字。
type drawingResponse struct {
	item, part           int64
	itemStart, partStart int
	itemDone, partDone   bool
	contentPartDone      bool
	partSeen             bool
	ambiguous            bool
}

// readPart 用整段快照替换该段的增量。部分兼容端省略索引，按送达顺序逐段结束。
func (s *drawingResponse) readPart(text *answerText, frame, field gjson.Result, eventType string) error {
	if field.Type != gjson.String {
		return provider.ErrTestAnswerIncomplete
	}
	if !s.selectItem(text, frame) {
		return nil
	}
	contentPart := eventType == "response.content_part.done"
	next := s.part
	// 同一段会先发 output_text.done，再由 content_part.done 重发。之后的无索引快照属于下一段。
	if s.partDone && (!contentPart || s.contentPartDone) {
		next++
	}
	part, ok := drawingIndex(frame.Get("content_index"), next)
	if !ok || s.itemDone || part < s.part || part > s.part+1 || (!s.partSeen && part != 0) {
		s.ambiguous = true
		return nil
	}
	s.partSeen = true
	if part != s.part {
		s.part, s.partStart, s.partDone = part, text.Len(), false
		s.contentPartDone = false
	}
	if eventType != "response.output_text.delta" {
		text.Truncate(s.partStart)
		s.partDone = true
		s.contentPartDone = contentPart
	} else if s.partDone {
		s.ambiguous = true
		return nil
	}
	text.addField(field)
	return nil
}

// readItem 用完成的消息替换该消息的分段快照，推理和工具条目留在上游报文里。
func (s *drawingResponse) readItem(text *answerText, frame gjson.Result) error {
	item := frame.Get("item")
	if item.Get("type").String() != "message" || !s.selectItem(text, frame) {
		return nil
	}
	_, err := drawingItemText(text, item, s.itemStart)
	s.itemDone = true
	s.partStart = s.itemStart
	return err
}

func (s *drawingResponse) selectItem(text *answerText, frame gjson.Result) bool {
	next := s.item
	if s.itemDone {
		next++
	}
	item, ok := drawingIndex(frame.Get("output_index"), next)
	if !ok || item < s.item {
		s.ambiguous = true
		return false
	}
	if item != s.item {
		s.item, s.part = item, 0
		s.itemStart, s.partStart = text.Len(), text.Len()
		s.itemDone, s.partDone, s.partSeen = false, false, false
		s.contentPartDone = false
	}
	return true
}

// drawingIndex 接受协议中的非负整数，索引缺省时使用当前段的位置。
func drawingIndex(field gjson.Result, fallback int64) (int64, bool) {
	if !field.Exists() {
		return fallback, true
	}
	if field.Type != gjson.Number {
		return 0, false
	}
	index, err := strconv.ParseInt(field.Raw, 10, 32)
	return index, err == nil && index >= 0
}

// complete 优先读取终态中的作答。上游只发用量或推理摘要时，使用流中已经收齐的文字。
func (s *drawingResponse) complete(text *answerText, response gjson.Result) error {
	output := response.Get("output")
	if output.Type != gjson.Null && !output.IsArray() && !output.IsObject() {
		return provider.ErrTestAnswerIncomplete
	}
	offset := 0
	var found bool
	var err error
	forEachAnswerPart(output, func(item gjson.Result) bool {
		var visible bool
		visible, err = drawingItemText(text, item, offset)
		if visible {
			found = true
			offset = text.Len()
		}
		return err == nil
	})
	if !found && s.ambiguous && err == nil {
		return provider.ErrTestAnswerIncomplete
	}
	return err
}

// drawingItemText 在找到首个 output_text 时才替换文字，空字符串也属于上游给出的作答。
func drawingItemText(text *answerText, item gjson.Result, offset int) (bool, error) {
	if kind := item.Get("type").String(); kind != "" && kind != "message" {
		return false, nil
	}
	if status := item.Get("status").String(); status != "" && status != "completed" {
		return false, provider.ErrTestAnswerIncomplete
	}
	content := item.Get("content")
	if content.Type != gjson.Null && !content.IsArray() && !content.IsObject() {
		return false, provider.ErrTestAnswerIncomplete
	}
	var found bool
	var err error
	forEachAnswerPart(content, func(part gjson.Result) bool {
		switch part.Get("type").String() {
		case "refusal":
			err = provider.ErrTestAnswerIncomplete
		case "output_text":
			field := part.Get("text")
			if field.Type != gjson.String {
				err = provider.ErrTestAnswerIncomplete
				return false
			}
			if !found {
				text.Truncate(offset)
				found = true
			}
			text.addField(field)
			if text.overflow {
				err = provider.ErrTestAnswerLimit
			}
		}
		return err == nil
	})
	return found, err
}
