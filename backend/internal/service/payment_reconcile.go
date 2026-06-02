package service

import (
	"context"
	"fmt"

	dbent "github.com/TokenFlux/TokenRouter/ent"
	"github.com/TokenFlux/TokenRouter/ent/paymentorder"
	"github.com/TokenFlux/TokenRouter/internal/payment"
)

func (s *PaymentService) reconcilePaid(ctx context.Context, order *dbent.PaymentOrder) string {
	if s == nil || order == nil {
		return ""
	}
	return s.checkPaid(ctx, order)
}

func (s *PaymentService) ReconcilePendingWxpayOrders(ctx context.Context) (int, error) {
	if s == nil || s.entClient == nil {
		return 0, nil
	}
	orders, err := s.entClient.PaymentOrder.Query().
		Where(
			paymentorder.StatusIn(OrderStatusPending, OrderStatusExpired),
			paymentorder.PaymentTypeEQ(payment.TypeWxpay),
		).
		All(ctx)
	if err != nil {
		return 0, fmt.Errorf("query pending wxpay orders: %w", err)
	}
	recovered := 0
	for _, order := range orders {
		if s.reconcilePaid(ctx, order) == checkPaidResultAlreadyPaid {
			recovered++
		}
	}
	return recovered, nil
}
