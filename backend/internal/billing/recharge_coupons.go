package billing

import (
	"strings"
	"time"

	"golang.org/x/text/currency"

	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

const (
	// RechargeCouponMaxMinor 是 JSON 客户端可以精确表示的最大整数。
	RechargeCouponMaxMinor = 1<<53 - 1
)

var (
	ErrRechargeCouponTerms       = apperror.BadRequest("RECHARGE_COUPON_TERMS_INVALID", "recharge coupon requires a currency and a positive discount below the minimum")
	ErrRechargeCouponUnavailable = apperror.Conflict("RECHARGE_COUPON_UNAVAILABLE", "recharge coupon is unavailable")
)

// RechargeCouponTerms 记录券面金额，按币种允许的小数位转换为整数。
type RechargeCouponTerms struct {
	Currency      string `json:"currency"`
	MinimumMinor  int64  `json:"minimum_minor"`
	DiscountMinor int64  `json:"discount_minor"`
}

// RechargeCouponSnapshot 记录订单结算时使用的券和充值面额。
type RechargeCouponSnapshot struct {
	RechargeCouponTerms
	CouponID      int64 `json:"coupon_id"`
	RechargeMinor int64 `json:"recharge_minor"`
	ChargeMinor   int64 `json:"charge_minor"`
}

// RechargeCoupon 是用户领取的单张充值券。
type RechargeCoupon struct {
	RechargeCouponTerms
	ID         int64      `json:"id"`
	ClaimedAt  time.Time  `json:"claimed_at"`
	OrderID    *int64     `json:"order_id,omitempty"`
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
}

// Validate 检查可按原币种用于充值的门槛及折扣。
func (terms RechargeCouponTerms) Validate() error {
	if len(terms.Currency) != 3 || terms.Currency != strings.ToUpper(terms.Currency) || terms.Currency == "XXX" {
		return ErrRechargeCouponTerms
	}
	if _, err := currency.ParseISO(terms.Currency); err != nil {
		return ErrRechargeCouponTerms
	}
	if terms.DiscountMinor <= 0 || terms.MinimumMinor <= terms.DiscountMinor || terms.MinimumMinor > RechargeCouponMaxMinor {
		return ErrRechargeCouponTerms
	}
	return nil
}

// validateRedeemCouponTerms 检查优惠券字段和兑换码类型的组合。
func validateRedeemCouponTerms(codeType string, terms *RechargeCouponTerms, value float64, planID *int64) error {
	if codeType != RedeemTypeCoupon {
		if terms != nil {
			return ErrRechargeCouponTerms
		}
		return nil
	}
	if terms == nil || value != 0 || planID != nil {
		return ErrRechargeCouponTerms
	}
	return terms.Validate()
}
