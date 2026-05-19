package service

import (
	"context"
	"testing"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/payment"
	"github.com/TokenFlux/TokenRouter/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type paymentRedeemDeliveryRepo struct {
	nextID     int64
	codesByKey map[string]*RedeemCode
}

func newPaymentRedeemDeliveryRepo() *paymentRedeemDeliveryRepo {
	return &paymentRedeemDeliveryRepo{
		nextID:     1,
		codesByKey: make(map[string]*RedeemCode),
	}
}

func clonePaymentRedeemDeliveryCode(code *RedeemCode) *RedeemCode {
	if code == nil {
		return nil
	}
	cloned := *code
	return &cloned
}

func (r *paymentRedeemDeliveryRepo) Create(_ context.Context, code *RedeemCode) error {
	if _, exists := r.codesByKey[code.Code]; exists {
		return ErrRedeemCodeExists
	}
	cloned := *code
	if cloned.ID == 0 {
		cloned.ID = r.nextID
		r.nextID++
	}
	cloned.Status = cloned.PersistedStatus()
	code.ID = cloned.ID
	code.Status = cloned.Status
	r.codesByKey[cloned.Code] = &cloned
	return nil
}

func (r *paymentRedeemDeliveryRepo) CreateBatch(ctx context.Context, codes []RedeemCode) error {
	for i := range codes {
		if err := r.Create(ctx, &codes[i]); err != nil {
			return err
		}
	}
	return nil
}

func (r *paymentRedeemDeliveryRepo) GetByID(_ context.Context, id int64) (*RedeemCode, error) {
	for _, code := range r.codesByKey {
		if code.ID == id {
			return clonePaymentRedeemDeliveryCode(code), nil
		}
	}
	return nil, ErrRedeemCodeNotFound
}

func (r *paymentRedeemDeliveryRepo) GetByCode(_ context.Context, code string) (*RedeemCode, error) {
	redeemCode := r.codesByKey[code]
	if redeemCode == nil {
		return nil, ErrRedeemCodeNotFound
	}
	return clonePaymentRedeemDeliveryCode(redeemCode), nil
}

func (r *paymentRedeemDeliveryRepo) GetByCodeForUpdate(ctx context.Context, code string) (*RedeemCode, error) {
	return r.GetByCode(ctx, code)
}

func (r *paymentRedeemDeliveryRepo) Update(_ context.Context, code *RedeemCode) error {
	if _, exists := r.codesByKey[code.Code]; !exists {
		return ErrRedeemCodeNotFound
	}
	cloned := *code
	cloned.Status = cloned.PersistedStatus()
	r.codesByKey[cloned.Code] = &cloned
	return nil
}

func (r *paymentRedeemDeliveryRepo) Delete(_ context.Context, id int64) error {
	for code, redeemCode := range r.codesByKey {
		if redeemCode.ID == id {
			delete(r.codesByKey, code)
			return nil
		}
	}
	return ErrRedeemCodeNotFound
}

func (r *paymentRedeemDeliveryRepo) Use(_ context.Context, id, userID int64) error {
	for key, redeemCode := range r.codesByKey {
		if redeemCode.ID != id {
			continue
		}
		now := time.Now()
		cloned := *redeemCode
		cloned.UsedBy = &userID
		cloned.UsedAt = &now
		cloned.UsedCount++
		cloned.Status = cloned.PersistedStatus()
		r.codesByKey[key] = &cloned
		return nil
	}
	return ErrRedeemCodeNotFound
}

func (r *paymentRedeemDeliveryRepo) CreateUsage(context.Context, *RedeemCodeUsage) error {
	return nil
}

func (r *paymentRedeemDeliveryRepo) GetUsageByRedeemCodeAndUser(context.Context, int64, int64) (*RedeemCodeUsage, error) {
	return nil, nil
}

func (r *paymentRedeemDeliveryRepo) List(context.Context, pagination.PaginationParams) ([]RedeemCode, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (r *paymentRedeemDeliveryRepo) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string) ([]RedeemCode, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (r *paymentRedeemDeliveryRepo) ListByUser(context.Context, int64, int) ([]RedeemCode, error) {
	return nil, nil
}

func (r *paymentRedeemDeliveryRepo) ListByUserPaginated(context.Context, int64, pagination.PaginationParams, string) ([]RedeemCode, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (r *paymentRedeemDeliveryRepo) SumPositiveBalanceByUser(context.Context, int64) (float64, error) {
	return 0, nil
}

func TestExecuteRedeemCodeDeliveryFulfillmentCreatesUnusedRedeemCode(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user := client.User.Create().
		SetEmail("buyer@example.com").
		SetPasswordHash("hash").
		SaveX(ctx)

	providerKey := payment.TypeBEPUSDT
	paidAt := time.Now()
	order := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName("buyer").
		SetAmount(12.34).
		SetPayAmount(12.34).
		SetRechargeCode("PAY-CODE-123").
		SetOutTradeNo("sub2_redeem_delivery").
		SetPaymentType(payment.TypeUSDTBEP20).
		SetPaymentTradeNo("bepusdt_trade_1").
		SetProviderKey(providerKey).
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusPaid).
		SetPaidAt(paidAt).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("example.com").
		SaveX(ctx)

	redeemRepo := newPaymentRedeemDeliveryRepo()
	svc := &PaymentService{
		entClient:     client,
		redeemService: NewRedeemService(redeemRepo, nil, nil, nil, nil, nil, nil),
	}

	require.NoError(t, svc.ExecuteRedeemCodeDeliveryFulfillment(ctx, order.ID))

	reloadedOrder := client.PaymentOrder.GetX(ctx, order.ID)
	require.Equal(t, OrderStatusCompleted, reloadedOrder.Status)
	require.NotNil(t, reloadedOrder.CompletedAt)

	redeemCode, err := redeemRepo.GetByCode(ctx, "PAY-CODE-123")
	require.NoError(t, err)
	require.Equal(t, RedeemTypeBalance, redeemCode.Type)
	require.Equal(t, StatusUnused, redeemCode.Status)
	require.Equal(t, 0, redeemCode.UsedCount)
	require.Nil(t, redeemCode.UsedBy)
	require.InDelta(t, 12.34, redeemCode.Value, 0.001)
}
