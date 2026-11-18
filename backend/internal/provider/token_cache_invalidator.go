package provider

import (
	"context"
	"strconv"

	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
)

// TokenCacheInvalidator 根据提供商快照清理各平台的凭据缓存。
type TokenCacheInvalidator interface {
	InvalidateToken(ctx context.Context, provider *Record) error
}

// SessionInvalidator 提供提供商会话的失效操作。
type SessionInvalidator interface {
	Invalidate(int64)
}

// CompositeTokenCacheInvalidator 保留各平台独立键及尽力删除顺序。
type CompositeTokenCacheInvalidator struct {
	warn               func(string, ...any)
	cache              AccessTokenCache // 统一使用一个缓存接口，通过缓存键前缀区分平台
	qoderTokenProvider SessionInvalidator
}

func NewCompositeTokenCacheInvalidator(cache AccessTokenCache, qoderTokenProvider SessionInvalidator, warn func(string, ...any),
) *CompositeTokenCacheInvalidator {
	return &CompositeTokenCacheInvalidator{
		warn:               warn,
		cache:              cache,
		qoderTokenProvider: qoderTokenProvider,
	}
}

func (c *CompositeTokenCacheInvalidator) InvalidateToken(ctx context.Context, provider *Record) error {
	if c == nil || provider == nil {
		return nil
	}
	if provider.Type != capability.ProviderTypeOAuth && !provider.IsQoderCosy() {
		return nil
	}

	var keysToDelete []string
	providerIDKey := "provider:" + strconv.FormatInt(provider.ID, 10)

	switch provider.Platform {
	case capability.PlatformGemini:
		// Gemini 可能有两种缓存键：project_id 或 provider_id
		// 首次获取 token 时可能没有 project_id，之后自动检测到 project_id 后会使用新 key
		// 刷新时删除两种缓存键。
		keysToDelete = append(keysToDelete, GeminiOAuthTokenCacheKey(provider))
		keysToDelete = append(keysToDelete, "gemini:"+providerIDKey)
	case capability.PlatformAntigravity:
		// Antigravity 同样可能有两种缓存键
		keysToDelete = append(keysToDelete, AntigravityTokenCacheKey(provider))
		keysToDelete = append(keysToDelete, "ag:"+providerIDKey)
	case capability.PlatformOpenAI:
		keysToDelete = append(keysToDelete, OpenAITokenCacheKey(provider))
	case capability.PlatformGrok:
		keysToDelete = append(keysToDelete, GrokTokenCacheKey(provider))
		keysToDelete = append(keysToDelete, "grok:"+providerIDKey)
	case capability.PlatformAnthropic:
		keysToDelete = append(keysToDelete, ClaudeTokenCacheKey(provider))
	case capability.PlatformQoder:
		if provider.IsQoderCosy() {
			keysToDelete = append(keysToDelete, QoderTokenCacheKey(provider))
			if c.qoderTokenProvider != nil {
				c.qoderTokenProvider.Invalidate(provider.ID)
			}
		}
	default:
		return nil
	}

	if c.cache == nil {
		return nil
	}

	// 删除所有可能的缓存键（去重后）
	seen := make(map[string]bool)
	for _, key := range keysToDelete {
		if seen[key] {
			continue
		}
		seen[key] = true
		if err := c.cache.DeleteAccessToken(ctx, key); err != nil {
			if c.warn != nil {
				c.warn("token_cache_delete_failed", "key", key, "provider_id", provider.ID, "error", err)
			}
		}
	}

	return nil
}
