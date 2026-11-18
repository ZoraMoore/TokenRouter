package policy

import (
	"fmt"
	"net/textproto"
	"strings"

	"golang.org/x/net/http/httpguts"
)

// MaxForwardedClientIPHeaders 是自定义客户端地址 Header 的数量上限。
const MaxForwardedClientIPHeaders = 16

// NormalizeForwardedClientIPHeaders 规范并去重请求头名称，检查允许使用的名称。
func NormalizeForwardedClientIPHeaders(headers []string) ([]string, error) {
	normalized := make([]string, 0, len(headers))
	seen := make(map[string]struct{}, len(headers))
	for _, header := range headers {
		header = strings.TrimSpace(header)
		if !httpguts.ValidHeaderFieldName(header) {
			return nil, fmt.Errorf("invalid HTTP header field name %q", header)
		}
		canonical := textproto.CanonicalMIMEHeaderKey(header)
		key := strings.ToLower(canonical)
		if _, exists := seen[key]; exists {
			continue
		}
		if len(normalized) == MaxForwardedClientIPHeaders {
			return nil, fmt.Errorf("forwarded client IP headers must contain at most %d unique names", MaxForwardedClientIPHeaders)
		}
		seen[key] = struct{}{}
		normalized = append(normalized, canonical)
	}
	return normalized, nil
}
