package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	gatewayprovider "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	"github.com/TokenFlux/TokenRouter/internal/gateway/provider/googleforward"
	"github.com/TokenFlux/TokenRouter/internal/infra/telemetry/logging"
	"github.com/TokenFlux/TokenRouter/internal/ops"
	"github.com/TokenFlux/TokenRouter/internal/pkg/logredact"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
	"github.com/TokenFlux/TokenRouter/internal/server/httpx"
	"github.com/TokenFlux/TokenRouter/internal/upstream"
	gemininative "github.com/TokenFlux/TokenRouter/internal/upstream/gemini"
)

const googleErrorWrittenKey = "google_final_error_written"

// GeminiOutput 保存当前 HTTP 请求和静态输出配置。
type GeminiOutput struct {
	GoogleOutput
	Options googleforward.Options
}

// GeminiCustomCodeSkippedError 对自定义错误码未命中的请求隐藏上游细节并返回 500。
func (s *GeminiOutput) GeminiCustomCodeSkippedError(provider *gatewayprovider.ExecutionProvider, upstreamStatus int, upstreamRequestID string, body []byte, write func()) error {
	c := s.Context

	upstreamMsg := logredact.SanitizeUpstreamQueries(strings.TrimSpace(upstream.ExtractErrorMessage(body)))
	upstreamDetail := s.Options.ErrorDetail(body)
	SetOpsUpstreamError(c, upstreamStatus, upstreamMsg, upstreamDetail)
	AppendOpsUpstreamError(c, ops.OpsUpstreamErrorEvent{
		Platform: provider.Record.Platform,

		ProviderID: provider.Record.ID,

		ProviderName: provider.Record.Name,

		UpstreamStatusCode: upstreamStatus,

		UpstreamRequestID: upstreamRequestID,

		Kind: "http_error",

		Message: upstreamMsg,

		Detail: upstreamDetail,
	})
	write()
	if upstreamMsg == "" {
		return fmt.Errorf("gemini upstream error: %d (not in custom error codes)", upstreamStatus)
	}
	return fmt.Errorf("gemini upstream error: %d (not in custom error codes) message=%s", upstreamStatus, upstreamMsg)
}

// GeminiNativeUpstreamError 按原始状态码和响应体透传不可切换的 Gemini 错误。
func (s *GeminiOutput) GeminiNativeUpstreamError(provider *gatewayprovider.ExecutionProvider, resp *http.Response, respBody []byte, requestID string, isOAuth bool) error {
	c := s.Context

	respBody = gemininative.UnwrapIfNeeded(isOAuth, respBody)
	upstreamMsg := logredact.SanitizeUpstreamQueries(strings.TrimSpace(upstream.ExtractErrorMessage(respBody)))
	upstreamDetail := s.Options.ErrorDetail(respBody)
	SetOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)
	AppendOpsUpstreamError(c, ops.OpsUpstreamErrorEvent{
		Platform: provider.Record.Platform,

		ProviderID: provider.Record.ID,

		ProviderName: provider.Record.Name,

		UpstreamStatusCode: resp.StatusCode,

		UpstreamRequestID: requestID,

		Kind: "http_error",

		Message: upstreamMsg,

		Detail: upstreamDetail,
	})
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	MarkResponseCommitted(c)
	if !c.Writer.Written() {
		if beginGoogleError(c) {
			c.Data(resp.StatusCode, contentType, respBody)
		}
	} else {
		writeGoogleErrorBody(c, resp.StatusCode, respBody)
	}
	if upstreamMsg == "" {
		return fmt.Errorf("gemini upstream error: %d", resp.StatusCode)
	}
	return fmt.Errorf("gemini upstream error: %d message=%s", resp.StatusCode, upstreamMsg)
}

func (s *GeminiOutput) GeminiMappedError(provider *gatewayprovider.ExecutionProvider, upstreamStatus int, upstreamRequestID string, body []byte) error {
	c := s.Context

	MarkResponseCommitted(c)
	upstreamMsg := strings.TrimSpace(upstream.ExtractErrorMessage(body))
	upstreamMsg = logredact.SanitizeUpstreamQueries(upstreamMsg)
	upstreamDetail := ""
	if s.Options.LogErrorBody {
		maxBytes := s.Options.LogErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		upstreamDetail = logredact.TruncateUTF8(string(body), maxBytes)
	}
	SetOpsUpstreamError(c, upstreamStatus, upstreamMsg, upstreamDetail)
	AppendOpsUpstreamError(c, ops.OpsUpstreamErrorEvent{
		Platform: provider.Record.Platform,

		ProviderID: provider.Record.ID,

		ProviderName: provider.Record.Name,

		UpstreamStatusCode: upstreamStatus,

		UpstreamRequestID: upstreamRequestID,

		Kind: "http_error",

		Message: upstreamMsg,

		Detail: upstreamDetail,
	})

	if s.Options.LogErrorBody {
		logging.LegacyPrintf("service.gemini_messages_compat", "[Gemini] upstream error %d: %s", upstreamStatus, logredact.TruncateLine(body, s.Options.LogErrorBodyMaxBytes))
	}

	if status, errType, errMsg, matched := ApplyErrorPassthroughRule(
		c,
		capability.PlatformGemini,
		upstreamStatus,
		body,
		http.StatusBadGateway,
		"upstream_error",
		"Upstream request failed",
	); matched {
		_ = s.ClaudeError(status, errType, errMsg)
		if upstreamMsg == "" {
			upstreamMsg = errMsg
		}
		if upstreamMsg == "" {
			return fmt.Errorf("upstream error: %d (passthrough rule matched)", upstreamStatus)
		}
		return fmt.Errorf("upstream error: %d (passthrough rule matched) message=%s", upstreamStatus, upstreamMsg)
	}

	var statusCode int
	var errType, errMsg string

	if mapped := MapGeminiErrorBodyToClaudeError(body); mapped != nil {
		errType = mapped.Type
		if mapped.Message != "" {
			errMsg = mapped.Message
		}
		if mapped.StatusCode > 0 {
			statusCode = mapped.StatusCode
		}
	}

	switch upstreamStatus {
	case 400:
		if statusCode == 0 {
			statusCode = http.StatusBadRequest
		}
		if errType == "" {
			errType = "invalid_request_error"
		}
		if errMsg == "" {
			if upstreamMsg != "" {
				errMsg = upstreamMsg
			} else {
				errMsg = "Invalid request"
			}
		}
	case 401:
		if statusCode == 0 {
			statusCode = http.StatusBadGateway
		}
		if errType == "" {
			errType = "authentication_error"
		}
		if errMsg == "" {
			errMsg = "Upstream authentication failed, please contact administrator"
		}
	case 403:
		if statusCode == 0 {
			statusCode = http.StatusBadGateway
		}
		if errType == "" {
			errType = "permission_error"
		}
		if errMsg == "" {
			errMsg = "Upstream access forbidden, please contact administrator"
		}
	case 404:
		if statusCode == 0 {
			statusCode = http.StatusNotFound
		}
		if errType == "" {
			errType = "not_found_error"
		}
		if errMsg == "" {
			errMsg = "Resource not found"
		}
	case 429:
		if statusCode == 0 {
			statusCode = http.StatusTooManyRequests
		}
		if errType == "" {
			errType = "rate_limit_error"
		}
		if errMsg == "" {
			errMsg = "Upstream rate limit exceeded, please retry later"
		}
	case 529:
		if statusCode == 0 {
			statusCode = http.StatusServiceUnavailable
		}
		if errType == "" {
			errType = "overloaded_error"
		}
		if errMsg == "" {
			errMsg = "Upstream service overloaded, please retry later"
		}
	case 500, 502, 503, 504:
		if statusCode == 0 {
			statusCode = http.StatusBadGateway
		}
		if errType == "" {
			switch upstreamStatus {
			case 504:
				errType = "timeout_error"
			case 503:
				errType = "overloaded_error"
			default:
				errType = "api_error"
			}
		}
		if errMsg == "" {
			errMsg = "Upstream service temporarily unavailable"
		}
	default:
		if statusCode == 0 {
			statusCode = http.StatusBadGateway
		}
		if errType == "" {
			errType = "upstream_error"
		}
		if errMsg == "" {
			errMsg = "Upstream request failed"
		}
	}

	_ = s.ClaudeError(statusCode, errType, errMsg)
	if upstreamMsg == "" {
		return fmt.Errorf("upstream error: %d", upstreamStatus)
	}
	return fmt.Errorf("upstream error: %d message=%s", upstreamStatus, upstreamMsg)
}

func (s *GeminiOutput) ClaudeError(status int, errType, message string) error {
	c := s.Context
	if beginGoogleError(c) {
		WriteAnthropicStreamError(c, status, errType, "", message, c.Writer.Written(), MarkOpsStreamError)
	}
	return fmt.Errorf("%s", message)
}

func (s *GeminiOutput) GoogleError(status int, message string) error {
	WriteGoogleError(s.Context, status, message)
	return fmt.Errorf("%s", message)
}

func (s *GeminiOutput) GeminiOpenAICompatMappedError(
	provider *gatewayprovider.ExecutionProvider,
	upstreamStatus int,
	upstreamRequestID string,
	body []byte,
	protocol gemininative.OpenAICompatProtocol,
) error {
	c := s.Context

	upstreamMsg := logredact.SanitizeUpstreamQueries(strings.TrimSpace(upstream.ExtractErrorMessage(body)))
	SetOpsUpstreamError(c, upstreamStatus, upstreamMsg, "")
	if provider != nil {
		AppendOpsUpstreamError(c, ops.OpsUpstreamErrorEvent{
			Platform: provider.Record.Platform,

			ProviderID: provider.Record.ID,

			ProviderName: provider.Record.Name,

			UpstreamStatusCode: upstreamStatus,

			UpstreamRequestID: upstreamRequestID,

			Kind: "http_error",

			Message: upstreamMsg,
		})
	}

	if status, errType, errMsg, matched := ApplyErrorPassthroughRule(
		c,
		capability.PlatformGemini,
		upstreamStatus,
		body,
		http.StatusBadGateway,
		"upstream_error",
		"Upstream request failed",
	); matched {
		return s.GeminiOpenAICompatError(protocol, status, errType, errMsg)
	}

	statusCode := http.StatusBadGateway
	errType := "upstream_error"
	errMsg := "Upstream request failed"
	if mapped := MapGeminiErrorBodyToClaudeError(body); mapped != nil {
		if mapped.Type != "" {
			errType = mapped.Type
		}
		if mapped.Message != "" {
			errMsg = mapped.Message
		}
		if mapped.StatusCode > 0 {
			statusCode = mapped.StatusCode
		}
	}

	switch upstreamStatus {
	case http.StatusBadRequest:
		if statusCode == http.StatusBadGateway {
			statusCode = http.StatusBadRequest
		}
		if errType == "upstream_error" {
			errType = "invalid_request_error"
		}
		if errMsg == "Upstream request failed" {
			errMsg = "Invalid request"
		}
	case http.StatusNotFound:
		statusCode = http.StatusNotFound
		if errType == "upstream_error" {
			errType = "not_found_error"
		}
		if errMsg == "Upstream request failed" {
			errMsg = "Resource not found"
		}
	case http.StatusTooManyRequests:
		statusCode = http.StatusTooManyRequests
		if errType == "upstream_error" {
			errType = "rate_limit_error"
		}
		if errMsg == "Upstream request failed" {
			errMsg = "Upstream rate limit exceeded, please retry later"
		}
	case 529:
		statusCode = http.StatusServiceUnavailable
		if errType == "upstream_error" {
			errType = "overloaded_error"
		}
		if errMsg == "Upstream request failed" {
			errMsg = "Upstream service overloaded, please retry later"
		}
	}

	if upstreamMsg != "" && errMsg == "Upstream request failed" {
		errMsg = upstreamMsg
	}
	// 池模式的 4xx 不会切换提供商，客户端需要看到上游给出的具体校验原因；
	// 普通提供商仍保留兼容层的通用错误文案。
	if provider != nil && provider.View().IsPoolMode() && upstreamStatus >= http.StatusBadRequest && upstreamMsg != "" {
		errMsg = upstreamMsg
	}
	return s.GeminiOpenAICompatError(protocol, statusCode, errType, errMsg)
}

// GeminiOpenAICompatError 按客户端入口输出对应的 OpenAI 错误格式。
func (s *GeminiOutput) GeminiOpenAICompatError(
	protocol gemininative.OpenAICompatProtocol,
	status int,
	errType string,
	message string,
) error {
	c := s.Context

	if protocol == gemininative.OpenAICompatResponses {
		if beginGoogleError(c) {
			if c.Writer.Written() {
				DefaultOpenAIErrorOutput().WriteStreamingErrorWithCode(c, status, errType, errType, message, true, true)
			} else {
				c.JSON(status, gin.H{"error": gin.H{"code": errType, "message": message}})
			}
		}
		return fmt.Errorf("%s", message)
	}
	return s.ChatError(status, errType, message)
}

func (s *GeminiOutput) ChatError(status int, errType, message string) error {
	c := s.Context
	if beginGoogleError(c) {
		DefaultOpenAIErrorOutput().WriteStreamingErrorWithCode(c, status, errType, "", message, c.Writer.Written(), true)
	}
	return fmt.Errorf("%s", message)
}

func HTTPStatusToGoogleStatus(status int) string { return httpx.HTTPStatusToGoogleStatus(status) }

// beginGoogleError 选择一次最终错误输出，请求取消后停止写入。
func beginGoogleError(c *gin.Context) bool {
	if c.GetBool(googleErrorWrittenKey) || (c.Request != nil && c.Request.Context().Err() != nil) {
		return false
	}
	c.Set(googleErrorWrittenKey, true)
	MarkResponseCommitted(c)
	return true
}

// writeGoogleErrorBody 根据响应提交状态选择 JSON 或原生 SSE 错误。
func writeGoogleErrorBody(c *gin.Context, status int, body []byte) {
	if !beginGoogleError(c) {
		return
	}
	if !c.Writer.Written() || !strings.Contains(strings.ToLower(c.Writer.Header().Get("Content-Type")), "text/event-stream") {
		c.Data(status, "application/json; charset=utf-8", body)
		return
	}
	message := logredact.SanitizeUpstreamQueries(upstream.ExtractErrorMessage(body))
	if message == "" {
		message = http.StatusText(status)
	}
	value := gjson.GetBytes(body, "error")
	if !json.Valid(body) || !value.IsObject() {
		body, _ = json.Marshal(gin.H{"error": gin.H{"code": status, "message": message, "status": HTTPStatusToGoogleStatus(status)}})
	}
	body, _ = sjson.SetBytes(body, "error.message", message)
	code := int(gjson.GetBytes(body, "error.code").Int())
	if code < 400 || code > 599 {
		code = status
	}
	body, _ = sjson.SetBytes(body, "error.code", code)
	if name := gjson.GetBytes(body, "error.status"); name.Type != gjson.String || name.String() == "" {
		body, _ = sjson.SetBytes(body, "error.status", HTTPStatusToGoogleStatus(code))
	}
	var compact bytes.Buffer
	_ = json.Compact(&compact, body)
	MarkOpsStreamFailure(c, "upstream_error", "", message, status)
	sink := ResponseSink{Writer: c.Writer}
	if err := sink.Emit(upstream.OutputEvent{Data: []byte("data: " + compact.String() + "\n\n"), Terminal: true, Flush: true}); err != nil {
		_ = c.Error(err)
	}
}
