package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
	"github.com/TokenFlux/TokenRouter/internal/upstream"
)

const (
	CodexInviteDefaultUserAgent        = "Codex Desktop/0.0.0 (Linux; x86_64)"
	codexBackendAPIBaseURL             = "https://chatgpt.com/backend-api"
	codexInviteResetUnavailable        = "CODEX_INVITE_RESET_REFERRAL_UNAVAILABLE"
	codexInviteResetUnavailableMessage = "当前 Codex 推荐邀请入口暂不可用，但已有重置次数仍可使用"
)

// CodexInviteClient 只拥有一次邀请请求的传输参数与响应体。
type CodexInviteClient struct {
	Token           string
	UserAgent       string
	ProviderHeaders func(http.Header)
	Do              func(*http.Request) (*http.Response, error)
}

func (s *CodexInviteClient) GetJSON(ctx context.Context, path string, query map[string]string) (map[string]any, error) {
	target, err := BuildCodexBackendURL(path, query)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	s.applyHeaders(req)
	return s.doJSON(req)
}

func (s *CodexInviteClient) PostJSON(ctx context.Context, path string, body map[string]any) (map[string]any, error) {
	target, err := BuildCodexBackendURL(path, nil)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	s.applyHeaders(req)
	return s.doJSON(req)
}

func (s *CodexInviteClient) applyHeaders(req *http.Request) {
	*req = *req.WithContext(upstream.WithHTTPUpstreamProfile(req.Context(), upstream.HTTPUpstreamProfileOpenAI))
	req.Host = "chatgpt.com"
	req.Header.Set("Authorization", "Bearer "+s.Token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("OpenAI-Beta", openaiQuotaCodexBeta)
	req.Header.Set("OAI-Language", "zh-CN")
	req.Header.Set("originator", "Codex Desktop")
	req.Header.Set("X-OpenAI-Attach-Auth", "1")
	req.Header.Set("X-OpenAI-Attach-Integrity-State", "1")
	req.Header.Set("User-Agent", s.UserAgent)
	req.Header.Set("sec-fetch-site", "none")
	req.Header.Set("sec-fetch-mode", "no-cors")
	req.Header.Set("sec-fetch-dest", "empty")
	req.Header.Set("priority", "u=4, i")
	s.ProviderHeaders(req.Header)
}

func (s *CodexInviteClient) doJSON(req *http.Request) (map[string]any, error) {
	if s.Do == nil {
		return nil, apperror.InternalServer("HTTP_UPSTREAM_NOT_CONFIGURED", "http upstream is not configured")
	}
	req = req.WithContext(upstream.WithHTTPUpstreamProfile(req.Context(), upstream.HTTPUpstreamProfileOpenAI))
	sending := req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/referrals/invite")
	uncertain := func() error {
		if sending {
			return apperror.New(502, "CODEX_INVITE_SEND_UNKNOWN", "邀请结果未确认，请先在 Codex 中查看发送记录")
		}
		return apperror.New(502, "CODEX_INVITE_RESET_UPSTREAM_ERROR", "暂时无法确认 Codex 请求结果，请查询最新状态")
	}
	resp, err := s.Do(req)
	if err != nil {
		return nil, uncertain()
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		switch resp.StatusCode {
		case http.StatusBadRequest, http.StatusUnprocessableEntity:
			return nil, apperror.BadRequest("CODEX_INVITE_REJECTED", "Codex 拒绝了请求，请检查邀请资格和输入")
		case http.StatusForbidden:
			return nil, apperror.Forbidden(codexInviteResetUnavailable, codexInviteResetUnavailableMessage)
		case http.StatusConflict:
			return nil, apperror.Conflict("CODEX_INVITE_CONFLICT", "Codex 已记录该操作，请查询最新状态")
		case http.StatusTooManyRequests:
			return nil, apperror.New(429, "CODEX_INVITE_RATE_LIMITED", "Codex 限制了操作频率，请稍后再试")
		default:
			return nil, uncertain()
		}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, ImageErrorBodyReadLimit+1))
	if err != nil || int64(len(body)) > ImageErrorBodyReadLimit {
		return nil, uncertain()
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil || result == nil {
		return nil, uncertain()
	}
	return result, nil
}

func BuildCodexBackendURL(path string, query map[string]string) (string, error) {
	base, err := url.Parse(codexBackendAPIBaseURL)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	base.Path = strings.TrimRight(base.Path, "/") + path
	if len(query) > 0 {
		values := base.Query()
		for key, value := range query {
			values.Set(key, value)
		}
		base.RawQuery = values.Encode()
	}
	return base.String(), nil
}
