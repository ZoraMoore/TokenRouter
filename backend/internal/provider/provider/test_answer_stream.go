package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"

	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/upstream/qoder"
)

const (
	// 推理模型的思考与最终文字共用输出预算。
	pelicanMaxTokens = 32000

	answerClaude    answerProtocol = "messages"
	answerBedrock   answerProtocol = "bedrock"
	answerGemini    answerProtocol = "gemini"
	answerResponses answerProtocol = "responses"
	answerChat      answerProtocol = "chat_completions"
	answerQoder     answerProtocol = "qoder"
)

var errAnswerUpstream = errors.New("测智请求失败")

type answerProtocol string

// answerReadLimits 分别限制整条传输、单条报文、已提取的文字和最终回答的字节数。
type answerReadLimits struct {
	wire, frame, capture, output int
	drawing                      bool
}

// answerText 按协议拼接可见文字，超限时整份回答作废。
type answerText struct {
	bytes.Buffer
	limit    int
	overflow bool
	drawing  *drawingResponse
}

func testAnswerLimits(svg bool, mode string) answerReadLimits {
	if mode == "modeltrace" {
		return answerReadLimits{wire: 4 << 20, frame: 4 << 20, capture: 64 << 10, output: 64 << 10}
	}
	if svg {
		// 一轮推理可以产生许多事件，最终报文还可能重发 SVG 和推理记录。
		return answerReadLimits{wire: 32 << 20, frame: 8 << 20, capture: 4 << 20, output: provider.PelicanDrawingLimit, drawing: true}
	}
	return answerReadLimits{wire: 1 << 20, frame: 1 << 20, capture: provider.PelicanAnswerLimit, output: provider.PelicanAnswerLimit}
}

// readAnswer 将一次已完成的模型回答交给测智服务，错误响应的正文不进入采集器。
func (r *TestRun) readAnswer(resp *http.Response, protocol answerProtocol, sent []byte) error {
	return r.readAnswerForModel(resp, protocol, sent, gjson.GetBytes(sent, "model").String())
}

// readAnswerForModel 记录已收到回复的请求。部分协议把模型写在 URL 或请求头。
func (r *TestRun) readAnswerForModel(resp *http.Response, protocol answerProtocol, sent []byte, model string) error {
	if resp != nil {
		request := provider.TestEvent{Type: "test_request", Model: model, Protocol: string(protocol), ReasoningEffort: sentAnswerEffort(protocol, sent)}
		if r.IQMode == "modeltrace" {
			request.SystemPrompt = "none"
			switch {
			case protocol == answerResponses && gjson.GetBytes(sent, "instructions").Exists():
				request.SystemPrompt = "required_responses_instructions"
			case protocol == answerClaude && gjson.GetBytes(sent, "system").Exists():
				request.SystemPrompt = "required_claude_code"
			}
		}
		if err := r.Emit(request); err != nil {
			return err
		}
		if r.ReasoningEffort != "" && resp.Body != nil && (resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnprocessableEntity) {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
			return answerRequestError(gjson.GetBytes(body, "error"))
		}
	}
	text, err := completedTestAnswer(r.Context, resp, protocol, testAnswerLimits(r.SVGAnswer, r.IQMode))
	if err != nil {
		return err
	}
	if text != "" {
		if err = r.Emit(provider.TestEvent{Type: "content", Text: text}); err != nil {
			return err
		}
	}
	if r.SuppressCompletion() {
		return nil
	}
	return r.Emit(provider.TestEvent{Type: "test_complete", Success: true})
}

func (r *TestRun) tokenBudget(probeLimit int) int {
	if r.FullAnswer {
		return pelicanMaxTokens
	}
	return probeLimit
}

// completedTestAnswer 在上游协议的完成事件或完整 JSON 中取出最终文字。
func completedTestAnswer(ctx context.Context, resp *http.Response, protocol answerProtocol, limits answerReadLimits) (string, error) {
	if resp == nil || resp.StatusCode != http.StatusOK || resp.Body == nil {
		return "", errAnswerUpstream
	}
	contentType, _, _ := strings.Cut(resp.Header.Get("Content-Type"), ";")
	if protocol == answerBedrock || strings.EqualFold(strings.TrimSpace(contentType), "application/json") {
		reader := &io.LimitedReader{R: resp.Body, N: int64(min(limits.wire, limits.frame)) + 1}
		return completedJSONAnswer(ctx, reader, protocol, limits)
	}
	reader := &io.LimitedReader{R: resp.Body, N: int64(limits.wire) + 1}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), limits.frame+2)
	// Scanner 会预读后续事件。总量按已消费的行计算，包含 CRLF 和注释。
	remaining := limits.wire
	scanner.Split(func(data []byte, atEOF bool) (int, []byte, error) {
		advance, token, err := bufio.ScanLines(data, atEOF)
		if advance > remaining {
			return 0, nil, provider.ErrTestAnswerLimit
		}
		remaining -= advance
		return advance, token, err
	})
	text := answerText{limit: limits.capture}
	if limits.drawing && protocol == answerResponses {
		text.drawing = &drawingResponse{}
	}
	for scanner.Scan() {
		if len(scanner.Bytes()) > limits.frame {
			return "", provider.ErrTestAnswerLimit
		}
		if ctx.Err() != nil {
			return "", provider.ErrTestAnswerIncomplete
		}
		line := bytes.TrimSpace(scanner.Bytes())
		data, found := bytes.CutPrefix(line, []byte("data:"))
		if !found {
			continue
		}
		data = bytes.TrimSpace(data)
		if bytes.Equal(data, []byte("[DONE]")) {
			if protocol == answerChat || protocol == answerQoder {
				return text.result(limits.output)
			}
			return "", provider.ErrTestAnswerIncomplete
		}
		if !json.Valid(data) {
			return "", provider.ErrTestAnswerIncomplete
		}
		done, err := parseAnswerFrame(protocol, gjson.ParseBytes(data), &text)
		if err != nil {
			return "", err
		}
		if text.overflow {
			return "", provider.ErrTestAnswerLimit
		}
		if done {
			return text.result(limits.output)
		}
	}
	if reader.N == 0 || errors.Is(scanner.Err(), bufio.ErrTooLong) {
		return "", provider.ErrTestAnswerLimit
	}
	return "", provider.ErrTestAnswerIncomplete
}

// completedJSONAnswer 接受 Bedrock invoke 和兼容上游返回的非流式成品。
func completedJSONAnswer(ctx context.Context, reader *io.LimitedReader, protocol answerProtocol, limits answerReadLimits) (string, error) {
	data, err := io.ReadAll(reader)
	if reader.N == 0 {
		return "", provider.ErrTestAnswerLimit
	}
	if err != nil || ctx.Err() != nil || !json.Valid(data) {
		return "", provider.ErrTestAnswerIncomplete
	}
	message := gjson.ParseBytes(data)
	if message.Get("error").Type != gjson.Null || message.Get("type").String() == "error" {
		return "", answerRequestError(message.Get("error"))
	}
	text := answerText{limit: limits.output}
	switch protocol {
	case answerBedrock, answerClaude:
		if !claudeAnswerFinished(message.Get("stop_reason").String()) {
			return "", provider.ErrTestAnswerIncomplete
		}
		forEachAnswerPart(message.Get("content"), func(part gjson.Result) bool {
			if part.Get("type").String() == "text" {
				text.addField(part.Get("text"))
			}
			return !text.overflow
		})
	case answerResponses:
		if message.Get("status").String() != "completed" {
			return "", provider.ErrTestAnswerIncomplete
		}
		if limits.drawing {
			if err := (&drawingResponse{}).complete(&text, message); err != nil {
				return "", err
			}
		} else {
			appendResponseOutput(&text, message)
		}
	default:
		done, err := parseAnswerFrame(protocol, message, &text)
		if err != nil {
			return "", err
		}
		if !done {
			return "", provider.ErrTestAnswerIncomplete
		}
	}
	return text.result(limits.output)
}

func parseAnswerFrame(protocol answerProtocol, data gjson.Result, text *answerText) (bool, error) {
	if data.Get("error").Type != gjson.Null {
		return false, answerRequestError(data.Get("error"))
	}
	switch protocol {
	case answerClaude:
		switch data.Get("type").String() {
		case "content_block_start":
			if data.Get("content_block.type").String() == "text" {
				text.addField(data.Get("content_block.text"))
			}
		case "content_block_delta":
			if kind := data.Get("delta.type").String(); kind == "" || kind == "text_delta" {
				text.addField(data.Get("delta.text"))
			}
		case "message_delta":
			if reason := data.Get("delta.stop_reason").String(); reason != "" && !claudeAnswerFinished(reason) {
				return false, provider.ErrTestAnswerIncomplete
			}
		case "message_stop":
			return true, nil
		case "error":
			return false, errAnswerUpstream
		}
	case answerGemini:
		if wrapped := data.Get("response"); wrapped.Exists() {
			data = wrapped
		}
		if data.Get("error").Type != gjson.Null || data.Get("promptFeedback.blockReason").String() != "" {
			return false, errAnswerUpstream
		}
		candidate := data.Get("candidates.0")
		forEachAnswerPart(candidate.Get("content.parts"), func(part gjson.Result) bool {
			if !part.Get("thought").Bool() {
				text.addField(part.Get("text"))
			}
			return !text.overflow
		})
		return answerFinishReason(candidate.Get("finishReason").String(), "STOP")
	case answerChat:
		choice := data.Get("choices.0")
		text.addField(choice.Get("delta.content"))
		if text.Len() == 0 {
			text.addField(choice.Get("message.content"))
		}
		return answerFinishReason(choice.Get("finish_reason").String(), "stop")
	case answerResponses:
		switch eventType := data.Get("type").String(); eventType {
		case "response.output_text.delta":
			if text.drawing != nil {
				return false, text.drawing.readPart(text, data, data.Get("delta"), eventType)
			}
			text.addField(data.Get("delta"))
		case "response.output_text.done":
			if text.drawing != nil {
				return false, text.drawing.readPart(text, data, data.Get("text"), eventType)
			}
			if text.Len() == 0 {
				text.addField(data.Get("text"))
			}
		case "response.content_part.done":
			if text.drawing != nil {
				switch data.Get("part.type").String() {
				case "refusal":
					return false, provider.ErrTestAnswerIncomplete
				case "output_text":
					return false, text.drawing.readPart(text, data, data.Get("part.text"), eventType)
				}
			}
		case "response.output_item.done":
			if text.drawing != nil {
				return false, text.drawing.readItem(text, data)
			}
		case "response.completed", "response.done":
			response := data.Get("response")
			if status := response.Get("status").String(); status != "" && status != "completed" {
				return false, provider.ErrTestAnswerIncomplete
			}
			if text.drawing != nil {
				return true, text.drawing.complete(text, response)
			}
			// 最终报文可能带有若干输出段，先前的单段 done 事件只包含其中一段。
			if response.Get("output").IsArray() {
				text.Reset()
				appendResponseOutput(text, response)
			}
			return true, nil
		case "response.incomplete", "response.refusal.done":
			return false, provider.ErrTestAnswerIncomplete
		case "response.refusal.delta":
			if text.drawing != nil {
				return false, provider.ErrTestAnswerIncomplete
			}
		case "response.failed":
			return false, answerRequestError(data.Get("response.error"))
		case "error":
			return false, answerRequestError(data)
		}
	case answerQoder:
		events, err := qoder.ParseSSELine("data: " + data.Raw)
		if err != nil {
			return false, errAnswerUpstream
		}
		// COSY 把 OpenAI 或 Claude 的结束原因装在 body 字符串中。
		body := gjson.Parse(data.Get("body").String())
		finished, err := answerFinishReason(body.Get("choices.0.finish_reason").String(), "stop")
		if err != nil {
			return false, err
		}
		if reason := body.Get("data.stop_reason").String(); reason != "" && !claudeAnswerFinished(reason) {
			return false, provider.ErrTestAnswerIncomplete
		}
		for _, event := range events {
			if event.Type == "text_delta" {
				text.add(event.Text)
			}
			finished = finished || event.IsDone
		}
		return finished, nil
	}
	return false, nil
}

func appendResponseOutput(text *answerText, response gjson.Result) {
	forEachAnswerPart(response.Get("output"), func(item gjson.Result) bool {
		forEachAnswerPart(item.Get("content"), func(part gjson.Result) bool {
			if part.Get("type").String() == "output_text" {
				text.addField(part.Get("text"))
			}
			return !text.overflow
		})
		return !text.overflow
	})
}

// forEachAnswerPart 逐段读取数组，单段对象也可直接作为兼容上游的回答。
func forEachAnswerPart(parts gjson.Result, visit func(gjson.Result) bool) {
	if parts.IsArray() {
		parts.ForEach(func(_, part gjson.Result) bool { return visit(part) })
	} else if parts.Type != gjson.Null {
		visit(parts)
	}
}

func (text *answerText) addField(field gjson.Result) {
	if field.Type == gjson.String {
		text.add(field.String())
	}
}

func (text *answerText) add(value string) {
	if len(value) > text.limit-text.Len() {
		text.overflow = true
		return
	}
	_, _ = text.WriteString(value)
}

func (text *answerText) result(limit int) (string, error) {
	if text.overflow || text.Len() > limit {
		return "", provider.ErrTestAnswerLimit
	}
	return text.String(), nil
}

func answerFinishReason(reason, expected string) (bool, error) {
	switch reason {
	case "":
		return false, nil
	case expected:
		return true, nil
	default:
		return false, provider.ErrTestAnswerIncomplete
	}
}

func claudeAnswerFinished(reason string) bool {
	return reason == "end_turn" || reason == "stop_sequence"
}
