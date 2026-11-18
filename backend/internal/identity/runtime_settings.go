package identity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
)

// 身份设置键和管理员凭据前缀沿用已有存储与认证协议。
const (
	AdminAPIKeyPrefix                             = "admin-"
	SettingKeyAdminAPIKey                         = "admin_api_key"
	SettingKeyAliyunCaptchaAccessKeyID            = "aliyun_captcha_access_key_id"
	SettingKeyAliyunCaptchaAccessKeySecret        = "aliyun_captcha_access_key_secret"
	SettingKeyAliyunCaptchaEnabled                = "aliyun_captcha_enabled"
	SettingKeyAliyunCaptchaRegion                 = "aliyun_captcha_region"
	SettingKeyAliyunCaptchaSceneID                = "aliyun_captcha_scene_id"
	SettingKeyDefaultUserAPIKeyLimit              = "default_user_api_key_limit"
	SettingKeyDefaultUserRPMLimit                 = "default_user_rpm_limit"
	SettingKeyEmailVerifyEnabled                  = "email_verify_enabled"
	SettingKeyPasswordResetEnabled                = "password_reset_enabled"
	SettingKeyRegistrationEmailDomainQuotaEnabled = "registration_email_domain_quota_enabled"
	SettingKeyRegistrationEmailNormalization      = "registration_email_normalization"
	SettingKeyRegistrationEmailSuffixWhitelist    = "registration_email_suffix_whitelist"
	SettingKeyRegistrationEnabled                 = "registration_enabled"
	SettingKeySessionBindingEnabled               = "session_binding_enabled"
	SettingKeyStepUpEnabled                       = "step_up_enabled"
	SettingKeyTencentCaptchaAppID                 = "tencent_captcha_app_id"
	SettingKeyTencentCaptchaAppSecretKey          = "tencent_captcha_app_secret_key"
	SettingKeyTencentCaptchaCloudSecretID         = "tencent_captcha_cloud_secret_id"
	SettingKeyTencentCaptchaCloudSecretKey        = "tencent_captcha_cloud_secret_key"
	SettingKeyTencentCaptchaEnabled               = "tencent_captcha_enabled"
	SettingKeyTencentCaptchaRegion                = "tencent_captcha_region"
	SettingKeyTotpEnabled                         = "totp_enabled"
	SettingKeyTurnstileEnabled                    = "turnstile_enabled"
	SettingKeyTurnstileSecretKey                  = "turnstile_secret_key"
	SettingKeyUserEmailChangeEnabled              = "user_email_change_enabled"
)

// RuntimeSettingsStore 提供身份设置的按键读取、批量读取、写入和删除。
type RuntimeSettingsStore interface {
	GetValue(context.Context, string) (string, error)
	GetMultiple(context.Context, []string) (map[string]string, error)
	Set(context.Context, string, string) error
	Delete(context.Context, string) error
}

// RuntimeSettings 读取并解析注册、安全和验证码设置。
type RuntimeSettings struct {
	settingRepo RuntimeSettingsStore
	notFound    error
}

// NewRuntimeSettings 绑定设置存储，动态值在调用读取方法时查询。
func NewRuntimeSettings(repo RuntimeSettingsStore, notFound error) *RuntimeSettings {
	return &RuntimeSettings{settingRepo: repo, notFound: notFound}
}

// IsRegistrationEnabled 读取注册开关，读取失败时返回 false。
func (s *RuntimeSettings) IsRegistrationEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyRegistrationEnabled)
	if err != nil {
		// 安全默认：如果设置不存在或查询出错，默认关闭注册
		return false
	}
	return value == "true"
}

// IsEmailVerifyEnabled 读取邮箱验证开关，读取失败时返回 false。
func (s *RuntimeSettings) IsEmailVerifyEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyEmailVerifyEnabled)
	if err != nil {
		return false
	}
	return value == "true"
}

// IsRegistrationEmailDomainQuotaEnabled 读取注册邮箱域名配额开关，读取失败时返回 false。
func (s *RuntimeSettings) IsRegistrationEmailDomainQuotaEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyRegistrationEmailDomainQuotaEnabled)
	if err != nil {
		return false
	}
	return value == "true"
}

// IsUserEmailChangeEnabled 读取邮箱换绑开关，读取失败时返回 false。
func (s *RuntimeSettings) IsUserEmailChangeEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyUserEmailChangeEnabled)
	if err != nil {
		return false
	}
	return value == "true"
}

// GetRegistrationEmailSuffixWhitelist 读取邮箱后缀白名单，读取失败时返回空列表。
func (s *RuntimeSettings) GetRegistrationEmailSuffixWhitelist(ctx context.Context) []string {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyRegistrationEmailSuffixWhitelist)
	if err != nil {
		return []string{}
	}
	return ParseRegistrationEmailSuffixWhitelist(value)
}

// IsPasswordResetEnabled 检查邮箱验证和密码重置开关均已开启。
func (s *RuntimeSettings) IsPasswordResetEnabled(ctx context.Context) bool {
	// Password reset requires email verification to be enabled
	if !s.IsEmailVerifyEnabled(ctx) {
		return false
	}
	value, err := s.settingRepo.GetValue(ctx, SettingKeyPasswordResetEnabled)
	if err != nil {
		return false // 默认关闭
	}
	return value == "true"
}

// IsTotpEnabled 读取 TOTP 开关，读取失败时返回 false。
func (s *RuntimeSettings) IsTotpEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyTotpEnabled)
	if err != nil {
		return false // 默认关闭
	}
	return value == "true"
}

// IsSessionBindingEnabled 读取会话绑定开关，读取失败时返回 false。
func (s *RuntimeSettings) IsSessionBindingEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeySessionBindingEnabled)
	if err != nil {
		return false // 默认关闭
	}
	return value == "true"
}

// IsStepUpEnabled 读取二次验证开关，读取失败时返回 false。
func (s *RuntimeSettings) IsStepUpEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyStepUpEnabled)
	if err != nil {
		return false // 默认关闭
	}
	return value == "true"
}

// GetDefaultUserRPMLimit 读取默认用户 RPM 上限，读取失败或值非法时返回零。
func (s *RuntimeSettings) GetDefaultUserRPMLimit(ctx context.Context) int {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyDefaultUserRPMLimit)
	if err != nil || value == "" {
		return 0
	}
	if v, err := strconv.Atoi(value); err == nil && v >= 0 {
		return v
	}
	return 0
}

// GetDefaultUserAPIKeyLimit 读取默认 Key 数量上限，缺少设置或值非法时返回默认值。
func (s *RuntimeSettings) GetDefaultUserAPIKeyLimit(ctx context.Context) int {
	if s == nil || s.settingRepo == nil {
		return DefaultUserAPIKeyLimit
	}
	value, err := s.settingRepo.GetValue(ctx, SettingKeyDefaultUserAPIKeyLimit)
	if err != nil || value == "" {
		return DefaultUserAPIKeyLimit
	}
	if limit, err := strconv.Atoi(value); err == nil && IsValidUserAPIKeyLimit(limit) {
		return limit
	}
	return DefaultUserAPIKeyLimit
}

// IsTurnstileEnabled 读取 Turnstile 开关，读取失败时返回 false。
func (s *RuntimeSettings) IsTurnstileEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyTurnstileEnabled)
	if err != nil {
		return false
	}
	return value == "true"
}

// GetTurnstileSecretKey 读取 Turnstile 密钥，读取失败时返回空字符串。
func (s *RuntimeSettings) GetTurnstileSecretKey(ctx context.Context) string {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyTurnstileSecretKey)
	if err != nil {
		return ""
	}
	return value
}

// GetCaptchaProviderConfig 批量读取并解析验证码提供方设置。
func (s *RuntimeSettings) GetCaptchaProviderConfig(ctx context.Context) (CaptchaProviderConfig, error) {
	values, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingKeyTurnstileEnabled,
		SettingKeyTurnstileSecretKey,
		SettingKeyTencentCaptchaEnabled,
		SettingKeyTencentCaptchaAppID,
		SettingKeyTencentCaptchaAppSecretKey,
		SettingKeyTencentCaptchaCloudSecretID,
		SettingKeyTencentCaptchaCloudSecretKey,
		SettingKeyTencentCaptchaRegion,
		SettingKeyAliyunCaptchaEnabled,
		SettingKeyAliyunCaptchaAccessKeyID,
		SettingKeyAliyunCaptchaAccessKeySecret,
		SettingKeyAliyunCaptchaSceneID,
		SettingKeyAliyunCaptchaRegion,
	})
	if err != nil {
		return CaptchaProviderConfig{}, fmt.Errorf("read captcha provider settings: %w", err)
	}
	return CaptchaProviderConfig{
		TurnstileEnabled:   values[SettingKeyTurnstileEnabled] == "true",
		TurnstileSecretKey: values[SettingKeyTurnstileSecretKey],
		Tencent: TencentCaptchaConfig{
			Enabled:        values[SettingKeyTencentCaptchaEnabled] == "true",
			AppID:          values[SettingKeyTencentCaptchaAppID],
			AppSecretKey:   values[SettingKeyTencentCaptchaAppSecretKey],
			CloudSecretID:  values[SettingKeyTencentCaptchaCloudSecretID],
			CloudSecretKey: values[SettingKeyTencentCaptchaCloudSecretKey],
			Region:         NormalizeTencentCaptchaRegion(values[SettingKeyTencentCaptchaRegion]),
		},
		Aliyun: AliyunCaptchaConfig{
			Enabled:         values[SettingKeyAliyunCaptchaEnabled] == "true",
			AccessKeyID:     values[SettingKeyAliyunCaptchaAccessKeyID],
			AccessKeySecret: values[SettingKeyAliyunCaptchaAccessKeySecret],
			SceneID:         values[SettingKeyAliyunCaptchaSceneID],
			Region:          NormalizeAliyunCaptchaRegion(values[SettingKeyAliyunCaptchaRegion]),
		},
	}, nil
}

// GetTencentCaptchaConfig 读取腾讯验证码配置，读取失败时返回零值。
func (s *RuntimeSettings) GetTencentCaptchaConfig(ctx context.Context) TencentCaptchaConfig {
	config, err := s.GetCaptchaProviderConfig(ctx)
	if err != nil {
		return TencentCaptchaConfig{}
	}
	return config.Tencent
}

// GenerateAdminAPIKey 生成带 admin- 前缀的随机密钥并保存到设置表。
func (s *RuntimeSettings) GenerateAdminAPIKey(ctx context.Context) (string, error) {
	// 生成 32 字节随机数 = 64 位十六进制字符
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}

	key := AdminAPIKeyPrefix + hex.EncodeToString(bytes)

	// 存储到 settings 表
	if err := s.settingRepo.Set(ctx, SettingKeyAdminAPIKey, key); err != nil {
		return "", fmt.Errorf("save admin api key: %w", err)
	}

	return key, nil
}

// GetAdminAPIKeyStatus 查询管理员密钥是否存在，并返回掩码值。
func (s *RuntimeSettings) GetAdminAPIKeyStatus(ctx context.Context) (maskedKey string, exists bool, err error) {
	key, err := s.settingRepo.GetValue(ctx, SettingKeyAdminAPIKey)
	if err != nil {
		if errors.Is(err, s.notFound) {
			return "", false, nil
		}
		return "", false, err
	}
	if key == "" {
		return "", false, nil
	}

	// 脱敏：显示前 10 位和后 4 位
	if len(key) > 14 {
		maskedKey = key[:10] + "..." + key[len(key)-4:]
	} else {
		maskedKey = key
	}

	return maskedKey, true, nil
}

// GetAdminAPIKey 读取管理员密钥，未配置时返回空字符串。
func (s *RuntimeSettings) GetAdminAPIKey(ctx context.Context) (string, error) {
	key, err := s.settingRepo.GetValue(ctx, SettingKeyAdminAPIKey)
	if err != nil {
		if errors.Is(err, s.notFound) {
			return "", nil // 未配置，返回空字符串
		}
		return "", err // 数据库错误
	}
	return key, nil
}

// DeleteAdminAPIKey 删除已保存的管理员密钥。
func (s *RuntimeSettings) DeleteAdminAPIKey(ctx context.Context) error {
	return s.settingRepo.Delete(ctx, SettingKeyAdminAPIKey)
}

// IsRegistrationEmailNormalizationEnabled 读取注册邮箱归一化开关，读取失败时返回 false。
func (s *RuntimeSettings) IsRegistrationEmailNormalizationEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyRegistrationEmailNormalization)
	if err != nil {
		return false
	}
	return value == "true"
}
