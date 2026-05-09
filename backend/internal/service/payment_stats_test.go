//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	dbent "github.com/TokenFlux/TokenRouter/ent"
	"github.com/TokenFlux/TokenRouter/internal/domain"
	"github.com/TokenFlux/TokenRouter/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestPaymentDashboardStatsWithRangeBuildsPurchaseDistribution(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("buyer@example.com").
		SetPasswordHash("hash").
		SetUsername("buyer").
		Save(ctx)
	require.NoError(t, err)

	paidAt := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	plan, err := client.SubscriptionPlan.Create().
		SetName("Pro 版").
		SetPrice(30).
		Save(ctx)
	require.NoError(t, err)
	planID := plan.ID
	createPaidPaymentStatsOrder(t, ctx, client, paymentStatsOrderSeed{
		userID:      user.ID,
		userEmail:   user.Email,
		userName:    user.Username,
		paymentType: payment.TypeAlipay,
		orderType:   payment.OrderTypeBalance,
		status:      OrderStatusCompleted,
		amount:      50,
		paidAt:      paidAt,
		tradeNo:     "balance-in-range",
	})
	createPaidPaymentStatsOrder(t, ctx, client, paymentStatsOrderSeed{
		userID:      user.ID,
		userEmail:   user.Email,
		userName:    user.Username,
		paymentType: payment.TypeStripe,
		orderType:   payment.OrderTypeSubscription,
		status:      OrderStatusPaid,
		amount:      30,
		paidAt:      paidAt.Add(time.Hour),
		tradeNo:     "plan-one",
		planID:      &planID,
		planName:    "专业版",
	})
	createPaidPaymentStatsOrder(t, ctx, client, paymentStatsOrderSeed{
		userID:      user.ID,
		userEmail:   user.Email,
		userName:    user.Username,
		paymentType: payment.TypeStripe,
		orderType:   payment.OrderTypeSubscription,
		status:      OrderStatusRecharging,
		amount:      20,
		paidAt:      paidAt.Add(2 * time.Hour),
		tradeNo:     "plan-two",
		planID:      &planID,
		planName:    "专业版",
	})
	createPaidPaymentStatsOrder(t, ctx, client, paymentStatsOrderSeed{
		userID:      user.ID,
		userEmail:   user.Email,
		userName:    user.Username,
		paymentType: payment.TypeWxpay,
		orderType:   payment.OrderTypeBalance,
		status:      OrderStatusPending,
		amount:      999,
		paidAt:      paidAt,
		tradeNo:     "pending-ignored",
	})
	createPaidPaymentStatsOrder(t, ctx, client, paymentStatsOrderSeed{
		userID:      user.ID,
		userEmail:   user.Email,
		userName:    user.Username,
		paymentType: payment.TypeWxpay,
		orderType:   payment.OrderTypeBalance,
		status:      OrderStatusCompleted,
		amount:      999,
		paidAt:      paidAt.AddDate(0, 0, 5),
		tradeNo:     "outside-range",
	})

	svc := &PaymentService{entClient: client}
	stats, err := svc.GetDashboardStatsWithRange(
		ctx,
		time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC),
	)
	require.NoError(t, err)

	require.Equal(t, 100.0, stats.TotalAmount)
	require.Equal(t, 3, stats.TotalCount)
	require.Len(t, stats.DailySeries, 3)

	require.Len(t, stats.PurchaseDistribution, 2)
	balanceStat := findPurchaseDistributionStat(t, stats.PurchaseDistribution, payment.OrderTypeBalance)
	require.Equal(t, payment.OrderTypeBalance, balanceStat.Label)
	require.Equal(t, 50.0, balanceStat.Amount)
	require.Equal(t, 1, balanceStat.Count)
	subscriptionStat := findPurchaseDistributionStat(t, stats.PurchaseDistribution, payment.OrderTypeSubscription)
	require.Equal(t, "Pro 版", subscriptionStat.Label)
	require.Equal(t, planID, *subscriptionStat.PlanID)
	require.Equal(t, 50.0, subscriptionStat.Amount)
	require.Equal(t, 2, subscriptionStat.Count)
}

func TestPaymentDashboardStatsWithRangeFallsBackToPlanSnapshotWhenPlanDeleted(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("deleted-plan@example.com").
		SetPasswordHash("hash").
		SetUsername("deleted-plan-user").
		Save(ctx)
	require.NoError(t, err)

	planID := int64(22)
	paidAt := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	createPaidPaymentStatsOrder(t, ctx, client, paymentStatsOrderSeed{
		userID:      user.ID,
		userEmail:   user.Email,
		userName:    user.Username,
		paymentType: payment.TypeStripe,
		orderType:   payment.OrderTypeSubscription,
		status:      OrderStatusCompleted,
		amount:      66,
		paidAt:      paidAt,
		tradeNo:     "deleted-plan-order",
		planID:      &planID,
		planName:    "历史套餐",
	})

	svc := &PaymentService{entClient: client}
	stats, err := svc.GetDashboardStatsWithRange(
		ctx,
		time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC),
	)
	require.NoError(t, err)

	require.Len(t, stats.PurchaseDistribution, 1)
	require.Equal(t, "历史套餐", stats.PurchaseDistribution[0].Label)
	require.Equal(t, planID, *stats.PurchaseDistribution[0].PlanID)
	require.Equal(t, 66.0, stats.PurchaseDistribution[0].Amount)
	require.Equal(t, 1, stats.PurchaseDistribution[0].Count)
}

func findPurchaseDistributionStat(t *testing.T, items []PurchaseDistributionStat, itemType string) PurchaseDistributionStat {
	t.Helper()
	for _, item := range items {
		if item.Type == itemType {
			return item
		}
	}
	t.Fatalf("purchase distribution item %q not found", itemType)
	return PurchaseDistributionStat{}
}

type paymentStatsOrderSeed struct {
	userID      int64
	userEmail   string
	userName    string
	paymentType string
	orderType   string
	status      string
	amount      float64
	paidAt      time.Time
	tradeNo     string
	planID      *int64
	planName    string
}

func createPaidPaymentStatsOrder(t *testing.T, ctx context.Context, client *dbent.Client, seed paymentStatsOrderSeed) {
	t.Helper()
	create := client.PaymentOrder.Create().
		SetUserID(seed.userID).
		SetUserEmail(seed.userEmail).
		SetUserName(seed.userName).
		SetAmount(seed.amount).
		SetPayAmount(seed.amount).
		SetFeeRate(0).
		SetRechargeCode("PAY-" + seed.tradeNo).
		SetOutTradeNo(seed.tradeNo).
		SetPaymentType(seed.paymentType).
		SetPaymentTradeNo("trade-" + seed.tradeNo).
		SetOrderType(seed.orderType).
		SetStatus(seed.status).
		SetExpiresAt(seed.paidAt.Add(time.Hour)).
		SetPaidAt(seed.paidAt).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com")
	if seed.planID != nil {
		create.SetPlanID(*seed.planID).
			SetPlanSnapshot(domain.SubscriptionPlanSnapshot{
				Name:         seed.planName,
				Price:        seed.amount,
				ValidityDays: 30,
			})
	}
	_, err := create.Save(ctx)
	require.NoError(t, err)
}
