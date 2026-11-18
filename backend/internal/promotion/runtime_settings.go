package promotion

import (
	"context"
	"math"
	"strconv"
	"strings"
)

// 推广设置键沿用既有数据库格式，由推广模块拥有解释权。
const (
	SettingKeyAffiliateAdminRechargeEnabled = "affiliate_admin_recharge_enabled"
	SettingKeyAffiliateEnabled              = "affiliate_enabled"
	SettingKeyAffiliateRebateDurationDays   = "affiliate_rebate_duration_days"
	SettingKeyAffiliateRebateFreezeHours    = "affiliate_rebate_freeze_hours"
	SettingKeyAffiliateRebatePerInviteeCap  = "affiliate_rebate_per_invitee_cap"
	SettingKeyAffiliateRebateRate           = "affiliate_rebate_rate"
	SettingKeyInvitationCodeEnabled         = "invitation_code_enabled"
	SettingKeyPromoCodeEnabled              = "promo_code_enabled"
)

// RuntimeSettingsStore 按键读取推广设置。
type RuntimeSettingsStore interface {
	GetValue(context.Context, string) (string, error)
}

// RuntimeSettings 在每次调用时读取推广设置。
type RuntimeSettings struct{ settingRepo RuntimeSettingsStore }

// NewRuntimeSettings 构造无副作用的推广设置读取器。
func NewRuntimeSettings(repo RuntimeSettingsStore) *RuntimeSettings {
	return &RuntimeSettings{settingRepo: repo}
}

// IsPromoCodeEnabled 读取优惠码开关，读取失败时默认启用。
func (s *RuntimeSettings) IsPromoCodeEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyPromoCodeEnabled)
	if err != nil {
		return true // 默认启用
	}
	return value != "false"
}

// IsInvitationCodeEnabled 读取邀请码开关，读取失败时默认关闭。
func (s *RuntimeSettings) IsInvitationCodeEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyInvitationCodeEnabled)
	if err != nil {
		return false // 默认关闭
	}
	return value == "true"
}

// IsAffiliateEnabled 读取推广开关，读取失败时返回默认值。
func (s *RuntimeSettings) IsAffiliateEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyAffiliateEnabled)
	if err != nil {
		return AffiliateEnabledDefault
	}
	return value == "true"
}

// IsAffiliateAdminRechargeEnabled 读取管理员充值返利开关，读取失败时返回默认值。
func (s *RuntimeSettings) IsAffiliateAdminRechargeEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyAffiliateAdminRechargeEnabled)
	if err != nil {
		return AdminRechargeRebateEnabledDefault
	}
	return value == "true"
}

// GetAffiliateRebateRatePercent 读取返利比例，读取或解析失败时返回默认值。
func (s *RuntimeSettings) GetAffiliateRebateRatePercent(ctx context.Context) float64 {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyAffiliateRebateRate)
	if err != nil {
		return AffiliateRebateRateDefault
	}
	rate, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return AffiliateRebateRateDefault
	}
	return ClampRebateRate(rate)
}

// GetAffiliateRebateFreezeHours 读取返利冻结小时数，读取失败或值非法时返回默认值。
func (s *RuntimeSettings) GetAffiliateRebateFreezeHours(ctx context.Context) int {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyAffiliateRebateFreezeHours)
	if err != nil {
		return AffiliateRebateFreezeHoursDefault
	}
	hours, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || hours < 0 {
		return AffiliateRebateFreezeHoursDefault
	}
	if hours > AffiliateRebateFreezeHoursMax {
		return AffiliateRebateFreezeHoursMax
	}
	return hours
}

// GetAffiliateRebateDurationDays 读取返利有效天数，读取失败或值非法时返回默认值。
func (s *RuntimeSettings) GetAffiliateRebateDurationDays(ctx context.Context) int {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyAffiliateRebateDurationDays)
	if err != nil {
		return AffiliateRebateDurationDaysDefault
	}
	days, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || days < 0 {
		return AffiliateRebateDurationDaysDefault
	}
	if days > AffiliateRebateDurationDaysMax {
		return AffiliateRebateDurationDaysMax
	}
	return days
}

// GetAffiliateRebatePerInviteeCap 读取每位受邀人的返利上限，读取失败或值非法时返回默认值。
func (s *RuntimeSettings) GetAffiliateRebatePerInviteeCap(ctx context.Context) float64 {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyAffiliateRebatePerInviteeCap)
	if err != nil {
		return AffiliateRebatePerInviteeCapDefault
	}
	capValue, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || capValue < 0 || math.IsNaN(capValue) || math.IsInf(capValue, 0) {
		return AffiliateRebatePerInviteeCapDefault
	}
	return capValue
}
