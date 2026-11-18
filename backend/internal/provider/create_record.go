package provider

import (
	"context"
	"errors"
	"time"
)

// CreationOptions 提供创建账号所需的时钟、时区、指纹种子和模型设置。
type CreationOptions struct {
	Now                   func() time.Time
	LoadLocation          func(string) (*time.Location, error)
	NewSeed               func() string
	ReadOpenAIOAuthModels OpenAIOAuthModelPolicyReader
}

func BuildProviderForCreate(ctx context.Context, input *CreateProviderInput, providerExtra map[string]any, options CreationOptions) (*Record, error) {
	// 受管会话状态由系统维护，废弃字段不得通过通用提供商接口写入。
	DiscardDeprecatedProviderExtra(providerExtra)
	if err := NormalizeUpstreamUsageExtra(providerExtra); err != nil {
		return nil, err
	}
	delete(providerExtra, OllamaCloudUsageSessionExtraKey)
	delete(providerExtra, OllamaCloudUsageAutoRefreshExtraKey)
	delete(providerExtra, OllamaCloudUsageSnapshotExtraKey)
	delete(providerExtra, CNUsageMonitorSnapshotExtraKey)
	providerExtra = PrepareCodexFingerprintExtraForCreate(input.Platform, input.Type, providerExtra, options.NewSeed)
	provider := &Record{
		Now: options.Now, LoadLocation: options.LoadLocation,
		Name:        input.Name,
		Notes:       NormalizeProviderNotes(input.Notes),
		Platform:    input.Platform,
		Type:        input.Type,
		Credentials: input.Credentials,
		Extra:       providerExtra,
		ProxyID:     input.ProxyID,
		Concurrency: NormalizeProviderConcurrency(input.Platform, input.Type, input.Concurrency),
		Priority:    input.Priority,
		Status:      StatusActive,
		Schedulable: true,
	}
	if err := ApplyOpenAICreationModels(ctx, provider, options.ReadOpenAIOAuthModels); err != nil {
		return nil, err
	}
	if err := NormalizeCNProviderCredentials(provider, true); err != nil {
		return nil, err
	}
	if err := NormalizeOpenAIAPIKeyConfiguration(provider); err != nil {
		return nil, err
	}
	if err := NormalizeProviderProtocols(provider); err != nil {
		return nil, err
	}
	// 预计算固定时间重置的下次重置时间
	if provider.Extra != nil {
		if err := ValidateQuotaResetConfig(provider.Extra, options.LoadLocation); err != nil {
			return nil, err
		}
		ComputeQuotaResetAt(provider.Extra, options.Now(), options.LoadLocation)
		NormalizeFixedQuotaWindows(provider.Extra, options.Now(), options.LoadLocation)
	}
	if input.ExpiresAt != nil && *input.ExpiresAt > 0 {
		expiresAt := time.Unix(*input.ExpiresAt, 0)
		provider.ExpiresAt = &expiresAt
	}
	if input.AutoPauseOnExpired != nil {
		provider.AutoPauseOnExpired = *input.AutoPauseOnExpired
	} else {
		provider.AutoPauseOnExpired = true
	}
	if input.RateMultiplier != nil {
		if *input.RateMultiplier < 0 {
			return nil, errors.New("rate_multiplier must be >= 0")
		}
		provider.RateMultiplier = input.RateMultiplier
	}
	if input.LoadFactor != nil && *input.LoadFactor > 0 {
		if *input.LoadFactor > 10000 {
			return nil, errors.New("load_factor must be <= 10000")
		}
		provider.LoadFactor = input.LoadFactor
	}
	if err := ValidateGeminiThirdPartyBaseURL(provider); err != nil {
		return nil, err
	}
	return provider, nil
}
