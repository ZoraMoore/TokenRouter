package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/TokenFlux/TokenRouter/internal/provider"
)

// Recheck 根据已保存的初测结果查找运行，重复请求返回同一轮次。
func (s *MaintenanceStore) Recheck(ctx context.Context, resultID int64) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, "SELECT id FROM provider_maintenance_runs WHERE pelican_result_id=$1", resultID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, provider.ErrMaintenanceNotFound
	}
	return id, err
}

// QueuePelicanRecheck 在初测结果的事务中登记待复检账号，账号和模型取自当次分组作答。
func QueuePelicanRecheck(ctx context.Context, tx *sql.Tx, resultID int64) (int64, error) {
	var target provider.MaintenanceTarget
	var observed provider.MaintenanceGuard
	var requested, prompt string
	err := tx.QueryRowContext(ctx, `
		SELECT r.model_id,r.prompt,r.answer,a.group_id,a.provider_id,COALESCE(v.name,''),
			a.request_model,a.actual_model,a.provider_revision,a.group_updated_at,a.membership_created_at
		FROM pelican_test_results r
		JOIN pelican_candy_attempts a ON a.round_id=r.round_id AND a.provider_id=r.provider_id AND a.plan_id=r.plan_id
		LEFT JOIN providers v ON v.id=a.provider_id
		WHERE r.id=$1 AND r.schedule_action='recheck_queued' AND r.target_provider_id IS NULL
			AND r.question_kind='candy' AND r.status='suspected' AND r.detail_code='answer_mismatch'`, resultID).
		Scan(&requested, &prompt, &target.InitialAnswer, &target.GroupID, &target.ProviderID, &target.ProviderName,
			&target.Model, &target.ActualModel, &observed.Revision, &observed.GroupVersion, &observed.MembershipAt)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, provider.ErrMaintenanceNotFound
	}
	if err != nil {
		return 0, err
	}
	if !provider.IsStandardPelicanCandy(prompt, "21") {
		return 0, provider.ErrMaintenanceInvalid
	}
	draft, err := json.Marshal(provider.MaintenanceDraft{
		Kind: provider.MaintenanceIQ, Scope: "groups", GroupIDs: []int64{target.GroupID}, Model: requested,
	})
	if err != nil {
		return 0, err
	}
	rawTarget, err := json.Marshal(target)
	if err != nil {
		return 0, err
	}
	guard, err := json.Marshal(observed)
	if err != nil {
		return 0, err
	}
	var id int64
	err = tx.QueryRowContext(ctx, `INSERT INTO provider_maintenance_runs(pelican_result_id,plan_revision,draft,trigger,status,scheduled_at)
		VALUES ($1,0,$2,'pelican','queued',now()) ON CONFLICT (pelican_result_id) DO NOTHING RETURNING id`, resultID, string(draft)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, `SELECT id FROM provider_maintenance_runs WHERE pelican_result_id=$1`, resultID).Scan(&id)
		return id, err
	}
	if err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO provider_maintenance_items(run_id,provider_id,target,observed,stage,state)
		VALUES ($1,$2,$3,$4,'probe','queued')`, id, target.ProviderID, string(rawTarget), string(guard))
	return id, err
}
