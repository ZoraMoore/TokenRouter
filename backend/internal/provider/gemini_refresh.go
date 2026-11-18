package provider

import (
	"context"
	"time"
)

// GeminiTokenRefresher 保留平台资格与合并规则，技术缓存身份由外层注入。
type GeminiTokenRefresher struct {
	Authorization *GeminiAuthorization
	Key           func(*Record) string
}

func (r *GeminiTokenRefresher) CanRefresh(value *Record) bool {
	return CanRefreshGemini(value)
}

func (r *GeminiTokenRefresher) NeedsRefresh(value *Record, window time.Duration) bool {
	return NeedsRefreshGemini(value, window)
}

func (r *GeminiTokenRefresher) CacheKey(value *Record) string {
	return r.Key(value)
}

func (r *GeminiTokenRefresher) Refresh(ctx context.Context, value *Record) (map[string]any, error) {
	return RefreshGeminiCredentials(ctx, value, r.Authorization)
}

func CanRefreshGemini(provider *Record) bool {
	return provider.Platform == PlatformGemini && provider.Type == ProviderTypeOAuth
}

func NeedsRefreshGemini(provider *Record, refreshWindow time.Duration) bool {
	if !CanRefreshGemini(provider) {
		return false
	}
	expiresAt := provider.GetCredentialAsTime("expires_at")
	if expiresAt == nil {
		return false
	}
	return time.Until(*expiresAt) < refreshWindow
}

func RefreshGeminiCredentials(ctx context.Context, provider *Record, authorization *GeminiAuthorization) (map[string]any, error) {
	tokenInfo, err := authorization.RefreshProviderToken(ctx, provider)
	if err != nil {
		return nil, err
	}

	newCredentials := authorization.BuildProviderCredentials(tokenInfo)
	newCredentials = MergeCredentials(provider.Credentials, newCredentials)

	return newCredentials, nil
}
