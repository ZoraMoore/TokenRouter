package billing

import (
	"context"
)

// SettlementEffectInput 包含已提交的结算金额和待更新的缓存标识。
type SettlementEffectInput struct {
	UserID, KeyID             int64
	HasUser, HasKeyRateLimits bool
	Cost                      *CostBreakdown
	Result                    *UsageBillingApplyResult
}

// SettlementEffects 是提交后调用的无状态端口集合，缓存队列由唯一 Eligibility 持有。
type SettlementEffects struct {
	Cache                         *Eligibility
	Background                    func(string, func()) bool
	Observe                       Observe
	BalanceWarning                func(int64, float64, error)
	ProviderUsed                  func()
	NotifyBalance, NotifyProvider func()
}

// Finalize 保留缓存、Key 窗口、提供商完成和通知的执行顺序。
func (e SettlementEffects) Finalize(input SettlementEffectInput) {
	if input.Cost == nil {
		return
	}
	if input.Result != nil && input.Result.BalanceAmountUSD > 0 && input.HasUser {
		e.SyncBalance(context.Background(), input.UserID, input.Result)
	}
	rateCost := input.Cost.ActualCost
	if input.Result != nil {
		rateCost = input.Result.SubscriptionAmountUSD + input.Result.BalanceAmountUSD
	}
	if rateCost > 0 && input.HasKeyRateLimits {
		e.Cache.QueueUpdateAPIKeyRateLimitUsage(input.KeyID, rateCost)
	}
	if e.ProviderUsed != nil {
		e.ProviderUsed()
	}
	if e.NotifyBalance != nil {
		e.launch("billing/settlement:notifyBalance", e.NotifyBalance)
	}
	if e.NotifyProvider != nil {
		e.launch("billing/settlement:notifyProvider", e.NotifyProvider)
	}
}

func (e SettlementEffects) launch(name string, fn func()) bool {
	if e.Background != nil {
		return e.Background(name, fn)
	}
	fn()
	return true
}

// SyncBalance 低于准入阈值时失效，其余情况保持原异步扣减。
func (e SettlementEffects) SyncBalance(ctx context.Context, id int64, result *UsageBillingApplyResult) {
	if e.Cache == nil || result == nil || result.BalanceAmountUSD <= 0 {
		return
	}
	if result.NewBalance != nil && e.Cache.BalanceBelowEligibilityThreshold(*result.NewBalance) {
		if err := e.Cache.InvalidateUserBalance(ctx, id); err != nil && e.BalanceWarning != nil {
			e.BalanceWarning(id, *result.NewBalance, err)
		}
		return
	}
	e.Cache.QueueDeductBalance(id, result.BalanceAmountUSD)
}

// BalanceBeforeSettlement 优先使用事务返回值还原扣费前余额。
func BalanceBeforeSettlement(snapshot float64, result *UsageBillingApplyResult) float64 {
	if result != nil && result.NewBalance != nil {
		return *result.NewBalance + result.BalanceAmountUSD
	}
	return snapshot
}
