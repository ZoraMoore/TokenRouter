package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	dbent "github.com/TokenFlux/TokenRouter/ent"
	"github.com/TokenFlux/TokenRouter/internal/billing"
)

const claimLimitFields = `enabled, extract(epoch FROM starts_at)::bigint, extract(epoch FROM ends_at)::bigint,
	floor(extract(epoch FROM tracking_since))::bigint`

// GetClaimLimit 读取当前配置和历史统计起点。
func (r *RedeemStore) GetClaimLimit(ctx context.Context) (billing.RedeemClaimSettings, error) {
	return scanClaimSettings(r.client.QueryContext(ctx, "SELECT "+claimLimitFields+" FROM redeem_claim_settings WHERE singleton"))
}

// SetClaimLimit 在同一条语句中更新整个时段。
func (r *RedeemStore) SetClaimLimit(ctx context.Context, limit billing.RedeemClaimLimit) (billing.RedeemClaimSettings, error) {
	return scanClaimSettings(r.client.QueryContext(ctx, `
		UPDATE redeem_claim_settings
		SET enabled = $1, starts_at = to_timestamp($2::double precision), ends_at = to_timestamp($3::double precision)
		WHERE singleton
		RETURNING `+claimLimitFields, limit.Enabled, limit.StartsAt, limit.EndsAt))
}

// RecordUserClaim 按用户串行核对限领，再使用数据库时间记录此次兑换。
func (r *RedeemStore) RecordUserClaim(ctx context.Context, userID, codeID int64) (time.Time, error) {
	tx := dbent.TxFromContext(ctx)
	if tx == nil {
		return time.Time{}, errors.New("recording a user redeem claim requires a redeem transaction")
	}
	client := tx.Client()
	if err := lockRedeemClaimUser(ctx, client, userID); err != nil {
		return time.Time{}, err
	}
	// 后续语句使用等待用户锁之后的快照。共享锁防止途中切换限领时段。
	settings, err := scanClaimSettings(client.QueryContext(ctx,
		"SELECT "+claimLimitFields+" FROM redeem_claim_settings WHERE singleton FOR SHARE"))
	if err != nil {
		return time.Time{}, err
	}
	rows, err := client.QueryContext(ctx, `
		WITH claim AS (SELECT clock_timestamp() AS claimed_at)
		INSERT INTO redeem_user_claims (user_id, redeem_code_id, claimed_at)
		SELECT $1, $2, claim.claimed_at FROM claim
		WHERE NOT (
		    $3::boolean AND claim.claimed_at >= to_timestamp($4::double precision)
		    AND claim.claimed_at < to_timestamp($5::double precision)
		    AND EXISTS (
		        SELECT 1 FROM redeem_user_claims
		        WHERE user_id = $1 AND claimed_at >= to_timestamp($4::double precision)
		        AND claimed_at < to_timestamp($5::double precision)
		    )
		)
		RETURNING claimed_at`, userID, codeID, settings.Enabled, settings.StartsAt, settings.EndsAt)
	if err != nil {
		return time.Time{}, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return time.Time{}, err
		}
		return time.Time{}, billing.ErrRedeemClaimLimitReached
	}
	var claimedAt time.Time
	err = rows.Scan(&claimedAt)
	return claimedAt, err
}

func lockRedeemClaimUser(ctx context.Context, client sqlExecutor, userID int64) error {
	rows, err := client.QueryContext(ctx, "SELECT id FROM users WHERE id = $1 AND deleted_at IS NULL FOR NO KEY UPDATE", userID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return billing.ErrUserNotFound
	}
	var lockedID int64
	return rows.Scan(&lockedID)
}

func scanClaimSettings(rows *sql.Rows, err error) (billing.RedeemClaimSettings, error) {
	if err != nil {
		return billing.RedeemClaimSettings{}, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return billing.RedeemClaimSettings{}, err
		}
		return billing.RedeemClaimSettings{}, sql.ErrNoRows
	}
	var settings billing.RedeemClaimSettings
	err = rows.Scan(&settings.Enabled, &settings.StartsAt, &settings.EndsAt, &settings.TrackingSince)
	return settings, err
}
