package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"

	"github.com/TokenFlux/TokenRouter/internal/infra/httpclient/tlsfingerprint"
	openaiprotocol "github.com/TokenFlux/TokenRouter/internal/protocol/openai"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/upstream"
)

const (
	stateProbeURL         = "https://chatgpt.com/backend-api/codex/responses"
	stateProbeHeader      = "X-Codex-Turn-State"
	stateProbeStepTimeout = 45 * time.Second
	stateProbeWireLimit   = 1 << 20
	stateProbeTextLimit   = 16 << 10
	stateProbeHeaderLimit = 16 << 10
)

// stateProbeSession 保存一次测试使用的账号、代理、TLS 配置和 HTTP 会话。
type stateProbeSession struct {
	target  openaiTestTarget
	model   string
	token   string
	proxy   string
	profile *tlsfingerprint.Profile
	headers http.Header
	jar     http.CookieJar
}

type stateProbeShot struct {
	status    int
	state     string
	stateLen  int
	reported  string
	visible   string
	failure   string
	sent      bool
	completed bool
}

type stateProbeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ProbeTurnState 在同一会话里保留首发的 Cookie、状态和对话上下文。
func (t openaiTestTarget) ProbeTurnState(ctx context.Context, model string) provider.StateProbeResult {
	var result provider.StateProbeResult
	firstStart := time.Now()
	firstCtx, firstCancel := context.WithTimeout(ctx, stateProbeStepTimeout)
	defer firstCancel()
	session, failure := t.prepareProbeSession(firstCtx, model)
	if failure != "" {
		result.Failure = failure
		result.MintLatencyMs = time.Since(firstStart).Milliseconds()
		return result
	}
	input := []stateProbeMessage{{Role: "user", Content: "请只回复 OK。"}}
	first := session.request(firstCtx, input, "")
	firstCancel()
	result.MintLatencyMs = time.Since(firstStart).Milliseconds()
	result.MintStatus, result.TicketLength, result.MintCompleted = first.status, first.stateLen, first.completed
	result.MintReportedModel, result.ReportedModel = first.reported, first.reported
	if first.sent {
		result.Model = model
	}
	if first.failure != "" {
		result.Failure = first.failure
		return result
	}
	if first.state == "" {
		result.Failure = "no_ticket"
		return result
	}
	result.Minted = true
	if ctx.Err() != nil {
		return result
	}
	if first.visible != "" {
		input = append(input, stateProbeMessage{Role: "assistant", Content: first.visible})
	}
	input = append(input, stateProbeMessage{Role: "user", Content: "请再回复一次 OK。"})
	secondStart := time.Now()
	secondCtx, secondCancel := context.WithTimeout(ctx, stateProbeStepTimeout)
	defer secondCancel()
	second := session.request(secondCtx, input, first.state)
	result.ContinueLatencyMs = time.Since(secondStart).Milliseconds()
	result.ContinueStatus, result.ContinueTicketLength, result.ContinueCompleted = second.status, second.stateLen, second.completed
	result.ContinueReportedModel = second.reported
	if second.reported != "" {
		result.ReportedModel = second.reported
	}
	result.Failure = second.failure
	result.NewTicket = second.completed && second.state != "" && second.state != first.state
	return result
}

func (t openaiTestTarget) prepareProbeSession(ctx context.Context, model string) (*stateProbeSession, string) {
	if t.executor == nil || t.executor.Transport == nil {
		return nil, "upstream_error"
	}
	token := strings.TrimSpace(t.record.GetOpenAIAccessToken())
	if token == "" {
		return nil, "account_error"
	}
	run := NewTestRun(ctx, nil, nil)
	defer run.Cancel()
	run.Automatic = true
	agent := strings.TrimSpace(t.record.GetOpenAIUserAgent())
	if agent == "" {
		agent = "TokenRouter"
	}
	run.Headers.Set("User-Agent", agent)
	if t.executor.Prepare != nil {
		if err := t.executor.Prepare(run, t.record); err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return nil, "timeout"
			}
			return nil, "account_error"
		}
	}
	sessionID, err := uuid.NewRandom()
	if err != nil {
		return nil, "upstream_error"
	}
	headers := make(http.Header)
	headers.Set("session_id", sessionID.String())
	headers.Set("User-Agent", agent)
	headers.Set("Accept", "text/event-stream")
	headers.Set("Content-Type", "application/json")
	headers.Set("Authorization", "Bearer "+token)
	setTestChatGPTHeaders(headers, t.record)
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, "upstream_error"
	}
	s := &stateProbeSession{target: t, model: model, token: token, headers: headers, jar: jar}
	if t.executor.ResolveTLS != nil {
		s.profile = t.executor.ResolveTLS(run, t.record)
	}
	if t.record.ProxyID != nil && t.record.Proxy != nil {
		s.proxy = t.record.Proxy.URL()
	}
	return s, ""
}

func (s *stateProbeSession) request(ctx context.Context, input []stateProbeMessage, previous string) (shot stateProbeShot) {
	record := s.target.record
	if !record.AllowsOpenAIOAuthTarget(record.OpenAIOAuthFinalModelPolicy(ctx), s.model, ModelDefaults()) {
		shot.failure = "model_unsupported"
		return shot
	}
	if err := ctx.Err(); err != nil {
		shot.failure = "timeout"
		return shot
	}
	payload, err := json.Marshal(map[string]any{"model": s.model, "input": input, "instructions": "请简短回答。", "stream": true, "store": false})
	if err != nil {
		shot.failure = "upstream_error"
		return shot
	}
	requestCtx := upstream.WithHTTPUpstreamRedirectsDisabled(upstream.WithHTTPUpstreamProfile(ctx, upstream.HTTPUpstreamProfileOpenAI))
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, stateProbeURL, bytes.NewReader(payload))
	if err != nil {
		shot.failure = "upstream_error"
		return shot
	}
	req.Header = s.headers.Clone()
	if previous != "" {
		req.Header.Set(stateProbeHeader, previous)
	}
	for _, cookie := range s.jar.Cookies(req.URL) {
		req.AddCookie(cookie)
	}
	resp, err := s.target.executor.Transport.DoWithTLS(req, s.proxy, record.ID, record.Concurrency, s.profile)
	if err != nil || resp == nil {
		shot.failure = "network_error"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			shot.failure = "timeout"
		}
		return shot
	}
	shot.status, shot.sent = resp.StatusCode, true
	if resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	states := resp.Header.Values(stateProbeHeader)
	if len(states) > 0 {
		shot.stateLen = len(states[0])
	}
	if resp.StatusCode != http.StatusOK {
		shot.failure = stateProbeHTTPFailure(resp)
		if ctx.Err() == context.DeadlineExceeded {
			shot.failure = "timeout"
		}
		return shot
	}
	if resp.Body == nil {
		shot.failure = "stream_error"
		return shot
	}
	if len(states) > 1 || (len(states) == 1 && !validProbeState(states[0])) {
		shot.failure = "invalid_state"
		return shot
	}
	if len(states) == 1 {
		shot.state = states[0]
	}
	s.jar.SetCookies(req.URL, resp.Cookies())
	shot.readStream(ctx, resp.Body)
	if ctx.Err() == context.DeadlineExceeded {
		shot.completed = false
		shot.failure = "timeout"
	}
	if !s.reportableModel(shot.reported, req, resp, previous, shot.state) {
		shot.reported = ""
	}
	return shot
}

// reportableModel 拦下混入模型字段的会话状态、Cookie 及该账号的凭据。
func (s *stateProbeSession) reportableModel(model string, req *http.Request, resp *http.Response, previous, state string) bool {
	if !provider.SupportsStateProbeModel(model) || strings.HasPrefix(model, "sk-") || strings.HasPrefix(model, "eyJ") {
		return false
	}
	secrets := []string{s.token, state, previous, req.Header.Get("session_id"), req.Header.Get("ChatGPT-Account-ID")}
	for _, cookie := range append(req.Cookies(), resp.Cookies()...) {
		secrets = append(secrets, cookie.Value)
	}
	for _, value := range secrets {
		if value != "" && (model == value || (len(value) >= 6 && strings.Contains(model, value))) {
			return false
		}
	}
	return true
}

// validProbeState 接受可安全原样写入单个 HTTP 头的状态值。
func validProbeState(state string) bool {
	return len(state) <= stateProbeHeaderLimit && strings.IndexFunc(state, func(r rune) bool { return r < 0x21 || r > 0x7e }) < 0
}

func stateProbeHTTPFailure(resp *http.Response) string {
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return "account_error"
	case http.StatusTooManyRequests:
		return "rate_limited"
	case http.StatusBadRequest, http.StatusNotFound, http.StatusUnprocessableEntity:
		if resp.Body == nil {
			return "upstream_error"
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		fault := gjson.GetBytes(raw, "error")
		code := strings.ToLower(fault.Get("code").String())
		switch code {
		case "model_not_found", "model_not_supported", "unsupported_model":
			return "model_unsupported"
		}
		message := strings.ToLower(fault.Get("message").String())
		if strings.Contains(message, "model") && (strings.Contains(message, "not supported") || strings.Contains(message, "does not exist")) {
			return "model_unsupported"
		}
	}
	return "upstream_error"
}

// readStream 同时限制整条 SSE 和下一步需要保存的上下文。单独的 EOF 或 [DONE] 无法证明该回合完成。
func (s *stateProbeShot) readStream(ctx context.Context, body io.Reader) {
	limited := &io.LimitedReader{R: body, N: stateProbeWireLimit + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 4096), stateProbeWireLimit+2)
	visible := answerText{limit: stateProbeTextLimit}
	var data openaiprotocol.SSEDataAccumulator
	ended := false
	s.failure = "stream_error"
	accept := func(raw []byte) {
		if ended {
			return
		}
		if !gjson.ValidBytes(raw) {
			ended = true
			return
		}
		frame := gjson.ParseBytes(raw)
		switch frame.Get("type").String() {
		case "response.done":
			if frame.Get("response.status").String() != "completed" {
				ended = true
				return
			}
			fallthrough
		case "response.created", "response.in_progress", "response.completed":
			if model := frame.Get("response.model"); model.Type == gjson.String && model.String() != "" {
				s.reported = model.String()
			}
			if frame.Get("response.error").Type != gjson.Null || frame.Get("response.incomplete_details").Type != gjson.Null {
				ended = true
				return
			}
		}
		done, err := parseAnswerFrame(answerResponses, frame, &visible)
		if err != nil {
			ended = true
			return
		}
		if visible.overflow {
			s.failure = "output_limit"
			ended = true
			return
		}
		if done {
			s.completed, s.visible, s.failure = true, visible.String(), ""
			ended = true
		}
	}
	for scanner.Scan() {
		if limited.N == 0 {
			s.failure = "output_limit"
			return
		}
		if ctx.Err() != nil {
			return
		}
		line := scanner.Text()
		if value, ok := openaiprotocol.ExtractSSEDataLine(line); ok && strings.TrimSpace(value) == "[DONE]" {
			data.Flush(accept)
			return
		}
		data.AddLine(line, accept)
		if ended {
			return
		}
	}
	if limited.N == 0 {
		s.failure = "output_limit"
		return
	}
	if scanner.Err() == nil && ctx.Err() == nil {
		data.Flush(accept)
	}
}
