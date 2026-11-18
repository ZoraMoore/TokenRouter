package completion

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestIndependentImageSettlementRates 核对独立图片倍率覆盖余额、订阅及套餐倍率。
func TestIndependentImageSettlementRates(t *testing.T) {
	key := &KeySnapshot{ID: 22, Group: &GroupSnapshot{
		ID: 8, RateMultiplier: 0.15, ImageRateIndependent: true, ImageRateMultiplier: 1,
	}}
	cost := &CostBreakdown{TotalCost: 0.03, ActualCost: 0.03, BillingMode: string(BillingModeImage)}
	subscription, balance, scale := RatesForMode(key, cost, 0.15, 0.13, time.Now())
	require.Equal(t, 1.0, subscription)
	require.Equal(t, 1.0, balance)
	command := BuildCommand("image-request", nil, &SettlementInput{
		Cost: cost, APIKey: key, User: &PayerSnapshot{ID: 11}, Provider: &ProviderSnapshot{ID: 33},
		SubscriptionRateMultiplier: subscription, BalanceRateMultiplier: balance, SubscriptionRateMultiplierScale: scale,
	})
	require.NotNil(t, command)
	require.True(t, command.DisablePlanGroupRateMultiplier)
	require.Equal(t, 1.0, command.BalanceRateMultiplier)
	// token 请求继续使用分组和用户的有效倍率。
	cost.BillingMode = string(BillingModeToken)
	subscription, balance, _ = RatesForMode(key, cost, 0.15, 0.13, time.Now())
	require.Equal(t, 0.15, subscription)
	require.Equal(t, 0.13, balance)
}
