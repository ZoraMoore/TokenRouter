package payment

import (
	"context"
	"errors"
	"math"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/TokenFlux/TokenRouter/internal/billing"
	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

type rechargeCouponReader interface {
	RechargeCoupon(context.Context, int64, int64) (*billing.RechargeCoupon, error)
	ListRechargeCoupons(context.Context, int64, bool, int, int) ([]billing.RechargeCoupon, int64, error)
}

// RechargeCouponQuote 返回用券订单提交前的应付金额和到账额。
type RechargeCouponQuote struct {
	Amount         float64                         `json:"amount"`
	CouponSnapshot *billing.RechargeCouponSnapshot `json:"coupon_snapshot"`
	FeeBreakdown
}

type checkoutPricing struct {
	fee       FeeBreakdown
	payString string
	payAmount float64
	coupon    *billing.RechargeCouponSnapshot
}

// ListRechargeCoupons 返回请求者自己的券，可筛选出尚未被订单使用的券。
func (s *Checkout) ListRechargeCoupons(ctx context.Context, userID int64, available bool, page, size int) ([]billing.RechargeCoupon, int64, error) {
	store, ok := s.store.(rechargeCouponReader)
	if !ok {
		return nil, 0, errors.New("recharge coupon reader unavailable")
	}
	return store.ListRechargeCoupons(ctx, userID, available, page, size)
}

// QuoteRechargeCoupon 使用下单规则报价。支付方式和优惠券在提交订单时重新校验。
func (s *Checkout) QuoteRechargeCoupon(ctx context.Context, req CreateOrderRequest) (*RechargeCouponQuote, error) {
	req.OrderType = OrderTypeBalance
	req.PaymentType = NormalizeVisibleMethod(req.PaymentType)
	if req.PaymentType == "" || req.CouponID <= 0 {
		return nil, apperror.BadRequest("INVALID_INPUT", "payment type and recharge coupon are required")
	}
	cfg, err := s.configService.GetPaymentConfig(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, apperror.Forbidden("PAYMENT_DISABLED", "payment system is disabled")
	}
	if _, err := s.ValidateOrderInput(ctx, req, cfg); err != nil {
		return nil, err
	}
	coupon, err := s.checkoutCoupon(ctx, req)
	if err != nil {
		return nil, err
	}
	currency, err := s.configService.ValidateMethodCurrencyConsistency(ctx, req.PaymentType)
	if err != nil {
		return nil, err
	}
	pricing, err := priceCheckout(req, cfg, req.Amount, currency, coupon)
	if err != nil {
		return nil, err
	}
	return &RechargeCouponQuote{
		Amount:         CalculateCreditedBalance(req.Amount, cfg.BalanceRechargeMultiplier),
		FeeBreakdown:   pricing.fee,
		CouponSnapshot: pricing.coupon,
	}, nil
}

func (s *Checkout) checkoutCoupon(ctx context.Context, req CreateOrderRequest) (*billing.RechargeCoupon, error) {
	if req.CouponID == 0 {
		return nil, nil
	}
	if req.CouponID < 0 || req.OrderType != OrderTypeBalance {
		return nil, apperror.BadRequest("RECHARGE_COUPON_ORDER_TYPE", "coupons apply to balance recharge only")
	}
	store, ok := s.store.(rechargeCouponReader)
	if !ok {
		return nil, errors.New("recharge coupon reader unavailable")
	}
	return store.RechargeCoupon(ctx, req.UserID, req.CouponID)
}

func priceCheckout(req CreateOrderRequest, cfg *PaymentConfig, limitAmount float64, currency string, coupon *billing.RechargeCoupon) (checkoutPricing, error) {
	var pricing checkoutPricing
	chargeAmount := limitAmount
	if coupon != nil {
		snapshot, err := quoteRechargeCoupon(*coupon, currency, limitAmount)
		if err != nil {
			return pricing, err
		}
		chargeAmount = rechargeCouponAmount(snapshot.ChargeMinor, currency)
		pricing.coupon = snapshot
	}
	var err error
	pricing.fee, pricing.payString, pricing.payAmount, err = CalculateCreateOrderPayAmountForOrderType(chargeAmount, cfg.EffectiveMethodFee(req.PaymentType), currency, req.OrderType, cfg.SubscriptionUSDToCNYRate)
	if err != nil {
		return pricing, err
	}
	if coupon != nil && !rechargeCouponFeeFitsOrder(pricing) {
		return pricing, apperror.BadRequest("INVALID_AMOUNT", "the discounted payment and its fees exceed order precision")
	}
	return pricing, nil
}

func quoteRechargeCoupon(coupon billing.RechargeCoupon, currency string, amount float64) (*billing.RechargeCouponSnapshot, error) {
	if coupon.OrderID != nil || coupon.ConsumedAt != nil {
		return nil, billing.ErrRechargeCouponUnavailable
	}
	if err := coupon.Validate(); err != nil {
		return nil, err
	}
	if !strings.EqualFold(currency, coupon.Currency) {
		return nil, apperror.BadRequest("RECHARGE_COUPON_CURRENCY", "the coupon currency does not match the payment method")
	}
	minor, ok := rechargeCouponMinor(amount, currency)
	if !ok {
		return nil, apperror.BadRequest("INVALID_AMOUNT", "recharge amount exceeds currency precision")
	}
	if minor < coupon.MinimumMinor {
		return nil, apperror.BadRequest("RECHARGE_COUPON_MINIMUM", "the amount is below the coupon minimum")
	}
	return &billing.RechargeCouponSnapshot{
		RechargeCouponTerms: coupon.RechargeCouponTerms,
		CouponID:            coupon.ID,
		RechargeMinor:       minor,
		ChargeMinor:         minor - coupon.DiscountMinor,
	}, nil
}

func rechargeCouponMinor(amount float64, currency string) (int64, bool) {
	if math.IsNaN(amount) || math.IsInf(amount, 0) || amount <= 0 {
		return 0, false
	}
	minor := decimal.NewFromFloat(amount).Shift(int32(CurrencyMaxFractionDigits(currency)))
	if !minor.IsInteger() || minor.GreaterThan(decimal.NewFromInt(billing.RechargeCouponMaxMinor)) {
		return 0, false
	}
	return minor.IntPart(), true
}

func rechargeCouponAmount(minor int64, currency string) float64 {
	return decimal.New(minor, -int32(CurrencyMaxFractionDigits(currency))).InexactFloat64()
}

// rechargeCouponFeeFitsOrder 核对实际扣款与费用可写入订单的两位小数字段。
func rechargeCouponFeeFitsOrder(pricing checkoutPricing) bool {
	if _, ok := rechargeCouponMinor(pricing.payAmount, pricing.coupon.Currency); !ok {
		return false
	}
	for _, amount := range []float64{pricing.payAmount, pricing.fee.FixedFee, pricing.fee.FeeRateAmount, pricing.fee.FeeAmount} {
		precise := decimal.NewFromFloat(amount)
		if !precise.Equal(precise.Truncate(2)) {
			return false
		}
	}
	got, ok := rechargeCouponMinor(pricing.fee.BaseAmount, pricing.coupon.Currency)
	return ok && got == pricing.coupon.ChargeMinor
}
