package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	dbent "github.com/TokenFlux/TokenRouter/ent"
	"github.com/TokenFlux/TokenRouter/internal/billing"
	"github.com/TokenFlux/TokenRouter/internal/payment"
)

const (
	// rechargeCouponFields 保持单张券与列表的扫描顺序一致。
	rechargeCouponFields = "id, currency, minimum_minor, discount_minor, claimed_at, order_id, consumed_at"
)

// RechargeCoupon 按持券人及券号读取结算所需的资料。
func (s *OrderStore) RechargeCoupon(ctx context.Context, userID, id int64) (*billing.RechargeCoupon, error) {
	rows, err := s.client.QueryContext(ctx, "SELECT "+rechargeCouponFields+" FROM recharge_coupons WHERE user_id=$1 AND id=$2", userID, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, billing.ErrRechargeCouponUnavailable
	}
	var coupon billing.RechargeCoupon
	return &coupon, scanRechargeCoupon(rows, &coupon)
}

// ListRechargeCoupons 列出持券人的券，供充值结算和领券历史使用。
func (s *OrderStore) ListRechargeCoupons(ctx context.Context, userID int64, available bool, page, size int) ([]billing.RechargeCoupon, int64, error) {
	if page < 1 {
		page = 1
	}
	size = min(max(size, 1), 100)
	const filter = " FROM recharge_coupons WHERE user_id=$1 AND (NOT $2 OR order_id IS NULL)"
	total, err := s.rechargeCouponCount(ctx, filter, userID, available)
	if err != nil {
		return nil, 0, err
	}
	result := make([]billing.RechargeCoupon, 0, size)
	if total == 0 || int64(page-1) > (total-1)/int64(size) {
		return result, total, nil
	}
	rows, err := s.client.QueryContext(ctx, "SELECT "+rechargeCouponFields+filter+" ORDER BY id DESC LIMIT $3 OFFSET $4", userID, available, size, (page-1)*size)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var coupon billing.RechargeCoupon
		if err := scanRechargeCoupon(rows, &coupon); err != nil {
			return nil, 0, err
		}
		result = append(result, coupon)
	}
	return result, total, rows.Err()
}

func (s *OrderStore) rechargeCouponCount(ctx context.Context, filter string, userID int64, available bool) (int64, error) {
	rows, err := s.client.QueryContext(ctx, "SELECT count(*)"+filter, userID, available)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return 0, rows.Err()
	}
	var total int64
	err = rows.Scan(&total)
	return total, err
}

func scanRechargeCoupon(row *sql.Rows, coupon *billing.RechargeCoupon) error {
	return row.Scan(&coupon.ID, &coupon.Currency, &coupon.MinimumMinor, &coupon.DiscountMinor, &coupon.ClaimedAt, &coupon.OrderID, &coupon.ConsumedAt)
}

// reserveRechargeCoupon 占用与结算快照一致的可用券。没有占到券时，下单事务回滚。
func reserveRechargeCoupon(ctx context.Context, tx *dbent.Tx, orderID, userID int64, snapshot billing.RechargeCouponSnapshot) error {
	result, err := tx.Client().ExecContext(ctx, `
		UPDATE recharge_coupons SET order_id=$1
		WHERE id=$2 AND user_id=$3 AND order_id IS NULL AND consumed_at IS NULL
		  AND currency=$4 AND minimum_minor=$5 AND discount_minor=$6`,
		orderID, snapshot.CouponID, userID, snapshot.Currency, snapshot.MinimumMinor, snapshot.DiscountMinor)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return billing.ErrRechargeCouponUnavailable
	}
	return err
}

// transitionCouponOrder 将付款、核销、被其他订单占用时的复核记录写入同一个事务。
func (s *OrderStore) transitionCouponOrder(ctx context.Context, c payment.OrderTransition) (int, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	n, err := transitionOrder(ctx, tx.Client(), c)
	if err != nil || n == 0 {
		return 0, err
	}
	order, err := tx.PaymentOrder.Get(ctx, c.ID)
	if err != nil {
		return 0, err
	}
	if order.CouponSnapshot.CouponID > 0 {
		if c.ConfirmCoupon && c.PaidAt != nil {
			if err := consumeRechargeCoupon(ctx, tx, order, *c.PaidAt); err != nil {
				return 0, err
			}
		} else if c.ReleaseCoupon && order.PaidAt == nil {
			_, err := tx.Client().ExecContext(ctx, `
				UPDATE recharge_coupons SET order_id=NULL WHERE order_id=$1 AND consumed_at IS NULL`, c.ID)
			if err != nil {
				return 0, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
}

// consumeRechargeCoupon 允许迟到的已付款订单使用空闲券。券已交给另一订单时记录实收，等待复核。
func consumeRechargeCoupon(ctx context.Context, tx *dbent.Tx, order *dbent.PaymentOrder, paidAt time.Time) error {
	snapshot := order.CouponSnapshot
	result, err := tx.Client().ExecContext(ctx, `
		UPDATE recharge_coupons SET order_id=$1, consumed_at=$2
		WHERE id=$3 AND user_id=$4 AND (order_id IS NULL OR order_id=$1) AND consumed_at IS NULL
		  AND currency=$5 AND minimum_minor=$6 AND discount_minor=$7`,
		order.ID, paidAt, snapshot.CouponID, order.UserID, snapshot.Currency, snapshot.MinimumMinor, snapshot.DiscountMinor)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil || n == 1 {
		return err
	}
	if err := tx.PaymentOrder.UpdateOneID(order.ID).
		SetStatus(payment.OrderStatusPaymentReview).
		SetFailedReason("recharge coupon could not be reserved for confirmed payment").
		Exec(ctx); err != nil {
		return err
	}
	detail, err := json.Marshal(map[string]any{"coupon_id": snapshot.CouponID, "paid_amount": order.PayAmount})
	if err != nil {
		return fmt.Errorf("encode recharge coupon audit: %w", err)
	}
	return tx.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(order.ID, 10)).
		SetAction("RECHARGE_COUPON_PAYMENT_REVIEW").
		SetOperator("system").
		SetDetail(string(detail)).
		Exec(ctx)
}
