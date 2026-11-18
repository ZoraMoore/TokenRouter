package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	gatewayprovider "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	provideradapter "github.com/TokenFlux/TokenRouter/internal/provider/provider"
	"github.com/TokenFlux/TokenRouter/internal/upstream/openai"
)

const codexAccountIdentityProjectionContextKey = "codex_account_identity_projection"

// 同义字段按优先级读取；只同步已经存在的别名，避免扩张上游协议表面。
var codexIdentityAliases = []struct {
	kind  string
	names []string
}{
	{"installation", []string{"x-codex-installation-id", "installation_id"}},
	{"session", []string{"session-id", "session_id"}},
	{"thread", []string{"thread-id", "thread_id"}},
	{"turn", []string{"turn-id", "turn_id"}},
	{"window", []string{"x-codex-window-id", "window-id", "window_id"}},
	{"request", []string{"x-client-request-id"}},
}

// 快照只属于当前凭据和租户；头与正文共享结果，重试不得再次哈希派生值。
type codexAccountIdentityProjection struct {
	namespace string
	apiKeyID  int64
	raw       map[string]string
	scoped    map[string]string
}

func codexIdentityMetadata(values map[string]any) map[string]any {
	result := map[string]any{}
	if raw, ok := values[openai.WSTurnMetadataHeader].(string); ok {
		_ = json.Unmarshal([]byte(raw), &result)
	}
	return result
}

func codexIdentityMapValue(values map[string]any, names []string) string {
	for _, name := range names {
		if raw, ok := values[name].(string); ok && strings.TrimSpace(raw) != "" {
			return strings.TrimSpace(raw)
		}
	}
	return ""
}

func syncCodexIdentityAliasesMap(values map[string]any, scoped map[string]string) bool {
	changed := false
	for _, group := range codexIdentityAliases {
		value := scoped[group.kind]
		if value == "" {
			continue
		}
		for _, name := range group.names {
			if old, exists := values[name]; exists && old != value {
				values[name] = value
				changed = true
			}
		}
	}
	return changed
}

func syncCodexIdentityAliasesHeaders(headers http.Header, scoped map[string]string) {
	for _, group := range codexIdentityAliases {
		value := scoped[group.kind]
		if value == "" {
			continue
		}
		for _, name := range group.names {
			if _, exists := headers[http.CanonicalHeaderKey(name)]; exists {
				headers.Set(name, value)
			}
		}
	}
}

func applyCodexIdentityProjectionMetadata(values map[string]any, scoped map[string]string) bool {
	changed := syncCodexIdentityAliasesMap(values, scoped)
	embedded := codexIdentityMetadata(values)
	if syncCodexIdentityAliasesMap(embedded, scoped) {
		if encoded, err := json.Marshal(embedded); err == nil {
			values[openai.WSTurnMetadataHeader] = string(encoded)
			changed = true
		}
	}
	return changed
}

func stageAndApplyCodexAccountIdentity(c *gin.Context, account *gatewayprovider.ExecutionProvider, apiKeyID int64, body map[string]any) bool {
	namespace := provideradapter.CodexIdentityNamespace(CodexIdentityRecord(c, account.View()))
	if body == nil || namespace == "" {
		return false
	}
	metadata, _ := body["client_metadata"].(map[string]any)
	embedded := codexIdentityMetadata(metadata)
	headerValues := map[string]any{}
	if c.Request != nil {
		for _, group := range codexIdentityAliases {
			for _, name := range group.names {
				if value := strings.TrimSpace(c.GetHeader(name)); value != "" {
					headerValues[name] = value
				}
			}
		}
		headerValues[openai.WSTurnMetadataHeader] = c.GetHeader(openai.WSTurnMetadataHeader)
	}
	headerEmbedded := codexIdentityMetadata(headerValues)
	projection := &codexAccountIdentityProjection{namespace: namespace, apiKeyID: apiKeyID, raw: map[string]string{}, scoped: map[string]string{}}
	cacheKey, _ := body["prompt_cache_key"].(string)
	cacheKey = strings.TrimSpace(cacheKey)
	cacheUsesSession := false
	for _, group := range codexIdentityAliases {
		// 明确的会话头优先，其次正文，最后嵌套元数据。冲突别名统一到此选择。
		for _, source := range []map[string]any{headerValues, metadata, headerEmbedded, embedded} {
			raw := codexIdentityMapValue(source, group.names)
			if projection.raw[group.kind] == "" {
				projection.raw[group.kind] = raw
			}
			if group.kind == "session" && cacheKey != "" {
				for _, name := range group.names {
					if value, ok := source[name].(string); ok && strings.TrimSpace(value) == cacheKey {
						cacheUsesSession = true
					}
				}
			}
		}
	}
	if projection.raw["session"] == "" {
		projection.raw["session"] = cacheKey
		cacheUsesSession = cacheKey != ""
	}
	for kind, raw := range projection.raw {
		projection.scoped[kind] = openai.ScopeCodexProviderIdentityValue(namespace, apiKeyID, kind, raw)
	}
	c.Set(codexAccountIdentityProjectionContextKey, projection)
	changed := applyCodexIdentityProjectionMetadata(metadata, projection.scoped)
	if cacheKey != "" {
		next := openai.ScopeCodexProviderIdentityValue(namespace, apiKeyID, "prompt-cache", cacheKey)
		if cacheUsesSession {
			next = projection.scoped["session"]
		}
		if next != cacheKey {
			body["prompt_cache_key"] = next
			changed = true
		}
	}
	return changed
}

// 账号切换与租户边界必须同时匹配，不能读取其他 attempt 的旧快照。
func stagedCodexAccountIdentityProjection(c *gin.Context, account *gatewayprovider.ExecutionProvider, apiKeyID int64) *codexAccountIdentityProjection {
	if c == nil {
		return nil
	}
	value, _ := c.Get(codexAccountIdentityProjectionContextKey)
	projection, _ := value.(*codexAccountIdentityProjection)
	if projection == nil || projection.namespace != provideradapter.CodexIdentityNamespace(CodexIdentityRecord(c, account.View())) || projection.apiKeyID != apiKeyID {
		return nil
	}
	return projection
}

// resolveCodexFingerprintIDs 从本次请求的隔离会话派生线程，正文与 Header 共用结果。
func resolveCodexFingerprintIDs(c *gin.Context, value *gatewayprovider.ExecutionProvider) *openai.FingerprintIDs {
	if value == nil {
		return nil
	}
	source := CodexIdentityRecord(c, value.View())
	if projection := stagedCodexAccountIdentityProjection(c, value, APIKeyIDFromContext(c)); projection != nil && projection.scoped["session"] != "" {
		ids := provideradapter.CodexFingerprintIDs(source, projection.scoped["session"], source.GetCodexFingerprintMode())
		if ids != nil {
			// 与会话相同的缓存键随收敛会话更新，独立缓存键具有不同的隔离值。
			ids.OriginalBodySessionID = projection.scoped["session"]
			ids.OriginalBodySessionIDCaptured = true
		}
		return ids
	}
	var headers http.Header
	if c != nil && c.Request != nil {
		headers = c.Request.Header
	}
	return provideradapter.CodexFingerprintIDsFromRequest(source, headers)
}

func applyStagedCodexAccountIdentityHeaders(c *gin.Context, account *gatewayprovider.ExecutionProvider, apiKeyID int64, headers http.Header) {
	projection := stagedCodexAccountIdentityProjection(c, account, apiKeyID)
	if projection == nil || headers == nil {
		return
	}
	syncCodexIdentityAliasesHeaders(headers, projection.scoped)
	if session := projection.scoped["session"]; session != "" {
		headers.Set("session_id", session)
		// conversation_id 仅在原来采用会话回退时同步，保留独立 conversation 的语义。
		originalConversation := strings.TrimSpace(c.GetHeader("conversation_id"))
		if headers.Get("conversation_id") != "" {
			if originalConversation == "" || originalConversation == projection.raw["session"] {
				headers.Set("conversation_id", session)
			} else {
				headers.Set("conversation_id", openai.IsolateOpenAIUpstreamSessionID(apiKeyID, projection.namespace, originalConversation))
			}
		}
	}
	embedded := map[string]any{openai.WSTurnMetadataHeader: headers.Get(openai.WSTurnMetadataHeader)}
	if applyCodexIdentityProjectionMetadata(embedded, projection.scoped) {
		if value, ok := embedded[openai.WSTurnMetadataHeader].(string); ok {
			headers.Set(openai.WSTurnMetadataHeader, value)
		}
	}
}

// ApplyCodexIdentityMap 同步本次尝试的头部和正文身份字段。
func ApplyCodexIdentityMap(c *gin.Context, p *gatewayprovider.ExecutionProvider, key int64, body map[string]any) bool {
	return stageAndApplyCodexAccountIdentity(c, p, key, body)
}

// ApplyCodexIdentityRaw 更新小型身份对象，输入历史保持原字节形式。
func ApplyCodexIdentityRaw(c *gin.Context, p *gatewayprovider.ExecutionProvider, key int64, body []byte) ([]byte, bool, error) {
	metadata := map[string]any{}
	if raw := gjson.GetBytes(body, "client_metadata"); raw.IsObject() {
		if err := json.Unmarshal([]byte(raw.Raw), &metadata); err != nil {
			return body, false, err
		}
	}
	value := map[string]any{"client_metadata": metadata, "prompt_cache_key": gjson.GetBytes(body, "prompt_cache_key").String()}
	changed := stageAndApplyCodexAccountIdentity(c, p, key, value)
	if !changed {
		return body, false, nil
	}
	rebuilt, err := json.Marshal(metadata)
	if err != nil {
		return body, false, err
	}
	next, err := sjson.SetRawBytes(body, "client_metadata", rebuilt)
	if err != nil {
		return body, false, err
	}
	if cache, ok := value["prompt_cache_key"].(string); ok && cache != "" {
		next, err = sjson.SetBytes(next, "prompt_cache_key", cache)
	}
	return next, true, err
}

// ApplyStagedCodexIdentityHeaders 为 WebSocket 建连同步本次尝试的身份。
func ApplyStagedCodexIdentityHeaders(c *gin.Context, p *gatewayprovider.ExecutionProvider, key int64, headers http.Header) {
	applyStagedCodexAccountIdentityHeaders(c, p, key, headers)
}
