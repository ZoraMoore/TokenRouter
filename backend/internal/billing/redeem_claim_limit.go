package billing

import (
	"context"
	"errors"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

const latestRedeemClaimTime = 253402300799

var (
	// ErrRedeemClaimLimitReached 表示用户在限领时段内已兑换过一张券。
	ErrRedeemClaimLimitReached = apperror.Conflict("REDEEM_CLAIM_LIMIT_REACHED", "you have already claimed a balance or discount code during this period")
	// ErrRedeemClaimWindowInvalid 表示开始和结束时间缺失或顺序无效。
	ErrRedeemClaimWindowInvalid = apperror.BadRequest("REDEEM_CLAIM_WINDOW_INVALID", "start and end times must define a valid claim window")
)

// RedeemClaimLimit 使用 Unix 秒表示余额和优惠券共用的限领时段。
type RedeemClaimLimit struct {
	Enabled  bool   `json:"enabled"`
	StartsAt *int64 `json:"starts_at"`
	EndsAt   *int64 `json:"ends_at"`
}

// RedeemClaimSettings 同时返回用户主动兑换记录的统计起点。
type RedeemClaimSettings struct {
	RedeemClaimLimit
	TrackingSince int64 `json:"tracking_since"`
}

type redeemClaimStore interface {
	GetClaimLimit(context.Context) (RedeemClaimSettings, error)
	SetClaimLimit(context.Context, RedeemClaimLimit) (RedeemClaimSettings, error)
	RecordUserClaim(context.Context, int64, int64) (time.Time, error)
}

// GetClaimLimit 返回兑换码管理页的限领设置。
func (s *RedeemService) GetClaimLimit(ctx context.Context) (RedeemClaimSettings, error) {
	store, ok := s.redeemRepo.(redeemClaimStore)
	if !ok {
		return RedeemClaimSettings{}, errors.New("redeem claim store is unavailable")
	}
	return store.GetClaimLimit(ctx)
}

// SetClaimLimit 校验时段后保存设置，关闭时允许保留原时段。
func (s *RedeemService) SetClaimLimit(ctx context.Context, limit RedeemClaimLimit) (RedeemClaimSettings, error) {
	hasWindow := limit.StartsAt != nil && limit.EndsAt != nil
	if hasWindow {
		if *limit.StartsAt <= 0 || *limit.EndsAt > latestRedeemClaimTime || *limit.StartsAt >= *limit.EndsAt {
			return RedeemClaimSettings{}, ErrRedeemClaimWindowInvalid
		}
	} else if limit.Enabled || limit.StartsAt != nil || limit.EndsAt != nil {
		return RedeemClaimSettings{}, ErrRedeemClaimWindowInvalid
	}
	store, ok := s.redeemRepo.(redeemClaimStore)
	if !ok {
		return RedeemClaimSettings{}, errors.New("redeem claim store is unavailable")
	}
	return store.SetClaimLimit(ctx, limit)
}
