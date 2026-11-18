package completion

import (
	"context"
	"fmt"

	"github.com/TokenFlux/TokenRouter/internal/billing"
)

type (
	ProviderActivity interface{ ScheduleLastUsedUpdate(int64) }
	AuthInvalidator  interface{ InvalidateAuthCacheByKey(context.Context, string) }
)

// CommitEffects 按 billing 规定的顺序更新已提交资金的缓存并发送通知。
type CommitEffects struct {
	Funds         billing.SettlementEffects
	Activity      ProviderActivity
	Auth          AuthInvalidator
	Notifications *billing.BalanceNotifyService
	Observe       func(string, string)
}

func (e *CommitEffects) ProviderUsed(id int64) { e.Activity.ScheduleLastUsedUpdate(id) }
func (e *CommitEffects) InvalidateAuth(ctx context.Context, key string) {
	if e.Auth != nil {
		e.Auth.InvalidateAuthCacheByKey(ctx, key)
	}
}

func (e *CommitEffects) Settled(p SettlementInput, result *billing.UsageBillingApplyResult) {
	effects := e.Funds
	effects.ProviderUsed = func() { e.ProviderUsed(p.Provider.ID) }
	effects.NotifyBalance = func() { e.NotifyBalance(p, result) }
	effects.NotifyProvider = func() { e.NotifyProvider(p, result) }
	in := billing.SettlementEffectInput{Cost: p.Cost, Result: result, HasUser: p.User != nil}
	if p.User != nil {
		in.UserID = p.User.ID
	}
	if p.APIKey != nil {
		in.KeyID = p.APIKey.ID
		in.HasKeyRateLimits = p.APIKey.HasRateLimits
	}
	effects.Finalize(in)
}

func (e *CommitEffects) recoverNotification(name string) {
	if v := recover(); v != nil && e.Observe != nil {
		e.Observe(name, fmt.Sprint(v))
	}
}

// NotifyBalance 在结算提交后发送余额阈值通知。
func (e *CommitEffects) NotifyBalance(p SettlementInput, result *billing.UsageBillingApplyResult) {
	defer e.recoverNotification("notifyBalanceLow")
	if result == nil || result.BalanceAmountUSD <= 0 || p.User == nil || e.Notifications == nil {
		return
	}
	e.Notifications.CheckBalanceAfterDeduction(context.Background(), p.User.Notification, billing.BalanceBeforeSettlement(p.User.Balance, result), result.BalanceAmountUSD)
}

// NotifyProvider 按提供商成本发送通知，优先使用事务返回的额度状态。
func (e *CommitEffects) NotifyProvider(p SettlementInput, result *billing.UsageBillingApplyResult) {
	defer e.recoverNotification("notifyProviderQuota")
	if p.Cost.TotalCost <= 0 || p.Provider == nil || !p.Provider.QuotaEligible || e.Notifications == nil {
		return
	}
	var state *billing.ProviderQuotaState
	if result != nil {
		state = result.QuotaState
	}
	e.Notifications.CheckProviderQuotaAfterIncrement(context.Background(), p.Provider.Notification, p.Cost.TotalCost*p.ProviderRateMultiplier, state)
}
