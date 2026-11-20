//go:build integration

package payment_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	dbent "github.com/TokenFlux/TokenRouter/ent"
	"github.com/TokenFlux/TokenRouter/internal/billing"
	billingpostgres "github.com/TokenFlux/TokenRouter/internal/billing/postgres"
	"github.com/TokenFlux/TokenRouter/internal/payment"
	paymentpostgres "github.com/TokenFlux/TokenRouter/internal/payment/postgres"
)

type couponTestUserReader struct{ client *dbent.Client }

// GetByID 读取发券和到账所用的用户余额。
func (r couponTestUserReader) GetByID(ctx context.Context, id int64) (*billing.UserSummary, error) {
	user, err := r.client.User.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &billing.UserSummary{ID: user.ID, Balance: user.Balance}, nil
}

func TestRechargeCouponOrderCompetitionAndLatePayment(t *testing.T) {
	client := testEntClient(t)
	ctx := t.Context()
	user, err := client.User.Create().SetUsername("coupon-owner").SetEmail("coupon-owner@example.com").SetPasswordHash("hash").Save(ctx)
	require.NoError(t, err)
	redeemRepo := billingpostgres.NewRedeemCodeRepository(client)
	administrator := billing.NewRedeemAdmin(redeemRepo, billingpostgres.NewRedeemAdministrationMutations(client), nil)
	terms := billing.RechargeCouponTerms{Currency: "CNY", MinimumMinor: 10_000, DiscountMinor: 2_000}
	codes, err := administrator.GenerateRedeemCodes(ctx, &billing.GenerateRedeemCodesInput{
		Code: "integration-recharge-coupon", Type: billing.RedeemTypeCoupon, Count: 1, CouponTerms: &terms,
	})
	require.NoError(t, err)
	redeemer := billing.NewRedeemService(redeemRepo, couponTestUserReader{client}, nil, nil, nil, billingpostgres.NewRedeemMutations(client, billingpostgres.RedeemWriters{
		Balances: billingpostgres.NewBalanceStore(client),
	}), nil, nil, billing.RedeemRuntime{})
	claim, err := redeemer.Redeem(ctx, user.ID, codes[0].Code)
	require.NoError(t, err)
	require.NotNil(t, claim.Coupon)
	couponID := claim.Coupon.ID
	_, err = redeemer.Redeem(ctx, user.ID, codes[0].Code)
	require.Error(t, err)
	require.Equal(t, user.Balance, client.User.GetX(ctx, user.ID).Balance, "领券不增加余额")

	store := paymentpostgres.NewOrderStore(client)
	_, err = store.RechargeCoupon(ctx, user.ID+1, couponID)
	require.ErrorIs(t, err, billing.ErrRechargeCouponUnavailable)
	owned, total, err := store.ListRechargeCoupons(ctx, user.ID, true, 1, 20)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, owned, 1)
	require.Equal(t, couponID, owned[0].ID)

	draft := payment.CheckoutDraft{
		Order: payment.Order{
			UserID: user.ID, UserEmail: user.Email, UserName: user.Username,
			Amount: 200, PayAmount: 80, PaymentType: payment.TypeAlipay, OrderType: payment.OrderTypeBalance,
			CouponSnapshot: &billing.RechargeCouponSnapshot{
				RechargeCouponTerms: billing.RechargeCouponTerms{Currency: "CNY", MinimumMinor: 10000, DiscountMinor: 2000},
				CouponID:            couponID, RechargeMinor: 10000, ChargeMinor: 8000,
			},
		},
		MaxPending: 3, LimitAmount: 100, TimeoutMinutes: 30,
	}
	type checkoutResult struct {
		order *payment.Order
		err   error
	}
	concurrent := make(chan checkoutResult, 2)
	for range 2 {
		go func() {
			order, err := store.CreateCheckout(ctx, draft)
			concurrent <- checkoutResult{order: order, err: err}
		}()
	}
	var first *payment.Order
	for range 2 {
		result := <-concurrent
		if result.err != nil {
			require.ErrorIs(t, result.err, billing.ErrRechargeCouponUnavailable)
		} else {
			require.Nil(t, first, "优惠券只能分配给一笔订单")
			first = result.order
		}
	}
	require.NotNil(t, first)
	require.Equal(t, draft.Order.CouponSnapshot, first.CouponSnapshot)
	updated, err := store.TransitionOrder(ctx, payment.OrderTransition{
		ID: first.ID, From: []string{payment.OrderStatusPending}, Status: payment.OrderStatusCancelled, ReleaseCoupon: true,
	})
	require.NoError(t, err)
	require.Equal(t, 1, updated)
	second, err := store.CreateCheckout(ctx, draft)
	require.NoError(t, err)
	require.NoError(t, store.ForceExpire(ctx, second.ID, "test forced expiration"))
	coupon, err := store.RechargeCoupon(ctx, user.ID, couponID)
	require.NoError(t, err)
	require.NotNil(t, coupon.OrderID)
	require.Equal(t, second.ID, *coupon.OrderID)

	fulfillment := payment.NewFulfillment(store, nil, nil, redeemer, nil, payment.FulfillmentRuntime{})
	require.Error(t, fulfillment.ConfirmPayment(ctx, first, "late-trade", 79.99, payment.TypeAlipay, nil))
	require.NoError(t, fulfillment.ConfirmPayment(ctx, first, "late-trade", 80, payment.TypeAlipay, nil))
	late, err := store.Order(ctx, first.ID)
	require.NoError(t, err)
	require.Equal(t, payment.OrderStatusPaymentReview, late.Status)
	require.NotNil(t, late.PaidAt)
	require.True(t, store.HasAudit(ctx, first.ID, "RECHARGE_COUPON_PAYMENT_REVIEW"))
	require.Equal(t, user.Balance, client.User.GetX(ctx, user.ID).Balance, "迟到付款待核对期间不发放积分")
	require.NoError(t, fulfillment.ConfirmPayment(ctx, second, "reserved-trade", 80, payment.TypeAlipay, nil))
	require.NoError(t, fulfillment.ConfirmPayment(ctx, second, "reserved-trade", 80, payment.TypeAlipay, nil))
	settled, err := store.Order(ctx, second.ID)
	require.NoError(t, err)
	require.Equal(t, payment.OrderStatusCompleted, settled.Status)
	require.Equal(t, user.Balance+200, client.User.GetX(ctx, user.ID).Balance, "按充值面额与倍率到账一次")
	var usedBy int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT order_id FROM recharge_coupons WHERE id=$1 AND consumed_at IS NOT NULL", couponID).Scan(&usedBy))
	require.Equal(t, settled.ID, usedBy)
}

// TestRedeemClaimLimitAcrossTypesAndPaidFulfillment 在同一时段并发兑换两种券，并检查支付到账。
func TestRedeemClaimLimitAcrossTypesAndPaidFulfillment(t *testing.T) {
	client := testEntClient(t)
	ctx := t.Context()
	first, err := client.User.Create().SetUsername("claim-first").SetEmail("claim-first@example.com").SetPasswordHash("hash").Save(ctx)
	require.NoError(t, err)
	second, err := client.User.Create().SetUsername("claim-second").SetEmail("claim-second@example.com").SetPasswordHash("hash").Save(ctx)
	require.NoError(t, err)
	repo := billingpostgres.NewRedeemCodeRepository(client)
	service := billing.NewRedeemService(repo, couponTestUserReader{client}, nil, nil, nil, billingpostgres.NewRedeemMutations(client,
		billingpostgres.RedeemWriters{Balances: billingpostgres.NewBalanceStore(client)}), nil, nil, billing.RedeemRuntime{})
	original, err := service.GetClaimLimit(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := service.SetClaimLimit(context.Background(), original.RedeemClaimLimit)
		require.NoError(t, err)
	})
	now := time.Now()
	startsAt := now.Add(-time.Hour).Unix()
	endsAt := now.Add(time.Hour).Unix()
	limit := billing.RedeemClaimLimit{Enabled: true, StartsAt: &startsAt, EndsAt: &endsAt}
	_, err = service.SetClaimLimit(ctx, billing.RedeemClaimLimit{Enabled: true, StartsAt: &endsAt, EndsAt: &startsAt})
	require.ErrorIs(t, err, billing.ErrRedeemClaimWindowInvalid)
	settings, err := service.SetClaimLimit(ctx, limit)
	require.NoError(t, err)
	require.Equal(t, limit, settings.RedeemClaimLimit)

	store := paymentpostgres.NewOrderStore(client)
	fulfillment := payment.NewFulfillment(store, nil, nil, service, nil, payment.FulfillmentRuntime{})
	pay := func(tradeID string) {
		t.Helper()
		order, err := store.CreateCheckout(ctx, payment.CheckoutDraft{
			Order: payment.Order{
				UserID: first.ID, UserEmail: first.Email, UserName: first.Username,
				Amount: 30, PayAmount: 30, PaymentType: payment.TypeAlipay, OrderType: payment.OrderTypeBalance,
			},
			MaxPending: 3, LimitAmount: 30, TimeoutMinutes: 30,
		})
		require.NoError(t, err)
		require.NoError(t, fulfillment.ConfirmPayment(ctx, order, tradeID, 30, payment.TypeAlipay, nil))
	}
	pay("claim-limit-payment-before")

	balance := &billing.RedeemCode{Code: "claim-window-balance", Type: billing.RedeemTypeBalance, Value: 5}
	coupon := &billing.RedeemCode{
		Code: "claim-window-coupon", Type: billing.RedeemTypeCoupon,
		CouponTerms: &billing.RechargeCouponTerms{Currency: "CNY", MinimumMinor: 1000, DiscountMinor: 100},
	}
	require.NoError(t, service.CreateCode(ctx, balance))
	require.NoError(t, service.CreateCode(ctx, coupon))

	type claimResult struct {
		code string
		err  error
	}
	start := make(chan struct{})
	results := make(chan claimResult, 2)
	for _, code := range []*billing.RedeemCode{balance, coupon} {
		go func() {
			<-start
			_, err := service.RedeemForUser(ctx, first.ID, code.Code)
			results <- claimResult{code.Code, err}
		}()
	}
	close(start)
	successful := 0
	rejectedCode := ""
	balanceAwarded := 0.0
	for range 2 {
		result := <-results
		if result.err == nil {
			successful++
			if result.code == balance.Code {
				balanceAwarded = balance.Value
			}
		} else {
			require.ErrorIs(t, result.err, billing.ErrRedeemClaimLimitReached)
			rejectedCode = result.code
		}
	}
	require.Equal(t, 1, successful)
	require.NotEmpty(t, rejectedCode)
	_, err = service.RedeemForUser(ctx, second.ID, rejectedCode)
	require.NoError(t, err, "第二个用户能兑换第一人被拒绝的券")

	pay("claim-limit-payment-after")
	require.Equal(t, first.Balance+60+balanceAwarded, client.User.GetX(ctx, first.ID).Balance)
	var claims int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM redeem_user_claims WHERE user_id = $1", first.ID).Scan(&claims))
	require.Equal(t, 1, claims, "两笔支付到账都不占用户兑换额度")

	nextStart := now.Add(-3 * time.Hour).Unix()
	nextEnd := now.Add(-2 * time.Hour).Unix()
	_, err = service.SetClaimLimit(ctx, billing.RedeemClaimLimit{Enabled: true, StartsAt: &nextStart, EndsAt: &nextEnd})
	require.NoError(t, err)
	outside := &billing.RedeemCode{Code: "claim-outside-window", Type: billing.RedeemTypeBalance, Value: 5}
	require.NoError(t, service.CreateCode(ctx, outside))
	_, err = service.RedeemForUser(ctx, first.ID, outside.Code)
	require.NoError(t, err, "已结束的时段不限制当前兑换")
}
