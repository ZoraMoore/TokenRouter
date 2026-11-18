package httpapi

import (
	"net/http"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/egress"
	egressadapter "github.com/TokenFlux/TokenRouter/internal/egress/provider"
)

// WriteOpenAIPassthroughResponseHeaders 保留配额头强制放行和旧回合状态清理顺序。
func WriteOpenAIPassthroughResponseHeaders(dst http.Header, src http.Header, filter *egress.CompiledHeaderFilter) {
	if dst == nil || src == nil {
		return
	}
	if filter != nil {
		egressadapter.WriteFilteredHeaders(dst, src, filter)
	} else {
		// 未配置过滤器时透传 content-type。
		if v := strings.TrimSpace(src.Get("Content-Type")); v != "" {
			dst.Set("Content-Type", v)
		}
	}
	// 透传 x-codex-* 响应头时，用 EqualFold 查找大小写变体。
	// 标准 http.Response.Header 通常使用规范化键，测试和自建响应也可能保留小写键。
	getCaseInsensitiveValues := func(h http.Header, want string) []string {
		if h == nil {
			return nil
		}
		for k, vals := range h {
			if strings.EqualFold(k, want) {
				return vals
			}
		}
		return nil
	}

	for _, rawKey := range []string{
		"x-codex-primary-used-percent",
		"x-codex-primary-reset-after-seconds",
		"x-codex-primary-window-minutes",
		"x-codex-secondary-used-percent",
		"x-codex-secondary-reset-after-seconds",
		"x-codex-secondary-window-minutes",
		"x-codex-primary-over-secondary-limit-percent",
	} {
		vals := getCaseInsensitiveValues(src, rawKey)
		if len(vals) == 0 {
			continue
		}
		key := http.CanonicalHeaderKey(rawKey)
		dst.Del(key)
		for _, v := range vals {
			dst.Add(key, v)
		}
	}

	// 回合状态不受通用响应头白名单控制；上游缺失时也要清理旧值，避免
	// failover 后把其它提供商的状态留在下游响应中。
	turnStateKey := http.CanonicalHeaderKey(CodexTurnStateHeader)
	dst.Del(turnStateKey)
	for _, value := range getCaseInsensitiveValues(src, CodexTurnStateHeader) {
		dst.Add(turnStateKey, value)
	}
}
