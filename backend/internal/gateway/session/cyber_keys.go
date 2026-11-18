package session

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/gateway/requeststate"
	"github.com/TokenFlux/TokenRouter/internal/upstream"
)

// CyberSessionTranscriptBlockKeys 返回完整请求键，以及容忍改写的可选上下文键。
// 只有观察到模型生成历史，才产生后者。
func CyberSessionTranscriptBlockKeys(apiKeyID int64, body []byte) []string {
	derived := deriveOpenAICyberTranscriptBlockKeys(apiKeyID, body)
	if len(derived.lookupKeys) == 0 {
		return nil
	}
	keys := []string{derived.lookupKeys[len(derived.lookupKeys)-1]}
	if derived.preLatestUserKey != "" && derived.preLatestUserKey != keys[0] {
		keys = append(keys, derived.preLatestUserKey)
	}
	return keys
}

func CyberSessionTranscriptLookupKeys(apiKeyID int64, body []byte) []string {
	return deriveOpenAICyberTranscriptBlockKeys(apiKeyID, body).lookupKeys
}

// CyberSessionScopeKey 标记曾命中的请求来源，命中后再解析转录并执行 MGET，最终屏蔽由内容键决定。
func CyberSessionScopeKey(apiKeyID int64, clientIP, userAgent string) string {
	if apiKeyID <= 0 {
		return ""
	}
	raw := "cyber-scope:v1|api_key=" + strconv.FormatInt(apiKeyID, 10) +
		"|ip=" + strings.TrimSpace(clientIP) +
		"|ua=" + requeststate.NormalizeSessionUserAgent(userAgent)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func HashCyberSessionBlockKey(apiKeyID int64, raw string) string {
	if raw == "" {
		return ""
	}
	isolated := upstream.IsolateSessionID(apiKeyID, raw)
	sum := sha256.Sum256([]byte(isolated))
	return hex.EncodeToString(sum[:])
}
