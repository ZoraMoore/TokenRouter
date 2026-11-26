package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/tidwall/gjson"

	logger "github.com/TokenFlux/TokenRouter/internal/infra/telemetry/logging"
	"github.com/TokenFlux/TokenRouter/internal/protocol/bridge"
	geminiwire "github.com/TokenFlux/TokenRouter/internal/protocol/gemini"
	"github.com/TokenFlux/TokenRouter/internal/upstream"
)

type ResponseOptions struct {
	Context                       context.Context
	Stream                        upstream.GeminiStreamOptions
	ObserveRaw                    func([]byte)
	ObserveState                  func(*upstream.TokenUsage, *int)
	GoogleError                   func(int, string) error
	ReadBody                      func(io.Reader) ([]byte, error)
	WriteHeaders                  func(http.Header, http.Header)
	DebugHeaders, HasHeaderFilter bool
	ObserveImages                 func([]byte)
	ReverseTools                  func([]byte) []byte
	ClaudeError, ChatError        func(int, string, string) error
	CompatError                   func(OpenAICompatProtocol, int, string, string) error
}
type (
	ResponseAdapter struct{ Options ResponseOptions }
	StreamResult    struct {
		Usage        *upstream.TokenUsage
		FirstTokenMs *int
	}
)

type NativeStreamResult struct {
	Usage        *upstream.TokenUsage
	FirstTokenMs *int
}

func (s *ResponseAdapter) HandleNonStreamingResponse(c *upstream.OutputContext, resp *http.Response, originalModel string) (*upstream.TokenUsage, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, s.Options.ClaudeError(http.StatusBadGateway, "upstream_error", "Failed to read upstream response")
	}

	unwrappedBody, err := UnwrapGeminiResponse(body)
	if err != nil {
		return nil, s.Options.ClaudeError(http.StatusBadGateway, "upstream_error", "Failed to parse upstream response")
	}
	s.observeRaw(unwrappedBody)
	s.Options.ObserveImages(unwrappedBody)

	var geminiResp map[string]any
	if err := json.Unmarshal(unwrappedBody, &geminiResp); err != nil {
		return nil, s.Options.ClaudeError(http.StatusBadGateway, "upstream_error", "Failed to parse upstream response")
	}

	claudeResp, usage := ConvertGeminiToClaudeMessage(geminiResp, originalModel, unwrappedBody, false)
	responseBody, err := json.Marshal(claudeResp)
	if err != nil {
		return nil, err
	}
	c.NextEvent(bridge.CompatJSONHasContent(responseBody), true)
	c.Data(http.StatusOK, "application/json; charset=utf-8", responseBody)

	return usage, nil
}

func (s *ResponseAdapter) HandleStreamingResponse(c *upstream.OutputContext, resp *http.Response, startTime time.Time, originalModel string) (*StreamResult, error) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}

	state := bridge.NewNativeGeminiMessagesStream(bridge.NativeGeminiRuntime{RandomHex: upstream.RandomHex, MessageID: upstream.GenerateAnthropicMsgID})

	var firstTokenMs *int
	writeEvent := func(event bridge.NativeMessageEvent) {
		if event.FirstToken && firstTokenMs == nil {
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
			if s.Options.ObserveState != nil {
				s.Options.ObserveState(nil, firstTokenMs)
			}
		}
		if event.Name != "" {
			WriteSSE(c.Writer, event.Name, event.Data, func(payload []byte) {
				semantic, terminal := bridge.CompatOutputMeaning(append([]byte("data: "), payload...))
				c.NextEvent(semantic, terminal)
			})
		}
		if event.Flush {
			flusher.Flush()
		}
	}
	for event := range state.Begin(upstream.GenerateAnthropicMsgID(), originalModel) {
		writeEvent(event)
	}
	observed, err := s.readStream(resp, true, func(event upstream.GeminiStreamEvent) error {
		if len(event.Data) == 0 {
			if event.Terminal {
				c.Complete()
			}
			return nil
		}
		s.observeRaw(event.Data)
		s.Options.ObserveImages(event.Data)
		var payload map[string]any
		if err := json.Unmarshal(event.Data, &payload); err != nil {
			return err
		}
		for message := range state.Process(payload, event.Data) {
			writeEvent(message)
		}
		if event.Terminal {
			c.Complete()
		}
		return nil
	})
	// Refresh sparse usage before final conversion so absent tail fields cannot
	// replace input or cache counters that were observed in earlier frames.
	if len(observed.UsageJSON) > 0 {
		var usagePayload map[string]any
		_ = json.Unmarshal(observed.UsageJSON, &usagePayload)
		for range state.Process(usagePayload, observed.UsageJSON) {
		}
	}
	result := &StreamResult{Usage: LegacyNativeGeminiUsage(state.Usage()), FirstTokenMs: firstTokenMs}
	if c.Err() != nil {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	for event := range state.Finish() {
		writeEvent(event)
	}
	return result, c.Err()
}

func WriteSSE(w io.Writer, event string, data any, observers ...func([]byte)) {
	if event != "" {
		_, _ = fmt.Fprintf(w, "event: %s\n", event)
	}
	b, _ := json.Marshal(data)
	for _, observe := range observers {
		if observe != nil {
			observe(b)
		}
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", string(b))
}

func UnwrapIfNeeded(isOAuth bool, raw []byte) []byte {
	if !isOAuth {
		return raw
	}
	inner, err := UnwrapGeminiResponse(raw)
	if err != nil {
		return raw
	}
	return inner
}

func CollectGeminiSSE(body io.Reader, isOAuth bool, observers ...func([]byte)) (map[string]any, *upstream.TokenUsage, error) {
	adapter := ResponseAdapter{Options: ResponseOptions{ObserveRaw: func(raw []byte) {
		for _, observe := range observers {
			if observe != nil {
				observe(raw)
			}
		}
	}}}
	var closer io.ReadCloser
	if existing, ok := body.(io.ReadCloser); ok {
		closer = existing
	} else {
		closer = io.NopCloser(body)
	}
	return adapter.collectSSE(&http.Response{Body: closer}, isOAuth)
}

func PickGeminiCollectResult(last map[string]any, lastWithParts map[string]any) map[string]any {
	return bridge.NativePickGeminiCollectResult(last, lastWithParts)
}

// AppendCollectedGeminiParts 聚合 SSE 增量，同时保持 reasoning 与普通文本的边界。
func AppendCollectedGeminiParts(collected []any, parts []map[string]any) []any {
	for _, part := range parts {
		partCopy := make(map[string]any, len(part))
		maps.Copy(partCopy, part)

		text, hasText := partCopy["text"].(string)
		if hasText && text != "" && len(collected) > 0 {
			if previous, ok := collected[len(collected)-1].(map[string]any); ok {
				previousText, previousHasText := previous["text"].(string)
				previousThought, _ := previous["thought"].(bool)
				currentThought, _ := partCopy["thought"].(bool)
				if previousHasText && previousThought == currentThought {
					previousCopy := make(map[string]any, len(previous))
					maps.Copy(previousCopy, previous)
					previousCopy["text"] = previousText + text
					collected[len(collected)-1] = previousCopy
					continue
				}
			}
		}

		collected = append(collected, partCopy)
	}
	return collected
}

// MergeCollectedGeminiParts 将完整 SSE part 序列写回最终 Gemini 响应。
func MergeCollectedGeminiParts(response map[string]any, collectedParts []any) map[string]any {
	if len(collectedParts) == 0 {
		return response
	}

	// 浅拷贝外层响应，避免直接改动调用方保留的 map。
	result := make(map[string]any)
	maps.Copy(result, response)

	// 取出或创建 candidates。
	candidates, ok := result["candidates"].([]any)
	if !ok || len(candidates) == 0 {
		candidates = []any{map[string]any{}}
	} else {
		candidates = append([]any(nil), candidates...)
	}

	// 取出第一个 candidate。
	candidate, ok := candidates[0].(map[string]any)
	if !ok {
		candidate = make(map[string]any)
		candidates[0] = candidate
	} else {
		candidateCopy := make(map[string]any, len(candidate))
		maps.Copy(candidateCopy, candidate)
		candidate = candidateCopy
		candidates[0] = candidate
	}

	// 取出或创建 content。
	content, ok := candidate["content"].(map[string]any)
	if !ok {
		content = map[string]any{"role": "model"}
		candidate["content"] = content
	} else {
		contentCopy := make(map[string]any, len(content))
		maps.Copy(contentCopy, content)
		content = contentCopy
		candidate["content"] = content
	}

	content["parts"] = append([]any(nil), collectedParts...)
	result["candidates"] = candidates

	return result
}

func (s *ResponseAdapter) HandleNativeNonStreamingResponse(c *upstream.OutputContext, resp *http.Response, isOAuth bool) (*upstream.TokenUsage, error) {
	if s.Options.DebugHeaders {
		logger.LegacyPrintf("service.gemini_messages_compat", "[GeminiAPI] ========== Response Headers ==========")
		for key, values := range resp.Header {
			if strings.HasPrefix(strings.ToLower(key), "x-ratelimit") {
				logger.LegacyPrintf("service.gemini_messages_compat", "[GeminiAPI] %s: %v", key, values)
			}
		}
		logger.LegacyPrintf("service.gemini_messages_compat", "[GeminiAPI] ========================================")
	}

	respBody, err := s.Options.ReadBody(resp.Body)
	if err != nil {
		return nil, err
	}

	if isOAuth {
		unwrappedBody, uwErr := UnwrapGeminiResponse(respBody)
		if uwErr == nil {
			respBody = unwrappedBody
		}
	}
	s.observeRaw(respBody)
	s.Options.ObserveImages(respBody)

	s.Options.WriteHeaders(c.Writer.Header(), resp.Header)

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.NextEvent(geminiwire.ObservePayload(respBody).Semantic, true)
	c.Data(resp.StatusCode, contentType, respBody)

	if u := ExtractGeminiUsage(respBody); u != nil {
		return u, nil
	}
	return &upstream.TokenUsage{}, nil
}

func (s *ResponseAdapter) HandleNativeStreamingResponse(c *upstream.OutputContext, resp *http.Response, startTime time.Time, isOAuth bool) (*NativeStreamResult, error) {
	if s.Options.DebugHeaders {
		for key, values := range resp.Header {
			if strings.HasPrefix(strings.ToLower(key), "x-ratelimit") {
				logger.LegacyPrintf("service.gemini_messages_compat", "[GeminiAPI] %s: %v", key, values)
			}
		}
	}
	if s.Options.HasHeaderFilter {
		s.Options.WriteHeaders(c.Writer.Header(), resp.Header)
	}
	c.Status(resp.StatusCode)
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	result := &NativeStreamResult{Usage: &upstream.TokenUsage{}}
	observed, err := s.readStream(resp, isOAuth, func(event upstream.GeminiStreamEvent) error {
		if len(event.Data) == 0 {
			if event.Terminal {
				c.Complete()
			}
			return nil
		}
		if usage := ExtractGeminiUsage(event.Data); usage != nil {
			result.Usage = usage
		}
		if result.FirstTokenMs == nil {
			ms := int(time.Since(startTime).Milliseconds())
			result.FirstTokenMs = &ms
			if s.Options.ObserveState != nil {
				s.Options.ObserveState(nil, result.FirstTokenMs)
			}
		}
		s.observeRaw(event.Data)
		s.Options.ObserveImages(event.Data)
		if c.Err() != nil {
			return nil
		}
		c.NextEvent(geminiwire.ObservePayload(event.Data).Semantic, event.Terminal)
		if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", event.Data); err != nil {
			return nil
		}
		c.Writer.Flush()
		return nil
	})
	if usage := ExtractGeminiUsage(observed.UsageJSON); usage != nil {
		result.Usage = usage
	}
	if c.Err() != nil {
		return result, nil
	}
	return result, err
}

// UnwrapGeminiResponse 解包 Gemini OAuth 响应中的 response 字段
// 使用 gjson 零拷贝提取，避免完整 Unmarshal+Marshal。
func UnwrapGeminiResponse(raw []byte) ([]byte, error) {
	result := gjson.GetBytes(raw, "response")
	if result.Exists() && result.Type == gjson.JSON {
		return []byte(result.Raw), nil
	}
	return raw, nil
}

// ConvertGeminiToClaudeMessage 使用上游 ID 生成器转换响应，并转换网关用量类型。
func ConvertGeminiToClaudeMessage(geminiResp map[string]any, originalModel string, rawData []byte, includeInlineData bool) (map[string]any, *upstream.TokenUsage) {
	result, usage := bridge.NativeConvertGeminiToClaudeMessage(bridge.NativeGeminiRuntime{MessageID: upstream.GenerateAnthropicMsgID, RandomHex: upstream.RandomHex}, geminiResp, originalModel, rawData, includeInlineData)
	return result, LegacyNativeGeminiUsage(usage)
}

// ExtractGeminiUsage 调用协议桥接解析用量，并转换为上游用量类型。
func ExtractGeminiUsage(data []byte) *upstream.TokenUsage {
	return LegacyNativeGeminiUsage(bridge.NativeExtractGeminiUsage(data))
}

// ExtractGeminiParts 调用 Gemini 协议转换提取响应内容。
func ExtractGeminiParts(geminiResp map[string]any) []map[string]any {
	return bridge.NativeExtractGeminiParts(geminiResp)
}

func LegacyNativeGeminiUsage(usage *bridge.NativeGeminiUsage) *upstream.TokenUsage {
	return bridge.NativeUsageProjection(usage)
}

func (s *ResponseAdapter) observeRaw(payload []byte) {
	if s.Options.ObserveRaw != nil {
		s.Options.ObserveRaw(payload)
	}
}

func (s *ResponseAdapter) collectSSE(resp *http.Response, isOAuth bool) (map[string]any, *upstream.TokenUsage, error) {
	options := s.Options.Stream
	if isOAuth {
		options.Unwrap = UnwrapGeminiResponse
	}
	collected, observed, err := upstream.CollectGeminiStream(s.Options.Context, resp, options, s.observeRaw)
	usage := ExtractGeminiUsage(observed.UsageJSON)
	if usage == nil {
		usage = &upstream.TokenUsage{}
	}
	return collected, usage, err
}

func (s *ResponseAdapter) readStream(resp *http.Response, wrapped bool, consume func(upstream.GeminiStreamEvent) error) (upstream.GeminiStreamResult, error) {
	options := s.Options.Stream
	if wrapped {
		options.Unwrap = UnwrapGeminiResponse
	}
	return upstream.ReadGeminiStream(s.Options.Context, resp, options, consume)
}
