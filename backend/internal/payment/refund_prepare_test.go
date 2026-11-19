package payment

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/billing"
	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

type couponRefundOrderStore struct {
	RefundStore
	order *Order
}

func (s couponRefundOrderStore) Order(_ context.Context, _ int64) (*Order, error) {
	return s.order, nil
}

func TestPrepareRefundForRechargeCouponOrder(t *testing.T) {
	order := &Order{
		ID: 1, UserID: 2, Amount: 200, PayAmount: 80, Status: OrderStatusCompleted, OrderType: OrderTypeBalance,
		CouponSnapshot: &billing.RechargeCouponSnapshot{RechargeCouponTerms: billing.RechargeCouponTerms{Currency: "CNY"}, CouponID: 3},
	}
	workflow := NewRefundWorkflow(couponRefundOrderStore{order: order}, RefundRuntime{
		Instance: func(context.Context, *Order) (*ProviderInstance, error) {
			return &ProviderInstance{RefundEnabled: true}, nil
		},
		User: func(context.Context, int64) (*RefundUser, error) {
			return &RefundUser{Balance: 200}, nil
		},
	})

	t.Run("部分退款按到账比例计算实付", func(t *testing.T) {
		plan, _, err := workflow.PrepareRefund(t.Context(), order.ID, 50, "", false, true)
		require.NoError(t, err)
		require.Equal(t, 50.0, plan.BalanceToDeduct)
		require.Equal(t, 20.0, plan.GatewayAmount)
	})
	t.Run("全额退款只退实收金额", func(t *testing.T) {
		plan, _, err := workflow.PrepareRefund(t.Context(), order.ID, 0, "", false, true)
		require.NoError(t, err)
		require.Equal(t, 200.0, plan.BalanceToDeduct)
		require.Equal(t, 80.0, plan.GatewayAmount)
	})
	t.Run("退款拒绝亚分精度", func(t *testing.T) {
		_, _, err := workflow.PrepareRefund(t.Context(), order.ID, 50.001, "", false, true)
		require.Equal(t, "INVALID_AMOUNT", apperror.Reason(err))
	})
}
