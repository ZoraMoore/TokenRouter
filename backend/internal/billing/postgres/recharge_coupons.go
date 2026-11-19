package postgres

import (
	"context"
	"errors"
	"time"

	dbent "github.com/TokenFlux/TokenRouter/ent"
	"github.com/TokenFlux/TokenRouter/internal/billing"
)

// ClaimRechargeCoupon 写入券面副本，与兑换次数和使用记录共用一笔事务。
func (r *RedeemStore) ClaimRechargeCoupon(ctx context.Context, userID, codeID int64, terms billing.RechargeCouponTerms, now time.Time) (*billing.RechargeCoupon, error) {
	tx := dbent.TxFromContext(ctx)
	if tx == nil {
		return nil, errors.New("claiming a recharge coupon requires a redeem transaction")
	}
	rows, err := tx.Client().QueryContext(ctx, `
		INSERT INTO recharge_coupons (user_id, redeem_code_id, currency, minimum_minor, discount_minor, claimed_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id, redeem_code_id) DO NOTHING
		RETURNING id`, userID, codeID, terms.Currency, terms.MinimumMinor, terms.DiscountMinor, now)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, billing.ErrRedeemCodeAlreadyUsed
	}
	coupon := &billing.RechargeCoupon{RechargeCouponTerms: terms, ClaimedAt: now}
	return coupon, rows.Scan(&coupon.ID)
}
