package postgres

import (
	"context"

	"github.com/TokenFlux/TokenRouter/internal/billing"
)

// RedeemBalanceWriter 与身份并发写入分离，避免计费持有身份字段算法。
type RedeemBalanceWriter interface {
	UpdateBalance(context.Context, int64, float64) error
	ApplyRedeemBalanceAdjustment(context.Context, int64, float64) error
}
type RedeemConcurrencyWriter interface {
	UpdateConcurrency(context.Context, int64, int) error
	ApplyRedeemConcurrencyAdjustment(context.Context, int64, int) error
}

// RedeemWriters 将兑换写入操作转发到调用方传入的事务 context。
type RedeemWriters struct {
	Balances    RedeemBalanceWriter
	Concurrency RedeemConcurrencyWriter
}

func (w RedeemWriters) UpdateBalance(ctx context.Context, id int64, amount float64) error {
	return w.Balances.UpdateBalance(ctx, id, amount)
}

func (w RedeemWriters) ApplyRedeemBalanceAdjustment(ctx context.Context, id int64, amount float64) error {
	return w.Balances.ApplyRedeemBalanceAdjustment(ctx, id, amount)
}

func (w RedeemWriters) UpdateConcurrency(ctx context.Context, id int64, delta int) error {
	if w.Concurrency == nil {
		return billing.ErrRedeemConcurrencyUnsupported
	}
	return w.Concurrency.UpdateConcurrency(ctx, id, delta)
}

func (w RedeemWriters) ApplyRedeemConcurrencyAdjustment(ctx context.Context, id int64, delta int) error {
	if w.Concurrency == nil {
		return billing.ErrRedeemConcurrencyUnsupported
	}
	return w.Concurrency.ApplyRedeemConcurrencyAdjustment(ctx, id, delta)
}
