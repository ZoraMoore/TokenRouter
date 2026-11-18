package promotion

import (
	"strconv"
)

// AdminReadSettings 包含推广开关和返利参数。
type AdminReadSettings struct {
	AdminRechargeRebateEnabled   bool
	AffiliateEnabled             bool
	AffiliateRebateDurationDays  int
	AffiliateRebateFreezeHours   int
	AffiliateRebatePerInviteeCap float64
	AffiliateRebateRate          float64
	InvitationCodeEnabled        bool
	PromoCodeEnabled             bool
}

// ReadAdminSettings 从传入的设置值解析推广开关和返利参数。
func ReadAdminSettings(settings map[string]string) *AdminReadSettings {
	result := &AdminReadSettings{}
	result.PromoCodeEnabled = settings[SettingKeyPromoCodeEnabled] != "false"
	result.InvitationCodeEnabled = settings[SettingKeyInvitationCodeEnabled] == "true"
	result.AffiliateEnabled = settings[SettingKeyAffiliateEnabled] == "true"
	if rebateRate, err := strconv.ParseFloat(settings[SettingKeyAffiliateRebateRate], 64); err == nil {
		result.AffiliateRebateRate = ClampRebateRate(rebateRate)
	} else {
		result.AffiliateRebateRate = AffiliateRebateRateDefault
	}
	if freezeHours, err := strconv.Atoi(settings[SettingKeyAffiliateRebateFreezeHours]); err == nil && freezeHours >= 0 {
		if freezeHours > AffiliateRebateFreezeHoursMax {
			freezeHours = AffiliateRebateFreezeHoursMax
		}
		result.AffiliateRebateFreezeHours = freezeHours
	}
	if durationDays, err := strconv.Atoi(settings[SettingKeyAffiliateRebateDurationDays]); err == nil && durationDays >= 0 {
		if durationDays > AffiliateRebateDurationDaysMax {
			durationDays = AffiliateRebateDurationDaysMax
		}
		result.AffiliateRebateDurationDays = durationDays
	}
	if perInviteeCap, err := strconv.ParseFloat(settings[SettingKeyAffiliateRebatePerInviteeCap], 64); err == nil && perInviteeCap >= 0 {
		result.AffiliateRebatePerInviteeCap = perInviteeCap
	}
	result.AdminRechargeRebateEnabled = settings[SettingKeyAffiliateAdminRechargeEnabled] == "true"
	return result
}
