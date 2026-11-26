package antigravity

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	logger "github.com/TokenFlux/TokenRouter/internal/infra/telemetry/logging"
	"github.com/TokenFlux/TokenRouter/internal/protocol/bridge"
	protocolgemini "github.com/TokenFlux/TokenRouter/internal/protocol/gemini"
	"github.com/TokenFlux/TokenRouter/internal/upstream"
)

type ResponseOptions struct {
	Context                                                         context.Context
	Candidates                                                      int
	ObserveRaw                                                      func([]byte)
	MaxLineSize, StreamDataIntervalTimeout, StreamKeepaliveInterval int
	ReverseTools                                                    func([]byte) []byte
	ClaudeError, CompatError                                        func(int, string, string) error
	MapCollectionError                                              func(error) error
	Failover                                                        func([]byte) error
	IsFailover                                                      func(error) bool
	MarkCommitted                                                   func()
}

type (
	ResponseAdapter struct{ Options ResponseOptions }
	StreamResult    struct {
		Usage            *upstream.TokenUsage
		FirstTokenMs     *int
		ClientDisconnect bool // 客户端是否在流式传输过程中断开
	}
)

// ClientWriter 封装流式响应的客户端写入，自动检测断开并标记。
// 断开后所有写入操作变为 no-op，调用方通过 Disconnected() 判断是否继续 drain 上游。
type ClientWriter struct {
	w                upstream.OutputWriter
	flusher          http.Flusher
	disconnected     bool
	prefix           string // 日志前缀，标识来源方法
	beforeFirstWrite func()
}

func NewClientWriter(w upstream.OutputWriter, flusher http.Flusher, prefix string) *ClientWriter {
	return &ClientWriter{w: w, flusher: flusher, prefix: prefix}
}

// Write 写入数据到客户端，写入失败时标记断开并返回 false。
func (cw *ClientWriter) Write(p []byte) bool {
	if cw.disconnected {
		return false
	}
	cw.prepareFirstWrite()
	if _, err := cw.w.Write(p); err != nil {
		cw.markDisconnected()
		return false
	}
	cw.flusher.Flush()
	return true
}

// Fprintf 格式化写入数据到客户端，写入失败时标记断开并返回 false。
func (cw *ClientWriter) Fprintf(format string, args ...any) bool {
	if cw.disconnected {
		return false
	}
	cw.prepareFirstWrite()
	if _, err := fmt.Fprintf(cw.w, format, args...); err != nil {
		cw.markDisconnected()
		return false
	}
	cw.flusher.Flush()
	return true
}

func (cw *ClientWriter) Disconnected() bool { return cw.disconnected }

func (cw *ClientWriter) prepareFirstWrite() {
	if cw.beforeFirstWrite == nil {
		return
	}
	prepare := cw.beforeFirstWrite
	cw.beforeFirstWrite = nil
	prepare()
}

func (cw *ClientWriter) markDisconnected() {
	cw.disconnected = true
	logger.LegacyPrintf("service.antigravity_gateway", "Client disconnected during streaming (%s), continuing to drain upstream for billing", cw.prefix)
}

// HandleStreamReadError 处理上游读取错误的通用逻辑。
// 返回 (clientDisconnect, handled)：handled=true 表示错误已处理，调用方应返回已收集的 usage。
func HandleStreamReadError(err error, clientDisconnected bool, prefix string) (disconnect bool, handled bool) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		logger.LegacyPrintf("service.antigravity_gateway", "Context canceled during streaming (%s), returning collected usage", prefix)
		return true, true
	}
	if clientDisconnected {
		logger.LegacyPrintf("service.antigravity_gateway", "Upstream read error after client disconnect (%s): %v, returning collected usage", prefix, err)
		return true, true
	}
	return false, false
}

func (s *ResponseAdapter) HandleGeminiStreamingResponse(c *upstream.OutputContext, resp *http.Response, startTime time.Time) (*StreamResult, error) {
	c.Status(resp.StatusCode)
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	result := &StreamResult{Usage: &upstream.TokenUsage{}}
	observed, err := s.readGeminiStream(resp, s.UnwrapV1InternalResponse, func(event upstream.GeminiStreamEvent) error {
		if len(event.Data) == 0 {
			if event.Terminal {
				c.Complete()
			}
			return nil
		}
		if result.FirstTokenMs == nil {
			ms := int(time.Since(startTime).Milliseconds())
			result.FirstTokenMs = &ms
		}
		s.observeRaw(event.Data)
		if usage := bridge.NativeUsageProjection(bridge.NativeExtractGeminiUsage(event.Data)); usage != nil {
			result.Usage = usage
		}
		if c.Err() != nil {
			return nil
		}
		c.NextEvent(protocolgemini.ObservePayload(event.Data).Semantic, event.Terminal)
		if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", event.Data); err != nil {
			return nil
		}
		c.Writer.Flush()
		return nil
	}, func() {
		if result.FirstTokenMs != nil && c.Err() == nil {
			_, _ = fmt.Fprint(c.Writer, "\n\n")
			c.Writer.Flush()
		}
	})
	if usage := bridge.NativeUsageProjection(bridge.NativeExtractGeminiUsage(observed.UsageJSON)); usage != nil {
		result.Usage = usage
	}
	result.ClientDisconnect = c.Err() != nil || errors.Is(err, context.Canceled) || (s.Options.Context != nil && s.Options.Context.Err() != nil)
	if result.ClientDisconnect {
		return result, nil
	}
	return result, err
}

// HandleGeminiStreamToNonStreaming 读取上游流式响应，合并为非流式响应返回给客户端
// Gemini 流式响应是增量的，需要累积所有 chunk 的内容。
func (s *ResponseAdapter) HandleGeminiStreamToNonStreaming(c *upstream.OutputContext, resp *http.Response, startTime time.Time) (*StreamResult, error) {
	var firstTokenMs *int
	observe := func(data []byte) {
		if firstTokenMs == nil {
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
		}
		s.observeRaw(data)
	}

	response, observed, err := upstream.CollectGeminiStream(s.Options.Context, resp, upstream.GeminiStreamOptions{
		IdleTimeout:   time.Duration(s.Options.StreamDataIntervalTimeout) * time.Second,
		MaxFrameBytes: s.Options.MaxLineSize, Candidates: s.Options.Candidates, Unwrap: s.UnwrapV1InternalResponse,
	}, observe)
	usage := bridge.NativeUsageProjection(bridge.NativeExtractGeminiUsage(observed.UsageJSON))
	if usage == nil {
		usage = &upstream.TokenUsage{}
	}
	result := &StreamResult{Usage: usage, FirstTokenMs: firstTokenMs}
	if err != nil {
		return result, err
	}
	body, err := json.Marshal(response)
	if err != nil {
		return result, err
	}
	c.Data(http.StatusOK, "application/json", body)
	return result, c.Err()
}

// GetOrCreateGeminiParts 获取 Gemini 响应的 parts 结构，返回深拷贝和更新回调。
func GetOrCreateGeminiParts(response map[string]any) (result map[string]any, existingParts []any, setParts func([]any)) {
	// 深拷贝 response
	result = make(map[string]any)
	maps.Copy(result, response)

	// 获取或创建 candidates
	candidates, ok := result["candidates"].([]any)
	if !ok || len(candidates) == 0 {
		candidates = []any{map[string]any{}}
	}

	// 获取第一个 candidate
	candidate, ok := candidates[0].(map[string]any)
	if !ok {
		candidate = make(map[string]any)
		candidates[0] = candidate
	}

	// 获取或创建 content
	content, ok := candidate["content"].(map[string]any)
	if !ok {
		content = map[string]any{"role": "model"}
		candidate["content"] = content
	}

	// 获取现有 parts
	existingParts, ok = content["parts"].([]any)
	if !ok {
		existingParts = []any{}
	}

	// 返回更新回调
	setParts = func(newParts []any) {
		content["parts"] = newParts
		result["candidates"] = candidates
	}

	return result, existingParts, setParts
}

// MergeCollectedPartsToResponse 将收集的所有 parts 合并到 Gemini 响应中
// 这个函数会合并所有类型的 parts：text、thinking、functionCall、inlineData 等
// 保持原始顺序，只合并连续的普通 text parts。
func MergeCollectedPartsToResponse(response map[string]any, collectedParts []map[string]any) map[string]any {
	if len(collectedParts) == 0 {
		return response
	}

	result, _, setParts := GetOrCreateGeminiParts(response)

	// 合并策略：
	// 1. 保持原始顺序
	// 2. 连续的普通 text parts 合并为一个
	// 3. thinking、functionCall、inlineData 等保持原样
	var mergedParts []any
	var textBuffer strings.Builder

	flushTextBuffer := func() {
		if textBuffer.Len() > 0 {
			mergedParts = append(mergedParts, map[string]any{
				"text": textBuffer.String(),
			})
			textBuffer.Reset()
		}
	}

	for _, part := range collectedParts {
		// 检查是否是普通 text part
		if text, ok := part["text"].(string); ok {
			// 检查是否有 thought 标记
			if thought, _ := part["thought"].(bool); thought {
				// thinking part，先刷新 text buffer，然后保留原样
				flushTextBuffer()
				mergedParts = append(mergedParts, part)
			} else {
				// 普通 text，累积到 buffer
				_, _ = textBuffer.WriteString(text)
			}
		} else {
			// 非 text part（functionCall、inlineData 等），先刷新 text buffer，然后保留原样
			flushTextBuffer()
			mergedParts = append(mergedParts, part)
		}
	}

	// 刷新剩余的 text
	flushTextBuffer()

	setParts(mergedParts)
	return result
}

// MergeImagePartsToResponse 将收集到的图片 parts 合并到 Gemini 响应中。
func MergeImagePartsToResponse(response map[string]any, imageParts []map[string]any) map[string]any {
	if len(imageParts) == 0 {
		return response
	}

	result, existingParts, setParts := GetOrCreateGeminiParts(response)

	// 检查现有 parts 中是否已经有图片
	for _, p := range existingParts {
		if pm, ok := p.(map[string]any); ok {
			if _, hasInline := pm["inlineData"]; hasInline {
				return result // 已有图片，不重复添加
			}
		}
	}

	// 添加收集到的图片 parts
	for _, imgPart := range imageParts {
		existingParts = append(existingParts, imgPart)
	}
	setParts(existingParts)
	return result
}

// MergeTextPartsToResponse 将收集到的文本合并到 Gemini 响应中。
func MergeTextPartsToResponse(response map[string]any, textParts []string) map[string]any {
	if len(textParts) == 0 {
		return response
	}

	mergedText := strings.Join(textParts, "")
	result, existingParts, setParts := GetOrCreateGeminiParts(response)

	// 查找并更新第一个 text part，或创建新的
	newParts := make([]any, 0, len(existingParts)+1)
	textUpdated := false

	for _, p := range existingParts {
		pm, ok := p.(map[string]any)
		if !ok {
			newParts = append(newParts, p)
			continue
		}
		if _, hasText := pm["text"]; hasText && !textUpdated {
			// 用累积的文本替换
			newPart := make(map[string]any)
			maps.Copy(newPart, pm)
			newPart["text"] = mergedText
			newParts = append(newParts, newPart)
			textUpdated = true
		} else {
			newParts = append(newParts, pm)
		}
	}

	if !textUpdated {
		newParts = append([]any{map[string]any{"text": mergedText}}, newParts...)
	}

	setParts(newParts)
	return result
}

// CollectClaudeStreamResponse 收集上游流式响应，转换为 Claude 非流式格式返回
// 用于处理客户端非流式请求但上游只支持流式的情况。
func (s *ResponseAdapter) CollectClaudeStreamResponse(resp *http.Response, startTime time.Time, originalModel string) ([]byte, *StreamResult, error) {
	var firstTokenMs *int
	observe := func(data []byte) {
		if firstTokenMs == nil {
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
		}
		s.observeRaw(data)
	}

	collected, observed, err := upstream.CollectGeminiStream(s.Options.Context, resp, upstream.GeminiStreamOptions{
		IdleTimeout:   time.Duration(s.Options.StreamDataIntervalTimeout) * time.Second,
		MaxFrameBytes: s.Options.MaxLineSize, Candidates: s.Options.Candidates,
		Unwrap: s.unwrapConvertedGeminiResponse,
	}, observe)
	usage := bridge.NativeUsageProjection(bridge.NativeExtractGeminiUsage(observed.UsageJSON))
	if usage == nil {
		usage = &upstream.TokenUsage{}
	}
	result := &StreamResult{Usage: usage, FirstTokenMs: firstTokenMs}
	if err != nil {
		if streamError, ok := errors.AsType[*upstream.GeminiStreamError](err); ok {
			switch streamError.Kind {
			case "stream_empty":
				return nil, result, s.Options.Failover([]byte(`{"error":"empty stream response from upstream"}`))
			case "stream_timeout":
				return nil, result, fmt.Errorf("stream data interval timeout: %w", err)
			case "stream_frame_too_large":
				return nil, result, fmt.Errorf("%w: %w", bufio.ErrTooLong, err)
			}
		}
		return nil, result, err
	}
	body, err := json.Marshal(collected)
	if err != nil {
		return nil, result, err
	}
	converted, _, err := TransformGeminiToClaude(body, originalModel)
	return converted, result, err
}

// HandleClaudeStreamToNonStreaming 收集上游流式响应，转换为 Claude 非流式格式返回
// 用于处理客户端非流式请求但上游只支持流式的情况。
func (s *ResponseAdapter) HandleClaudeStreamToNonStreaming(c *upstream.OutputContext, resp *http.Response, startTime time.Time, originalModel string) (*StreamResult, error) {
	claudeResp, streamRes, err := s.CollectClaudeStreamResponse(resp, startTime, originalModel)
	if err != nil {
		if s.Options.IsFailover(err) {
			return streamRes, err
		}

		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return streamRes, err
		}

		errMsg := "Failed to parse upstream response"
		errType := "upstream_error"
		if strings.Contains(err.Error(), "stream data interval timeout") {
			errMsg = "Upstream stream data interval timeout"
			errType = "upstream_timeout"
		} else if errors.Is(err, bufio.ErrTooLong) {
			errMsg = "Upstream response line too long"
			errType = "response_too_large"
		}

		return streamRes, s.Options.ClaudeError(http.StatusBadGateway, errType, errMsg)
	}
	c.Data(http.StatusOK, "application/json", claudeResp)
	return streamRes, nil
}

// HandleClaudeStreamingResponse 处理 Claude 流式响应（Gemini SSE → Claude SSE 转换）。
func (s *ResponseAdapter) HandleClaudeStreamingResponse(c *upstream.OutputContext, resp *http.Response, startTime time.Time, originalModel string) (*StreamResult, error) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}
	processor := NewStreamingProcessor(originalModel)
	writer := NewClientWriter(c.Writer, flusher, "antigravity claude")
	var firstTokenMs *int
	observed, err := s.readGeminiStream(resp, s.unwrapConvertedGeminiResponse, func(event upstream.GeminiStreamEvent) error {
		if len(event.Data) > 0 {
			if firstTokenMs == nil {
				ms := int(time.Since(startTime).Milliseconds())
				firstTokenMs = &ms
			}
			s.observeRaw(event.Data)
			writer.Write(processor.ProcessLine("data: " + string(event.Data)))
		}
		if event.Terminal {
			c.Complete()
		}
		return nil
	}, func() {
		if processor.MessageStartSent() && !writer.Disconnected() {
			writer.Write([]byte(": ping\n\n"))
		}
	})
	processor.SetUsageSnapshot(bridge.NativeExtractGeminiUsage(observed.UsageJSON))
	finalEvents, converted := processor.Finish()
	result := &StreamResult{Usage: &upstream.TokenUsage{
		InputTokens: converted.InputTokens, OutputTokens: converted.OutputTokens,
		CacheReadInputTokens:     converted.CacheReadInputTokens,
		CacheCreationInputTokens: converted.CacheCreationInputTokens,
		ImageOutputTokens:        converted.ImageOutputTokens,
	}, ClientDisconnect: writer.Disconnected(), FirstTokenMs: firstTokenMs}
	if err != nil {
		if disconnected, handled := HandleStreamReadError(err, writer.Disconnected(), "antigravity claude"); handled {
			result.ClientDisconnect = disconnected
			return result, nil
		}
		var streamError *upstream.GeminiStreamError
		if !processor.MessageStartSent() || (errors.As(err, &streamError) && streamError.Kind == "stream_empty") {
			return result, s.Options.Failover([]byte(`{"error":"empty stream response from upstream"}`))
		}
		reason := "stream_read_error"
		if errors.As(err, &streamError) {
			switch streamError.Kind {
			case "stream_frame_too_large":
				reason = "response_too_large"
			case "stream_timeout":
				reason = "stream_timeout"
			}
		}
		writer.Write([]byte(fmt.Sprintf("event: error\ndata: {\"error\":%q}\n\n", reason)))
		return result, err
	}
	writer.Write(finalEvents)
	result.ClientDisconnect = writer.Disconnected()
	return result, nil
}

func (s *ResponseAdapter) ExtractImageInputSize(body []byte) string {
	var req protocolgemini.GeminiRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return ""
	}

	if req.GenerationConfig != nil && req.GenerationConfig.ImageConfig != nil {
		return strings.TrimSpace(req.GenerationConfig.ImageConfig.ImageSize)
	}

	return ""
}

func IsImageGenerationModel(model string) bool { return upstream.IsGeminiImageGenerationModel(model) }

func (s *ResponseAdapter) observeRaw(data []byte) {
	if s.Options.ObserveRaw != nil {
		s.Options.ObserveRaw(data)
	}
}

// UnwrapV1InternalResponse 提取 v1internal 的 response JSON 字段，缺失时返回传入的响应体。
func (s *ResponseAdapter) UnwrapV1InternalResponse(body []byte) ([]byte, error) {
	result := gjson.GetBytes(body, "response")
	if result.Exists() {
		return []byte(result.Raw), nil
	}
	return body, nil
}

func (s *ResponseAdapter) readGeminiStream(resp *http.Response, unwrap func([]byte) ([]byte, error), consume func(upstream.GeminiStreamEvent) error, keepalive ...func()) (upstream.GeminiStreamResult, error) {
	options := upstream.GeminiStreamOptions{
		IdleTimeout:   time.Duration(s.Options.StreamDataIntervalTimeout) * time.Second,
		MaxFrameBytes: s.Options.MaxLineSize, Candidates: s.Options.Candidates, Unwrap: unwrap,
	}
	if len(keepalive) > 0 {
		options.Keepalive = keepalive[0]
		options.KeepaliveInterval = time.Duration(s.Options.StreamKeepaliveInterval) * time.Second
	}
	return upstream.ReadGeminiStream(s.Options.Context, resp, options, consume)
}

func (s *ResponseAdapter) unwrapConvertedGeminiResponse(body []byte) ([]byte, error) {
	inner, err := s.UnwrapV1InternalResponse(body)
	if err != nil || !gjson.GetBytes(body, "response").IsObject() {
		return inner, err
	}
	identity := gjson.GetBytes(body, "responseId").String()
	if identity == "" {
		return inner, nil
	}
	return sjson.SetBytes(inner, "responseId", identity)
}
