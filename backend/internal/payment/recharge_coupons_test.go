package payment

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/billing"
	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

func TestQuoteRechargeCoupon(t *testing.T) {
	coupon := billing.RechargeCoupon{
		ID: 1,
		RechargeCouponTerms: billing.RechargeCouponTerms{
			Currency: "CNY", MinimumMinor: 10_000, DiscountMinor: 2_000,
		},
	}
	for _, input := range []struct {
		name, currency, reason string
		amount                 float64
	}{
		{name: "差一分", currency: "CNY", amount: 99.99, reason: "RECHARGE_COUPON_MINIMUM"},
		{name: "恰好达到门槛", currency: "CNY", amount: 100},
		{name: "币种不同", currency: "USD", amount: 100, reason: "RECHARGE_COUPON_CURRENCY"},
		{name: "人民币超出小数位", currency: "CNY", amount: 100.001, reason: "INVALID_AMOUNT"},
		{name: "金额超出精度", currency: "CNY", amount: 1e20, reason: "INVALID_AMOUNT"},
	} {
		t.Run(input.name, func(t *testing.T) {
			quoted, err := quoteRechargeCoupon(coupon, input.currency, input.amount)
			if input.reason != "" {
				require.Equal(t, input.reason, apperror.Reason(err))
				return
			}
			require.NoError(t, err)
			require.Equal(t, int64(10_000), quoted.RechargeMinor)
			require.Equal(t, int64(8_000), quoted.ChargeMinor)
			require.Equal(t, 200.0, CalculateCreditedBalance(input.amount, 2))
		})
	}
}

func TestPriceCheckoutCouponChargesFeesOnDiscountedAmount(t *testing.T) {
	coupon := &billing.RechargeCoupon{
		ID:                  1,
		RechargeCouponTerms: billing.RechargeCouponTerms{Currency: "CNY", MinimumMinor: 10_000, DiscountMinor: 2_000},
	}
	pricing, err := priceCheckout(CreateOrderRequest{OrderType: OrderTypeBalance, PaymentType: TypeAlipay}, &PaymentConfig{
		RechargeFeeRate: 2.5,
	}, 100, "CNY", coupon)
	require.NoError(t, err)
	require.Equal(t, 80.0, pricing.fee.BaseAmount)
	require.Equal(t, 2.0, pricing.fee.FeeRateAmount)
	require.Equal(t, 82.0, pricing.payAmount)
}

func TestCheckoutDailyLimitCountsCouponRechargeFaceValue(t *testing.T) {
	order := &Order{OrderType: OrderTypeBalance, Amount: 200, PayAmount: 80, CouponSnapshot: &billing.RechargeCouponSnapshot{
		RechargeCouponTerms: billing.RechargeCouponTerms{Currency: "CNY"}, RechargeMinor: 10_000,
	}}
	err := ValidateCheckoutDaily([]*Order{order}, 21, 120)
	require.Equal(t, "DAILY_LIMIT_EXCEEDED", apperror.Reason(err))
}
