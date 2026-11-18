package provider

import (
	"context"
	"maps"

	"github.com/TokenFlux/TokenRouter/internal/egress"
)

func (s *Admin) CreateProvider(ctx context.Context, input *CreateProviderInput) (*Record, error) {
	providerExtra := maps.Clone(input.Extra)
	DiscardDeprecatedProviderExtra(providerExtra)
	if err := NormalizeUpstreamUsageExtra(providerExtra); err != nil {
		return nil, err
	}
	providerExtra, err := NormalizeGrokMediaEligibilityExtra(input.Platform, providerExtra)
	if err != nil {
		return nil, err
	}
	if err := ValidateUpstreamRequestIDHeaderExtra(providerExtra); err != nil {
		return nil, err
	}

	// 绑定分组
	groupIDs := input.GroupIDs

	// 校验并规范化请求头覆写配置（header 名小写化、格式检查）
	if err := egress.NormalizeHeaderOverrideCredentials(input.Credentials); err != nil {
		return nil, err
	}
	// OAuth 兑换后不得持久化临时 SSO 或密码。
	input.Credentials = SanitizeStoredCredentials(input.Platform, input.Credentials)

	provider, err := BuildProviderForCreate(input, providerExtra, s.options.Creation)
	if err != nil {
		return nil, err
	}
	// 创建时生成并保存机器身份，编辑时按兼容规则读取已有身份。
	if provider.IsQoderCosy() {
		s.options.Credentials.Prepare(provider)
	}
	s.attachProxyForValidation(ctx, provider)
	if err := s.options.Credentials.Validate(ctx, provider); err != nil {
		return nil, err
	}
	if err := s.providerRepo.Create(ctx, provider); err != nil {
		return nil, err
	}

	// 绑定分组
	if len(groupIDs) > 0 {
		if err := s.providerRepo.BindGroups(ctx, provider.ID, groupIDs); err != nil {
			return nil, err
		}
	}

	// 后置任务使用自己的提供商值，不与返回给 HTTP 的可变对象共享。
	if provider.Type == ProviderTypeOAuth && (provider.Platform == PlatformOpenAI || provider.Platform == PlatformAntigravity) {
		value := CloneRecord(provider)
		s.options.Background("service/admin_provider.go:CreateProvider", func() {
			defer func() {
				if r := recover(); r != nil {
					event := "create_provider_openai_privacy_panic"
					if value.Platform == PlatformAntigravity {
						event = "create_provider_antigravity_privacy_panic"
					}
					s.options.Error(event, "provider_id", value.ID, "recover", r)
				}
			}()
			if value.Platform == PlatformOpenAI {
				s.options.Privacy.EnsureOpenAIPrivacy(context.Background(), value)
			} else {
				s.options.Privacy.EnsureAntigravityPrivacy(context.Background(), value)
			}
		})
	}

	return provider, nil
}

func (s *Admin) attachProxyForValidation(ctx context.Context, value *Record) {
	if s.options.Proxies == nil || value == nil || value.Proxy != nil || value.ProxyID == nil || *value.ProxyID <= 0 {
		return
	}
	if proxy, err := s.options.Proxies.GetByID(ctx, *value.ProxyID); err == nil && proxy != nil {
		value.Proxy = proxy
	}
}
