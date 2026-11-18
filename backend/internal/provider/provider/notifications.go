package provider

import (
	"github.com/TokenFlux/TokenRouter/internal/billing"
	"github.com/TokenFlux/TokenRouter/internal/provider"
)

func QuotaNotification(a *provider.Record) *billing.QuotaNotifyProvider {
	if a == nil {
		return nil
	}
	return &billing.QuotaNotifyProvider{ID: a.ID, Name: a.Name, Platform: a.Platform, Dimensions: []billing.QuotaNotifyDimension{
		{Name: "daily", Enabled: a.GetQuotaNotifyDailyEnabled(), Threshold: a.GetQuotaNotifyDailyThreshold(), ThresholdType: a.GetQuotaNotifyDailyThresholdType(), CurrentUsed: a.GetQuotaDailyUsed(), Limit: a.GetQuotaDailyLimit()},
		{Name: "weekly", Enabled: a.GetQuotaNotifyWeeklyEnabled(), Threshold: a.GetQuotaNotifyWeeklyThreshold(), ThresholdType: a.GetQuotaNotifyWeeklyThresholdType(), CurrentUsed: a.GetQuotaWeeklyUsed(), Limit: a.GetQuotaWeeklyLimit()},
		{Name: "total", Enabled: a.GetQuotaNotifyTotalEnabled(), Threshold: a.GetQuotaNotifyTotalThreshold(), ThresholdType: a.GetQuotaNotifyTotalThresholdType(), CurrentUsed: a.GetQuotaUsed(), Limit: a.GetQuotaLimit()},
	}}
}
