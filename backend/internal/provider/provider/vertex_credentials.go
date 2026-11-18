package provider

import (
	"context"
	"log/slog"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/protocol/google"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/upstream/vertex"
)

// GeminiTokenCacheKey 保留服务账号身份摘要与 OAuth 提供商 ID 两种命名空间。
func GeminiTokenCacheKey(value *provider.Record) string {
	if value != nil && value.Type == provider.ProviderTypeServiceAccount {
		if key, err := ParseVertexServiceAccountKey(value); err == nil {
			return VertexServiceAccountCacheKey(value, key)
		}
	}
	return provider.GeminiOAuthTokenCacheKey(value)
}

// ParseVertexServiceAccountKey 将提供商保存形状交给唯一平台解析器。
func ParseVertexServiceAccountKey(value *provider.Record) (*google.ServiceAccountKey, error) {
	raw, err := provider.VertexServiceAccountJSON(value)
	if err != nil {
		return nil, err
	}
	return vertex.ParseVertexServiceAccountJSON(raw)
}

// vertexServiceAccountProxyURL 按提供商的代理绑定读取代理地址。
func vertexServiceAccountProxyURL(value *provider.Record) string {
	if value == nil || value.ProxyID == nil || value.Proxy == nil {
		return ""
	}
	return value.Proxy.URL()
}

// VertexServiceAccountCacheKey 在提供商或已解析密钥缺失时返回对应的缓存键。
func VertexServiceAccountCacheKey(value *provider.Record, key *google.ServiceAccountKey) string {
	var id int64
	if value != nil {
		id = value.ID
	}
	if key == nil {
		if value == nil {
			return "vertex:service_account:"
		}
		return provider.VertexServiceAccountCacheKey(id, "", "", false)
	}
	return provider.VertexServiceAccountCacheKey(id, key.ClientEmail, key.PrivateKeyID, true)
}

// VertexServiceAccountAccessToken 组合提供商缓存协调与平台交换，不另建缓存或刷新锁。
func VertexServiceAccountAccessToken(ctx context.Context, cache provider.AccessTokenCache, value *provider.Record) (string, error) {
	key, err := ParseVertexServiceAccountKey(value)
	if err != nil {
		return "", err
	}
	return provider.GetVertexServiceAccountAccessToken(ctx, provider.VertexTokenOptions{
		ProviderID: value.ID,
		CacheKey:   VertexServiceAccountCacheKey(value, key),
		Cache:      cache,
		Warn:       slog.Warn,
		Exchange: func(ctx context.Context) (string, time.Duration, error) {
			return vertex.ExchangeServiceAccountToken(ctx, key, vertexServiceAccountProxyURL(value))
		},
	})
}
