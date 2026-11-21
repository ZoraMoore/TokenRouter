package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/provider/operation"
)

const (
	maintenance401Message = "Upstream HTTP 401: authentication rejected"
)

type maintenanceAccount struct {
	record provider.Record
	guard  provider.MaintenanceGuard
}

// Guard 获取作答前的身份、账号状态和分组信息，供写入时复核。
func (s *MaintenanceStore) Guard(ctx context.Context, id, groupID int64) (provider.MaintenanceGuard, error) {
	account, err := readMaintenanceAccount(ctx, s.db, id, groupID, false)
	return account.guard, err
}

func readMaintenanceAccount(ctx context.Context, exec providerRowQueryer, id, groupID int64, lock bool) (maintenanceAccount, error) {
	var account maintenanceAccount
	account.record.ID = id
	query := `SELECT pelican_candy_revision, platform, type, parent_provider_id, status, schedulable, expires_at,
		COALESCE(error_message,''), COALESCE(temp_unschedulable_reason,''), temp_unschedulable_until,
		rate_limit_reset_at, overload_until
		FROM providers WHERE id=$1 AND deleted_at IS NULL`
	if lock {
		query += " FOR UPDATE"
	}
	var rate, overload *time.Time
	record := &account.record
	err := exec.QueryRowContext(ctx, query, id).Scan(&account.guard.Revision, &record.Platform, &record.Type,
		&record.ParentProviderID, &record.Status, &record.Schedulable, &record.ExpiresAt, &record.ErrorMessage,
		&record.TempUnschedulableReason, &record.TempUnschedulableUntil, &rate, &overload)
	if errors.Is(err, sql.ErrNoRows) {
		return account, provider.ErrMaintenanceNotFound
	}
	if err != nil {
		return account, err
	}
	raw, err := json.Marshal([]any{record.ErrorMessage, record.TempUnschedulableReason, record.TempUnschedulableUntil, record.ExpiresAt, rate, overload})
	if err != nil {
		return account, err
	}
	digest := sha256.Sum256(raw)
	account.guard.AuthVersion = hex.EncodeToString(digest[:])
	if groupID == 0 {
		return account, nil
	}
	query = `SELECT g.updated_at, pg.created_at FROM groups g JOIN provider_groups pg ON pg.group_id=g.id
		WHERE pg.provider_id=$1 AND g.id=$2 AND g.status='active' AND g.deleted_at IS NULL`
	if lock {
		query += " FOR SHARE OF g, pg"
	}
	err = exec.QueryRowContext(ctx, query, id, groupID).Scan(&account.guard.GroupVersion, &account.guard.MembershipAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = provider.ErrMaintenanceNotFound
	}
	return account, err
}

func (s *MaintenanceStore) applyMaintenanceUpdate(ctx context.Context, tx *sql.Tx, work provider.MaintenanceWork, item *provider.MaintenanceItem, update *provider.MaintenanceUpdate) (int64, error) {
	if update.Stage == "verify" && update.ReplacementID > 0 {
		return 0, replaceMaintenanceProvider(ctx, tx, item.ProviderID, update.ReplacementID)
	}
	if update.Guard.Revision == 0 {
		return 0, nil
	}
	id, group := item.ProviderID, item.GroupID
	if item.Stage == "verify" {
		id, group = item.ReplacementID, 0
	}
	current, err := readMaintenanceAccount(ctx, tx, id, group, true)
	if errors.Is(err, provider.ErrMaintenanceNotFound) || err == nil && !update.Guard.Matches(current.guard) {
		markMaintenanceChanged(update)
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if current.record.ExpiresAt != nil && !s.providers.options.Now().Before(*current.record.ExpiresAt) {
		markMaintenanceChanged(update)
		return 0, nil
	}
	var changed bool
	switch update.Action {
	case "join_relogin", "begin_relogin":
		changed, err = s.attachMaintenanceRelogin(ctx, tx, work, item, update, current)
	case "clear_401":
		changed, err = clearMaintenance401(ctx, tx, &current.record, update)
	case "record_401":
		changed, err = s.recordMaintenance401(ctx, tx, &current.record)
	case "replacement_401":
		changed, err = setMaintenance401(ctx, tx, id, update)
	case "suspend":
		changed, err = s.suspendMaintenanceAccount(ctx, tx, id, group, current.guard)
		if !changed && err == nil {
			markMaintenanceChanged(update)
		}
		// 分组停调已经写入 outbox。
		if changed && group > 0 {
			return id, err
		}
	}
	if !changed || err != nil {
		return 0, err
	}
	if err := s.providers.enqueue(ctx, tx, ProviderChanged, &id, nil, nil); err != nil {
		return 0, err
	}
	return id, nil
}

func (s *MaintenanceStore) attachMaintenanceRelogin(ctx context.Context, tx *sql.Tx, work provider.MaintenanceWork, item *provider.MaintenanceItem, update *provider.MaintenanceUpdate, current maintenanceAccount) (bool, error) {
	if !current.record.AllowsMaintenanceAuthCheck() || !provider.HasMaintenance401(&current.record) {
		markMaintenanceChanged(update)
		return false, nil
	}
	job, err := activeOperation(ctx, tx, item.ProviderID)
	created := false
	if errors.Is(err, operation.ErrNotFound) {
		if update.Action == "join_relogin" {
			deferMaintenanceRelogin(update, s.providers.options.Now())
			return false, nil
		}
		if update.CredentialVersion == "" {
			markMaintenanceChanged(update)
			return false, nil
		}
		if work.Trigger == provider.MaintenanceSchedule {
			blocked, err := previousMaintenanceReloginFailed(ctx, tx, item.ProviderID, current.guard.Revision)
			if err != nil {
				return false, err
			}
			if blocked {
				update.Outcome, update.Detail = "needs_action", "previous_relogin_failed"
				return false, nil
			}
		}
		job, err = enqueueOperation(ctx, tx, operation.Job{
			Kind: operation.Relogin, ProviderID: item.ProviderID, ProviderName: item.ProviderName,
			ExpectedCredentialVersion: update.CredentialVersion,
		})
		created = true
	}
	if err != nil {
		return false, err
	}
	if job.Kind != operation.Relogin {
		deferMaintenanceRelogin(update, s.providers.options.Now())
		return false, nil
	}
	item.OperationID = job.ID
	update.Stage, update.Outcome, update.Detail = "relogin", "", job.Message
	if created && current.record.Status != provider.StatusError {
		return setMaintenance401(ctx, tx, item.ProviderID, update)
	}
	return false, nil
}

func previousMaintenanceReloginFailed(ctx context.Context, tx *sql.Tx, id, revision int64) (bool, error) {
	var blocked bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM provider_maintenance_items WHERE (
			(provider_id=$1 AND stage='relogin') OR (replacement_id=$1 AND stage='verify')
		) AND state='finished' AND operation_id IS NOT NULL
			AND (observed->>'Revision')::bigint=$2 AND result->>'outcome' IN ('failed','needs_action')
	)`, id, revision).Scan(&blocked)
	return blocked, err
}

func markMaintenanceChanged(update *provider.MaintenanceUpdate) {
	*update = provider.MaintenanceUpdate{MaintenanceResult: provider.MaintenanceResult{Outcome: "skipped", Detail: "changed"}}
}

func deferMaintenanceRelogin(update *provider.MaintenanceUpdate, now time.Time) {
	update.Detail = "operation_wait"
	update.RetryAt = now.Add(15 * time.Second)
}

func clearMaintenance401(ctx context.Context, tx *sql.Tx, record *provider.Record, update *provider.MaintenanceUpdate) (bool, error) {
	if !provider.HasMaintenance401(record) {
		markMaintenanceChanged(update)
		return false, nil
	}
	_, err := tx.ExecContext(ctx, `UPDATE providers SET updated_at=now(),
		status=CASE WHEN status='error' THEN 'active' ELSE status END,
		error_message=CASE WHEN status='error' THEN '' ELSE error_message END,
		temp_unschedulable_until=CASE WHEN $2 THEN NULL ELSE temp_unschedulable_until END,
		temp_unschedulable_reason=CASE WHEN $2 THEN NULL ELSE temp_unschedulable_reason END
		WHERE id=$1`, record.ID, provider.HasMaintenance401Cooldown(record))
	return err == nil, err
}

func setMaintenance401(ctx context.Context, tx *sql.Tx, id int64, update *provider.MaintenanceUpdate) (bool, error) {
	err := tx.QueryRowContext(ctx, `UPDATE providers SET status='error', error_message=$2, updated_at=now()
		WHERE id=$1 RETURNING pelican_candy_revision`, id, maintenance401Message).Scan(&update.Guard.Revision)
	return err == nil, err
}

func (s *MaintenanceStore) recordMaintenance401(ctx context.Context, tx *sql.Tx, record *provider.Record) (bool, error) {
	result, err := tx.ExecContext(ctx, `UPDATE providers SET temp_unschedulable_until=now()+($2::bigint * interval '1 second'),
		temp_unschedulable_reason=$3, updated_at=now()
		WHERE id=$1 AND status='active' AND schedulable
			AND (temp_unschedulable_until IS NULL OR temp_unschedulable_until<=now())`,
		record.ID, int(s.cooldown/time.Second), maintenance401Message)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count > 0, err
}

func (s *MaintenanceStore) suspendMaintenanceAccount(ctx context.Context, tx *sql.Tx, id, groupID int64, guard provider.MaintenanceGuard) (bool, error) {
	if groupID > 0 {
		return s.providers.DisableAfterCandyAnswer(ctx, tx, id, groupID, guard.Revision, guard.MembershipAt)
	}
	result, err := tx.ExecContext(ctx, `UPDATE providers SET schedulable=false, updated_at=now()
		WHERE id=$1 AND pelican_candy_revision=$2 AND schedulable AND status='active' AND deleted_at IS NULL`, id, guard.Revision)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count > 0, err
}
