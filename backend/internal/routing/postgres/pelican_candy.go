package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"

	postgresinfra "github.com/TokenFlux/TokenRouter/internal/infra/postgres"
	"github.com/TokenFlux/TokenRouter/internal/routing"
)

// pelicanCandyDisabler 写入提供商状态及同事务的调度事件。
type pelicanCandyDisabler func(context.Context, postgresinfra.Executor, int64, int64, int64, time.Time) (bool, error)

// BindCandyDisabler 由应用提供持久化停调的实现。
func (s *PelicanTestStore) BindCandyDisabler(disable pelicanCandyDisabler) {
	s.disableCandy = disable
}

// OpenCandyRound 初始化一次候选快照，接手过期租约时标记上一次中断的尝试。
func (s *PelicanTestStore) OpenCandyRound(ctx context.Context, plan routing.PelicanPlan, candidates []int64) (routing.PelicanCandyRound, error) {
	var round routing.PelicanCandyRound
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return round, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockCandyLease(ctx, tx, plan); err != nil {
		return round, err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO pelican_candy_rounds (plan_id, id, candidate_ids)
		VALUES ($1,$2,COALESCE($3::bigint[],'{}')) ON CONFLICT (plan_id) DO NOTHING`, plan.ID, plan.LeaseToken, pq.Array(candidates))
	if err != nil {
		return round, err
	}
	var captured, tried pq.Int64Array
	err = tx.QueryRowContext(ctx, `
		SELECT cr.id, cr.candidate_ids, cr.busy_since, ARRAY(
			SELECT a.provider_id FROM pelican_candy_attempts a WHERE a.round_id=cr.id
		)
		FROM pelican_candy_rounds cr WHERE cr.plan_id=$1`, plan.ID).Scan(&round.ID, &captured, &round.BusySince, &tried)
	if err != nil {
		return round, err
	}
	round.CandidateIDs, round.TriedIDs = captured, tried
	_, err = tx.ExecContext(ctx, `
		UPDATE pelican_candy_attempts SET finished_at=statement_timestamp(), status='inconclusive', detail_code='interrupted'
		WHERE round_id=$1 AND finished_at IS NULL`, round.ID)
	if err != nil {
		return round, err
	}
	return round, tx.Commit()
}

// BeginCandyAttempt 在请求发出前登记本轮尝试，冻结账号配置和归组身份。
func (s *PelicanTestStore) BeginCandyAttempt(ctx context.Context, plan routing.PelicanPlan, roundID string, target routing.PelicanTarget) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockCandyLease(ctx, tx, plan); err != nil {
		return false, err
	}
	insert, err := tx.ExecContext(ctx, `
		INSERT INTO pelican_candy_attempts
			(round_id, provider_id, plan_id, group_id, provider_revision, group_updated_at, membership_created_at)
		SELECT cr.id, v.id, cr.plan_id, g.id, v.pelican_candy_revision, g.updated_at, pg.created_at
		FROM pelican_candy_rounds cr
		JOIN provider_groups pg ON pg.group_id=$4 AND pg.provider_id=$3
		JOIN groups g ON g.id=pg.group_id AND g.updated_at=$5 AND g.status='active' AND g.deleted_at IS NULL
		JOIN providers v ON v.id=pg.provider_id AND v.schedulable AND v.status='active' AND v.deleted_at IS NULL
		WHERE cr.plan_id=$1 AND cr.id=$2 AND v.id=ANY(cr.candidate_ids)
		ON CONFLICT (round_id, provider_id) DO NOTHING`, plan.ID, roundID, target.ProviderID, plan.GroupID, target.GroupUpdatedAt)
	if err != nil {
		return false, err
	}
	count, err := insert.RowsAffected()
	if err != nil || count == 0 {
		return false, err
	}
	return true, tx.Commit()
}

// DeferCandyRound 释放 worker，保留本轮首次遇到忙碌的时间。
func (s *PelicanTestStore) DeferCandyRound(ctx context.Context, plan routing.PelicanPlan, roundID string, retryAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockCandyLease(ctx, tx, plan); err != nil {
		return err
	}
	update, err := tx.ExecContext(ctx, `
		UPDATE pelican_candy_rounds SET retry_at=$3, busy_since=COALESCE(busy_since, statement_timestamp())
		WHERE plan_id=$1 AND id=$2`, plan.ID, roundID, retryAt)
	if err := checkPelicanAffected(update, err, routing.ErrPelicanConflict); err != nil {
		return err
	}
	if err := releaseCandyLease(ctx, tx, plan); err != nil {
		return err
	}
	return tx.Commit()
}

// FinishCandyAttempt 把作答、处置和本轮进度写进同一事务。
func (s *PelicanTestStore) FinishCandyAttempt(ctx context.Context, plan routing.PelicanPlan, roundID string, result routing.PelicanResult, next *time.Time) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	group, settings, err := lockPelicanPublication(ctx, tx, plan)
	if err != nil {
		return "", err
	}
	if err := lockCandyLease(ctx, tx, plan); err != nil {
		return "", err
	}
	result.PlanID, result.GroupID = plan.ID, group
	result.RoundID = roundID
	if result.ProviderID != nil {
		result.ScheduleAction, err = s.finishCandyProvider(ctx, tx, result)
		if err != nil {
			return "", err
		}
	}
	if err := insertPelicanResult(ctx, tx, plan, result); err != nil {
		return "", err
	}
	if err := saveCandyProgress(ctx, tx, plan, roundID, result, next); err != nil {
		return "", err
	}
	if err := prunePelicanShowcase(ctx, tx, settings, &group, plan.ID); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return result.ScheduleAction, nil
}

func (s *PelicanTestStore) finishCandyProvider(ctx context.Context, tx *sql.Tx, result routing.PelicanResult) (string, error) {
	id := *result.ProviderID
	var revision int64
	var membership time.Time
	var groupUnchanged bool
	err := tx.QueryRowContext(ctx, `
		SELECT a.provider_revision, a.membership_created_at, g.updated_at=a.group_updated_at AND g.status='active'
		FROM pelican_candy_attempts a
		JOIN pelican_candy_rounds cr ON cr.id=a.round_id AND cr.plan_id=a.plan_id
		JOIN groups g ON g.id=a.group_id
		WHERE a.round_id=$1 AND a.provider_id=$2 AND a.plan_id=$3 AND a.finished_at IS NULL`,
		result.RoundID, id, result.PlanID).Scan(&revision, &membership, &groupUnchanged)
	if errors.Is(err, sql.ErrNoRows) {
		return "", routing.ErrPelicanConflict
	}
	if err != nil {
		return "", err
	}
	action := ""
	if result.Status == "suspected" && result.DetailCode == "answer_mismatch" {
		action, err = s.disableCandyProvider(ctx, tx, result.GroupID, id, revision, membership, groupUnchanged)
		if err != nil {
			return "", err
		}
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE pelican_candy_attempts SET finished_at=$3, status=$4, detail_code=$5, schedule_action=$6
		WHERE round_id=$1 AND provider_id=$2`, result.RoundID, id, result.TestedAt, result.Status, result.DetailCode, action)
	return action, err
}

func (s *PelicanTestStore) disableCandyProvider(ctx context.Context, tx *sql.Tx, group, id, revision int64, membership time.Time, groupUnchanged bool) (string, error) {
	if !groupUnchanged {
		return routing.PelicanScheduleAccountChanged, nil
	}
	if s.disableCandy == nil {
		return "", errors.New("糖果题停调写入尚未绑定")
	}
	changed, err := s.disableCandy(ctx, tx, id, group, revision, membership)
	if err != nil {
		return "", err
	}
	if changed {
		return routing.PelicanScheduleDisabled, nil
	}
	var alreadyDisabled bool
	err = tx.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM providers WHERE id=$1 AND deleted_at IS NULL AND NOT schedulable)`, id).Scan(&alreadyDisabled)
	if alreadyDisabled {
		return routing.PelicanScheduleAlreadyDisabled, err
	}
	return routing.PelicanScheduleAccountChanged, err
}

func saveCandyProgress(ctx context.Context, tx *sql.Tx, plan routing.PelicanPlan, roundID string, result routing.PelicanResult, next *time.Time) error {
	if result.ProviderID == nil || result.Status == "passed" {
		remove, err := tx.ExecContext(ctx, `DELETE FROM pelican_candy_rounds WHERE plan_id=$1 AND id=$2`, plan.ID, roundID)
		if err := checkPelicanAffected(remove, err, routing.ErrPelicanConflict); err != nil {
			return err
		}
		return advancePelicanPlan(ctx, tx, plan, result.TestedAt, next)
	}
	update, err := tx.ExecContext(ctx, `
		UPDATE pelican_candy_rounds SET retry_at=NULL, busy_since=NULL WHERE plan_id=$1 AND id=$2`, plan.ID, roundID)
	if err := checkPelicanAffected(update, err, routing.ErrPelicanConflict); err != nil {
		return err
	}
	return releaseCandyLease(ctx, tx, plan)
}

func lockCandyLease(ctx context.Context, tx *sql.Tx, plan routing.PelicanPlan) error {
	var id int64
	err := tx.QueryRowContext(ctx, `
		SELECT p.id FROM pelican_test_plans p JOIN groups g ON g.id=p.group_id
		WHERE p.id=$1 AND p.group_id=$2 AND p.lease_token=$3 AND p.plan_revision=$4
			AND p.lease_until > statement_timestamp() AND p.suspend_on_wrong_answer
			AND g.status='active' AND g.deleted_at IS NULL
		FOR NO KEY UPDATE OF p`, plan.ID, plan.GroupID, plan.LeaseToken, plan.Revision).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return routing.ErrPelicanConflict
	}
	return err
}

func releaseCandyLease(ctx context.Context, tx *sql.Tx, plan routing.PelicanPlan) error {
	update, err := tx.ExecContext(ctx, `
		UPDATE pelican_test_plans SET lease_token=NULL, lease_until=NULL WHERE id=$1 AND lease_token=$2`, plan.ID, plan.LeaseToken)
	return checkPelicanAffected(update, err, routing.ErrPelicanConflict)
}
