package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/TokenFlux/TokenRouter/internal/provider"
)

const (
	maintenanceLeasePeriod = provider.MaintenanceWorkTimeout + time.Minute
)

// Claim 用数据库租约限制所有节点同时执行的任务数。同一账号一次只能有一个有效租约。
func (s *MaintenanceStore) Claim(ctx context.Context) (provider.MaintenanceWork, error) {
	var work provider.MaintenanceWork
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return work, err
	}
	defer func() { _ = tx.Rollback() }()
	var acquired bool
	err = tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended('tokenrouter:provider-maintenance-claim', 0))`).Scan(&acquired)
	if err != nil {
		return work, err
	}
	if !acquired {
		return work, provider.ErrMaintenanceIdle
	}
	lease, err := uuid.NewRandom()
	if err != nil {
		return work, err
	}
	err = tx.QueryRowContext(ctx, `
		WITH selected AS (
			SELECT i.id FROM provider_maintenance_items i
			WHERE i.state IN ('queued','running') AND i.next_at <= now()
				AND (i.lease_until IS NULL OR i.lease_until <= now())
				AND (SELECT count(*) FROM provider_maintenance_items WHERE lease_until > now()) < $1
				AND NOT EXISTS (
					SELECT 1 FROM provider_maintenance_items active WHERE active.lease_until > now()
						AND COALESCE(active.replacement_id, active.provider_id) = COALESCE(i.replacement_id, i.provider_id)
				)
			ORDER BY i.next_at, i.id LIMIT 1 FOR UPDATE OF i SKIP LOCKED
		)
		UPDATE provider_maintenance_items i SET state='running', started_at=COALESCE(started_at,now()), lease=$2,
			lease_until=now()+($3::bigint * interval '1 second')
		FROM selected WHERE i.id=selected.id RETURNING i.id, i.run_id, i.started_at`,
		provider.MaintenanceWorkers, lease, int(maintenanceLeasePeriod/time.Second)).Scan(&work.ID, &work.RunID, &work.StartedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return work, provider.ErrMaintenanceIdle
	}
	if err != nil {
		return work, err
	}
	work.Lease = lease.String()
	work.Item, err = scanMaintenanceItem(tx.QueryRowContext(ctx,
		"SELECT "+maintenanceItemColumns+" FROM provider_maintenance_items i WHERE i.id=$1", work.ID))
	if err != nil {
		return work, err
	}
	var draft, observed []byte
	err = tx.QueryRowContext(ctx, `UPDATE provider_maintenance_runs r
		SET status='running', started_at=COALESCE(r.started_at,now())
		FROM provider_maintenance_items i WHERE r.id=i.run_id AND i.id=$1
		RETURNING COALESCE(r.plan_id,0), r.plan_revision, r.draft, r.trigger, i.observed`, work.ID).
		Scan(&work.Plan.ID, &work.Plan.Revision, &draft, &work.Trigger, &observed)
	if err != nil {
		return work, err
	}
	if err := errors.Join(json.Unmarshal(draft, &work.Plan.MaintenanceDraft), json.Unmarshal(observed, &work.Guard)); err != nil {
		return work, err
	}
	return work, tx.Commit()
}

// Finish 防止旧租约和旧计划写入结果。提供商处置与进度使用同一事务。
func (s *MaintenanceStore) Finish(ctx context.Context, work provider.MaintenanceWork, update provider.MaintenanceUpdate) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if update.Stage == "verify" && update.ReplacementID > 0 {
		// 替换账号引用可能更新多个计划。先串行取得这类事务的锁，再锁单个计划。
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('tokenrouter:provider-maintenance-replacements', 0))`); err != nil {
			return err
		}
	}
	// 单次复检的配置冻结在轮次中，普通计划仍需核对当前版本。
	plan := work.Plan
	if work.Trigger != provider.MaintenancePelican {
		plan, err = readMaintenancePlan(ctx, tx, work.Plan.ID, true)
		if err != nil {
			return err
		}
	}
	item, err := scanMaintenanceItem(tx.QueryRowContext(ctx, "SELECT "+maintenanceItemColumns+`
		FROM provider_maintenance_items i WHERE i.id=$1 AND i.run_id=$2 AND i.state='running'
			AND i.lease=$3::uuid AND i.lease_until>clock_timestamp() FOR UPDATE`, work.ID, work.RunID, work.Lease))
	if errors.Is(err, sql.ErrNoRows) {
		return provider.ErrMaintenanceChanged
	}
	if err != nil {
		return err
	}
	if plan.Revision != work.Plan.Revision && item.OperationID == "" {
		return provider.ErrMaintenanceChanged
	}
	changedID, err := s.applyMaintenanceUpdate(ctx, tx, work, &item, &update)
	if err != nil {
		return err
	}
	if err := saveMaintenanceUpdate(ctx, tx, work, item, update); err != nil {
		return err
	}
	if err := finalizeMaintenanceRuns(ctx, tx, work.Plan.ID, work.RunID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if changedID > 0 {
		s.providers.afterChangeDetached(ctx, changedID)
	}
	return nil
}

func saveMaintenanceUpdate(ctx context.Context, tx *sql.Tx, work provider.MaintenanceWork, item provider.MaintenanceItem, update provider.MaintenanceUpdate) error {
	result := update.MaintenanceResult
	if result.Stage == "" {
		result.Stage = item.Stage
	}
	if result.Probe == nil {
		result.Probe = item.Probe
	}
	result.AuthHTTPStatus = item.AuthHTTPStatus
	if item.Stage == "auth" && result.HTTPStatus != 0 {
		result.AuthHTTPStatus = result.HTTPStatus
	}
	state := "queued"
	if result.Outcome != "" {
		state = "finished"
	} else if item.OperationID != "" {
		state = "running"
	}
	if update.ReplacementID > 0 {
		item.ReplacementID = update.ReplacementID
	}
	guard := update.Guard
	if guard.Revision == 0 {
		guard = work.Guard
	}
	observed, err := json.Marshal(guard)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	var next *time.Time
	if !update.RetryAt.IsZero() {
		next = &update.RetryAt
	}
	saved, err := tx.ExecContext(ctx, `
		UPDATE provider_maintenance_items
		SET stage=$3, state=$4, result=$5, observed=$6, operation_id=NULLIF($7,'')::uuid,
			replacement_id=NULLIF($8::bigint,0), next_at=COALESCE($9::timestamptz,now()), lease=NULL, lease_until=NULL,
			finished_at=CASE WHEN $4='finished' THEN now() END
		WHERE id=$1 AND lease=$2::uuid AND lease_until>clock_timestamp()`,
		work.ID, work.Lease, result.Stage, state, string(raw), string(observed), item.OperationID, item.ReplacementID, next)
	if err != nil {
		return err
	}
	count, err := saved.RowsAffected()
	if err == nil && count != 1 {
		return provider.ErrMaintenanceChanged
	}
	return err
}

// replaceMaintenanceProvider 使用重登执行器已核实的替换关系更新所有指定账号的计划。
func replaceMaintenanceProvider(ctx context.Context, tx *sql.Tx, oldID, newID int64) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE provider_maintenance_plans p SET draft=jsonb_set(p.draft,'{provider_ids}',(
			SELECT jsonb_agg(id ORDER BY pos) FROM (
				SELECT CASE WHEN value::bigint=$1 THEN $2::bigint ELSE value::bigint END AS id, min(ord) AS pos
				FROM jsonb_array_elements_text(p.draft->'provider_ids') WITH ORDINALITY a(value, ord)
				GROUP BY 1
			) ids
		))
		WHERE p.draft->>'scope'='providers' AND p.draft->'provider_ids' @> jsonb_build_array($1::bigint)`, oldID, newID)
	return err
}
