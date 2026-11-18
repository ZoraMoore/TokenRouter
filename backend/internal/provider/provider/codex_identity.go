package provider

import (
	"net/http"
	"strings"

	providercore "github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
	"github.com/TokenFlux/TokenRouter/internal/upstream/openai"
)

// CodexIdentityNamespace 在构造 Header 和 metadata 时读取当前凭据并计算命名空间。
func CodexIdentityNamespace(provider *providercore.Record) string {
	if provider == nil || !provider.IsOpenAIOAuthLike() {
		return ""
	}
	upstreamProviderID := strings.TrimSpace(provider.GetChatGPTAccountID())
	if upstreamProviderID != "" {
		return openai.CodexProviderNamespace(openai.ProviderIdentityInput{ChatGPTAccountID: upstreamProviderID, ChatGPTUserID: strings.TrimSpace(provider.GetCredential("chatgpt_user_id"))})
	}
	if seed, ok := providercore.CodexFingerprintSeed(provider.Extra); ok {
		return openai.CodexProviderNamespace(openai.ProviderIdentityInput{Seed: seed, HasSeed: true})
	}
	if provider.Type == capability.ProviderTypeSetupToken {
		return openai.CodexProviderNamespace(openai.ProviderIdentityInput{SetupToken: strings.TrimSpace(provider.GetOpenAIAccessToken())})
	}
	return ""
}

// CodexFingerprintIDsFromRequest 只解析本 attempt 的客户端会话，复用原指纹生成器。
func CodexFingerprintIDsFromRequest(value *providercore.Record, headers http.Header) *openai.FingerprintIDs {
	if value == nil {
		return nil
	}
	mode := value.GetCodexFingerprintMode()
	if mode == providercore.CodexFingerprintOff {
		return nil
	}
	session := ""
	if headers != nil {
		session = openai.ExtractClientSessionID(headers)
	}
	return CodexFingerprintIDs(value, session, mode)
}

// SetChatGPTAccountHeaders 保留 OAuth 资格和无 Header 的短路。
func SetChatGPTAccountHeaders(headers http.Header, value *providercore.Record) {
	if headers == nil || value == nil || !value.IsOpenAIOAuthLike() {
		return
	}
	openai.SetChatGPTAccountHeaders(headers, value.GetChatGPTAccountID(), value.IsChatGPTAccountFedRAMP())
}
