package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/provider"
)

const (
	maintenancePlanColumns = `p.id, p.revision, p.draft, p.next_run_at,
		COALESCE(last.id, 0), last.scheduled_at, COALESCE(last.status, '')
		FROM provider_maintenance_plans p
		LEFT JOIN LATERAL (
			SELECT id, scheduled_at, status FROM provider_maintenance_runs
			WHERE plan_id=p.id ORDER BY id DESC LIMIT 1
		) last ON TRUE`
	maintenanceRunColumns = `r.id, COALESCE(r.plan_id,0), COALESCE(r.pelican_result_id,0), r.status, r.trigger, r.scheduled_at, r.started_at, r.finished_at,
		i.total, i.completed, i.running, i.total - i.completed
		FROM provider_maintenance_runs r
		CROSS JOIN LATERAL (
			SELECT count(*) AS total, count(*) FILTER (WHERE state IN ('finished', 'cancelled')) AS completed,
				count(*) FILTER (WHERE state='running') AS running
			FROM provider_maintenance_items WHERE run_id=r.id
		) i`
	maintenanceItemColumns = `i.target, i.state, i.stage, i.result,
		COALESCE(i.operation_id::text, ''), COALESCE(i.replacement_id, 0), i.finished_at`
)

// MaintenanceStore 保存巡检计划、轮次和每个账号的执行阶段。
type MaintenanceStore struct {
	db        *sql.DB
	providers *ProviderStore
	cooldown  time.Duration
}

type maintenanceScanner interface {
	Scan(...any) error
}

type providerRowQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func NewMaintenanceStore(db *sql.DB, providers *ProviderStore, cooldown time.Duration) *MaintenanceStore {
	if cooldown <= 0 {
		cooldown = 10 * time.Minute
	}
	return &MaintenanceStore{db: db, providers: providers, cooldown: cooldown}
}

func (s *MaintenanceStore) Plans(ctx context.Context) ([]provider.MaintenancePlan, error) {
	return s.readPlans(ctx, `ORDER BY p.id DESC`)
}

func (s *MaintenanceStore) Plan(ctx context.Context, id int64) (provider.MaintenancePlan, error) {
	return readMaintenancePlan(ctx, s.db, id, false)
}

func (s *MaintenanceStore) DuePlans(ctx context.Context, now time.Time) ([]provider.MaintenancePlan, error) {
	return s.readPlans(ctx, `WHERE p.enabled AND p.next_run_at <= $1 ORDER BY p.next_run_at, p.id LIMIT 50`, now)
}

func (s *MaintenanceStore) readPlans(ctx context.Context, filter string, args ...any) ([]provider.MaintenancePlan, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+maintenancePlanColumns+" "+filter, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	plans := make([]provider.MaintenancePlan, 0)
	for rows.Next() {
		plan, err := scanMaintenancePlan(rows)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return plans, rows.Err()
}

// SavePlan 变更后取消旧轮次的待派发项。已提交的重登保留其进度。
func (s *MaintenanceStore) SavePlan(ctx context.Context, plan provider.MaintenancePlan) (provider.MaintenancePlan, error) {
	draft, err := json.Marshal(plan.MaintenanceDraft)
	if err != nil {
		return provider.MaintenancePlan{}, err
	}
	if plan.ID == 0 {
		err = s.db.QueryRowContext(ctx, `INSERT INTO provider_maintenance_plans(draft, enabled, next_run_at)
			VALUES ($1, $2, $3) RETURNING id, revision, next_run_at`, string(draft), plan.Enabled, plan.NextRunAt).
			Scan(&plan.ID, &plan.Revision, &plan.NextRunAt)
		return plan, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return provider.MaintenancePlan{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := readMaintenancePlan(ctx, tx, plan.ID, true); err != nil {
		return provider.MaintenancePlan{}, err
	}
	res, err := tx.ExecContext(ctx, `UPDATE provider_maintenance_plans
		SET draft=$2, enabled=$3, next_run_at=$4, revision=revision+1
		WHERE id=$1 AND draft IS DISTINCT FROM $2::jsonb`, plan.ID, string(draft), plan.Enabled, plan.NextRunAt)
	if err != nil {
		return provider.MaintenancePlan{}, err
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return provider.MaintenancePlan{}, err
	}
	if changed > 0 {
		if err := cancelMaintenanceItems(ctx, tx, plan.ID); err != nil {
			return provider.MaintenancePlan{}, err
		}
	}
	saved, err := readMaintenancePlan(ctx, tx, plan.ID, false)
	if err != nil {
		return provider.MaintenancePlan{}, err
	}
	return saved, tx.Commit()
}

func cancelMaintenanceItems(ctx context.Context, tx *sql.Tx, planID int64) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE provider_maintenance_items i
		SET state='cancelled', result=i.result || '{"outcome":"cancelled","detail":"plan_changed"}',
			finished_at=now(), lease=NULL, lease_until=NULL
		FROM provider_maintenance_runs r WHERE r.id=i.run_id AND r.plan_id=$1
			AND i.state IN ('queued','running') AND i.operation_id IS NULL`, planID)
	if err != nil {
		return err
	}
	return finalizeMaintenanceRuns(ctx, tx, planID, 0)
}

// QueueRun 将同一计划的手动与定时触发合并。轮次和名单一起提交。
func (s *MaintenanceStore) QueueRun(ctx context.Context, plan provider.MaintenancePlan, trigger string, at time.Time, next *time.Time, targets []provider.MaintenanceTarget) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := readMaintenancePlan(ctx, tx, plan.ID, true)
	if err != nil {
		return 0, err
	}
	if current.Revision != plan.Revision || !slices.Equal(plan.ProviderIDs, current.ProviderIDs) {
		return 0, provider.ErrMaintenanceChanged
	}
	if trigger == provider.MaintenanceSchedule && (current.NextRunAt == nil || !current.NextRunAt.Equal(at)) {
		var id int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM provider_maintenance_runs
			WHERE plan_id=$1 AND trigger='schedule' AND scheduled_at=$2`, plan.ID, at).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, provider.ErrMaintenanceChanged
		}
		return id, err
	}
	if next != nil && current.NextRunAt != nil && plan.NextRunAt != nil && current.NextRunAt.Equal(*plan.NextRunAt) {
		if _, err := tx.ExecContext(ctx, `UPDATE provider_maintenance_plans SET next_run_at=$2 WHERE id=$1`, plan.ID, next); err != nil {
			return 0, err
		}
	}
	if current.LastRunStatus == "queued" || current.LastRunStatus == "running" {
		return current.LastRunID, tx.Commit()
	}
	// 调度器扫描时若已有一轮在途，本次到期并入该轮。
	if plan.LastRunStatus == "queued" || plan.LastRunStatus == "running" {
		return plan.LastRunID, tx.Commit()
	}
	id, err := insertMaintenanceRun(ctx, tx, current, trigger, at, targets)
	if err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func insertMaintenanceRun(ctx context.Context, tx *sql.Tx, plan provider.MaintenancePlan, trigger string, at time.Time, targets []provider.MaintenanceTarget) (int64, error) {
	draft, err := json.Marshal(plan.MaintenanceDraft)
	if err != nil {
		return 0, err
	}
	var id int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO provider_maintenance_runs(plan_id, plan_revision, draft, trigger, status, scheduled_at)
		VALUES ($1, $2, $3, $4, 'queued', $5) RETURNING id`, plan.ID, plan.Revision, string(draft), trigger, at).Scan(&id); err != nil {
		return 0, err
	}
	stage := "auth"
	if plan.Kind == provider.MaintenanceIQ {
		stage = "probe"
	}
	if len(targets) > 0 {
		data, err := json.Marshal(targets)
		if err != nil {
			return 0, err
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO provider_maintenance_items(run_id, provider_id, target, stage, state, result, finished_at)
			SELECT $1, (t->>'provider_id')::bigint, t, $2,
				CASE WHEN t ? 'reason' THEN 'finished' ELSE 'queued' END,
				CASE WHEN t ? 'reason' THEN jsonb_build_object('outcome','skipped','detail',t->>'reason') ELSE '{}'::jsonb END,
				CASE WHEN t ? 'reason' THEN now() END
			FROM jsonb_array_elements($3::jsonb) t
			ON CONFLICT (run_id, provider_id) DO NOTHING`, id, stage, string(data))
		if err != nil {
			return 0, err
		}
	}
	return id, finalizeMaintenanceRuns(ctx, tx, plan.ID, id)
}

func (s *MaintenanceStore) Runs(ctx context.Context, id int64) ([]provider.MaintenanceRun, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+maintenanceRunColumns+" WHERE r.plan_id=$1 ORDER BY r.id DESC LIMIT 30", id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	runs := make([]provider.MaintenanceRun, 0)
	for rows.Next() {
		run, err := scanMaintenanceRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (s *MaintenanceStore) Run(ctx context.Context, id int64) (provider.MaintenanceRunDetail, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return provider.MaintenanceRunDetail{}, err
	}
	defer func() { _ = tx.Rollback() }()
	run, err := scanMaintenanceRun(tx.QueryRowContext(ctx, "SELECT "+maintenanceRunColumns+" WHERE r.id=$1", id))
	if err != nil {
		return provider.MaintenanceRunDetail{}, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT "+maintenanceItemColumns+" FROM provider_maintenance_items i WHERE i.run_id=$1 ORDER BY i.id", id)
	if err != nil {
		return provider.MaintenanceRunDetail{}, err
	}
	defer func() { _ = rows.Close() }()
	detail := provider.MaintenanceRunDetail{MaintenanceRun: run, Items: make([]provider.MaintenanceItem, 0, run.Total)}
	for rows.Next() {
		item, err := scanMaintenanceItem(rows)
		if err != nil {
			return provider.MaintenanceRunDetail{}, err
		}
		detail.Items = append(detail.Items, item)
	}
	if err := rows.Err(); err != nil {
		return provider.MaintenanceRunDetail{}, err
	}
	return detail, tx.Commit()
}

func readMaintenancePlan(ctx context.Context, exec providerRowQueryer, id int64, lock bool) (provider.MaintenancePlan, error) {
	if lock {
		var locked int64
		err := exec.QueryRowContext(ctx, `SELECT id FROM provider_maintenance_plans WHERE id=$1 FOR UPDATE`, id).Scan(&locked)
		if errors.Is(err, sql.ErrNoRows) {
			err = provider.ErrMaintenanceNotFound
		}
		if err != nil {
			return provider.MaintenancePlan{}, err
		}
	}
	return scanMaintenancePlan(exec.QueryRowContext(ctx, "SELECT "+maintenancePlanColumns+" WHERE p.id=$1", id))
}

func scanMaintenancePlan(row maintenanceScanner) (provider.MaintenancePlan, error) {
	var plan provider.MaintenancePlan
	var raw []byte
	err := row.Scan(&plan.ID, &plan.Revision, &raw, &plan.NextRunAt, &plan.LastRunID, &plan.LastRunAt, &plan.LastRunStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return plan, provider.ErrMaintenanceNotFound
	}
	if err != nil {
		return plan, err
	}
	err = json.Unmarshal(raw, &plan.MaintenanceDraft)
	return plan, err
}

func scanMaintenanceRun(row maintenanceScanner) (provider.MaintenanceRun, error) {
	var run provider.MaintenanceRun
	err := row.Scan(&run.ID, &run.PlanID, &run.PelicanResultID, &run.Status, &run.Trigger, &run.ScheduledAt, &run.StartedAt,
		&run.FinishedAt, &run.Total, &run.Completed, &run.Running, &run.Remaining)
	if errors.Is(err, sql.ErrNoRows) {
		err = provider.ErrMaintenanceNotFound
	}
	return run, err
}

func scanMaintenanceItem(row maintenanceScanner) (provider.MaintenanceItem, error) {
	var item provider.MaintenanceItem
	var target, result []byte
	var stage string
	err := row.Scan(&target, &item.State, &stage, &result, &item.OperationID, &item.ReplacementID, &item.FinishedAt)
	if err != nil {
		return item, err
	}
	err = errors.Join(json.Unmarshal(target, &item.MaintenanceTarget), json.Unmarshal(result, &item.MaintenanceResult))
	item.Stage = stage
	return item, err
}

func finalizeMaintenanceRuns(ctx context.Context, tx *sql.Tx, planID, runID int64) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE provider_maintenance_runs r SET finished_at=now(),
			status=CASE WHEN EXISTS (SELECT 1 FROM provider_maintenance_items WHERE run_id=r.id AND state='cancelled')
				THEN 'cancelled' ELSE 'finished' END
		WHERE (($2::bigint>0 AND r.id=$2) OR ($2=0 AND r.plan_id=$1)) AND r.status IN ('queued','running')
			AND NOT EXISTS (SELECT 1 FROM provider_maintenance_items WHERE run_id=r.id AND state IN ('queued','running'))`, planID, runID)
	return err
}
