package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	providercore "github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/provider/operation"
)

const (
	reviveBaseURL   = "https://revivetoken.scienceedu.me"
	revivePageSize  = 100
	reviveBodyLimit = 2 << 20
)

var reviveIDPattern = regexp.MustCompile("^[0-9a-f]{32}$")

// ReviveClient 调用 ReviveToken 的档案、号池解绑和登录接口。
type ReviveClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

type reviveAccountPage struct {
	Items    []providercore.ReviveAccount `json:"items"`
	Accounts []providercore.ReviveAccount `json:"accounts"`
	Total    *int                         `json:"total"`
}

// NewReviveClient 使用服务端密钥和固定的 HTTPS 服务地址。
func NewReviveClient(apiKey string) *ReviveClient {
	return &ReviveClient{
		baseURL: reviveBaseURL,
		apiKey:  strings.TrimSpace(apiKey),
		http: &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (c *ReviveClient) Configured() bool {
	return c != nil && c.apiKey != ""
}

func (c *ReviveClient) FindAccount(ctx context.Context, email string) (string, error) {
	var match string
	firstTotal := -1
	for page := 1; ; page++ {
		query := url.Values{
			"email":     {email},
			"page":      {strconv.Itoa(page)},
			"page_size": {strconv.Itoa(revivePageSize)},
		}
		var data reviveAccountPage
		if err := c.request(ctx, http.MethodGet, "/api/v1/accounts?"+query.Encode(), "", http.StatusOK, &data); err != nil {
			return "", err
		}
		if data.Total == nil || *data.Total < 0 {
			return "", operation.NeedsAction("ReviveToken 未返回有效的档案分页总数")
		}
		if firstTotal < 0 {
			firstTotal = *data.Total
		}
		if *data.Total != firstTotal {
			return "", operation.RetryLater("ReviveToken 档案列表正在变化")
		}
		items := data.Items
		if items == nil {
			items = data.Accounts
		}
		for _, item := range items {
			if !strings.EqualFold(strings.TrimSpace(item.Email), email) {
				continue
			}
			id := reviveEntityID(item.ID, item.AccountID)
			if !reviveIDPattern.MatchString(id) {
				return "", operation.NeedsAction("ReviveToken 返回的档案 ID 无效")
			}
			if match != "" && id != match {
				return "", operation.NeedsAction("相同邮箱匹配多个 ReviveToken 档案")
			}
			match = id
		}
		if page >= (firstTotal-1)/revivePageSize+1 {
			break
		}
	}
	if match == "" {
		return "", operation.NeedsAction("ReviveToken 没有匹配的完整邮箱档案")
	}
	return match, nil
}

func (c *ReviveClient) GetAccount(ctx context.Context, id string) (providercore.ReviveAccount, error) {
	if !reviveIDPattern.MatchString(id) {
		return providercore.ReviveAccount{}, operation.NeedsAction("ReviveToken 档案 ID 无效")
	}
	var account providercore.ReviveAccount
	err := c.request(ctx, http.MethodGet, "/api/v1/accounts/"+id, "", http.StatusOK, &account)
	account.ID = reviveEntityID(account.ID, account.AccountID)
	return account, err
}

func (c *ReviveClient) DeletePoolAccount(ctx context.Context, id, key string) (string, error) {
	return c.submit(ctx, http.MethodDelete, id, "pool-account", key)
}

func (c *ReviveClient) SyncAccount(ctx context.Context, id, key string) (string, error) {
	return c.submit(ctx, http.MethodPost, id, "syncs", key)
}

func (c *ReviveClient) submit(ctx context.Context, method, id, action, key string) (string, error) {
	if !reviveIDPattern.MatchString(id) || key == "" {
		return "", operation.NeedsAction("ReviveToken 操作缺少有效的档案 ID 或幂等键")
	}
	var op providercore.ReviveOperation
	if err := c.request(ctx, method, "/api/v1/accounts/"+id+"/"+action, key, http.StatusAccepted, &op); err != nil {
		return "", err
	}
	opID := reviveEntityID(op.ID, op.OperationID)
	if !reviveIDPattern.MatchString(opID) || (op.AccountID != "" && op.AccountID != id) {
		return "", operation.RetryLater("ReviveToken 已受理但未返回可核对的操作 ID")
	}
	return opID, nil
}

func (c *ReviveClient) GetOperation(ctx context.Context, id string) (providercore.ReviveOperation, error) {
	if !reviveIDPattern.MatchString(id) {
		return providercore.ReviveOperation{}, operation.NeedsAction("ReviveToken 操作 ID 无效")
	}
	var op providercore.ReviveOperation
	err := c.request(ctx, http.MethodGet, "/api/v1/operations/"+id, "", http.StatusOK, &op)
	op.ID = reviveEntityID(op.ID, op.OperationID)
	return op, err
}

func (c *ReviveClient) request(ctx context.Context, method, path, key string, expectedStatus int, output any) error {
	if !c.Configured() {
		return operation.NeedsAction("服务端缺少 REVIVE_API_KEY")
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, nil)
	if err != nil {
		return operation.NeedsAction("无法构造 ReviveToken 请求")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return operation.RetryLater("连接 ReviveToken 失败，等待重试")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != expectedStatus {
		switch {
		case resp.StatusCode == http.StatusTooManyRequests:
			return operation.RetryAfter("ReviveToken 限流，等待重试", reviveRetryDelay(resp.Header.Get("Retry-After")))
		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
			return operation.NeedsAction(fmt.Sprintf("ReviveToken 接口返回 %d，请核对密钥权限", resp.StatusCode))
		case resp.StatusCode == http.StatusConflict:
			return operation.NeedsAction("ReviveToken 操作发生冲突（409），请核对外部操作状态")
		case resp.StatusCode >= http.StatusInternalServerError || resp.StatusCode == http.StatusRequestTimeout:
			return operation.RetryLater("ReviveToken 请求暂时失败，等待重试")
		default:
			return operation.NeedsAction(fmt.Sprintf("ReviveToken 接口返回意外状态 %d", resp.StatusCode))
		}
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, reviveBodyLimit+1))
	if err != nil || len(raw) > reviveBodyLimit {
		return operation.RetryLater("ReviveToken 响应读取失败")
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return operation.RetryLater("ReviveToken 响应格式无效")
	}
	if len(envelope.Data) > 0 {
		raw = envelope.Data
	}
	if err := json.Unmarshal(raw, output); err != nil {
		return operation.RetryLater("ReviveToken 响应格式无效")
	}
	return nil
}

func reviveRetryDelay(header string) time.Duration {
	if seconds, err := strconv.ParseInt(header, 10, 32); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(header); err == nil {
		return time.Until(when)
	}
	return 4 * time.Second
}

func reviveEntityID(id, alternate string) string {
	switch {
	case id == "":
		return alternate
	case alternate == "" || alternate == id:
		return id
	default:
		return ""
	}
}
