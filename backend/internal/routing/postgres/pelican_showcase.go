package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/lib/pq"

	"github.com/TokenFlux/TokenRouter/internal/routing"
)

const (
	pelicanSettingsFields = `enabled, api_enabled, max_items, auto_cleanup, retention_days`
	pelicanFreshDrawing   = `r.published AND (NOT s.auto_cleanup OR
		r.tested_at >= statement_timestamp() - s.retention_days * interval '24 hours')`
	pelicanPublishedDrawing = pelicanHasDrawing + ` AND r.status IN ` + pelicanDrawingStatuses + ` AND ` + pelicanFreshDrawing
	pelicanPlanSample       = `(r.question_kind NOT IN ` + pelicanDrawingKinds + ` OR r.status='inconclusive')`
)

// ShowcaseSettings 读取网页、API 展示开关和分组作品保留设置。
func (s *PelicanTestStore) ShowcaseSettings(ctx context.Context) (routing.PelicanShowcaseSettings, error) {
	return scanPelicanSettings(s.db.QueryRowContext(ctx, `
		SELECT `+pelicanSettingsFields+` FROM pelican_showcase_settings WHERE singleton`))
}

// SaveShowcaseSettings 写入设置并在同一事务中裁剪各组已收录的作品。
func (s *PelicanTestStore) SaveShowcaseSettings(ctx context.Context, value routing.PelicanShowcaseSettings) (routing.PelicanShowcaseSettings, error) {
	var empty routing.PelicanShowcaseSettings
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return empty, err
	}
	defer func() { _ = tx.Rollback() }()
	saved, err := scanPelicanSettings(tx.QueryRowContext(ctx, `
		UPDATE pelican_showcase_settings SET enabled=$1, api_enabled=$2, max_items=$3, auto_cleanup=$4, retention_days=$5
		WHERE singleton RETURNING `+pelicanSettingsFields, value.Enabled, value.APIEnabled, value.MaxItems, value.AutoCleanup, value.RetentionDays))
	if err != nil {
		return empty, pelicanWriteError(err)
	}
	if err := prunePelicanShowcase(ctx, tx, saved, nil, 0); err != nil {
		return empty, err
	}
	if err := tx.Commit(); err != nil {
		return empty, err
	}
	return saved, nil
}

// PruneShowcase 清理到期作品，设置行的写锁让整次清理使用同一份设置。
func (s *PelicanTestStore) PruneShowcase(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	settings, err := scanPelicanSettings(tx.QueryRowContext(ctx, `
		SELECT `+pelicanSettingsFields+` FROM pelican_showcase_settings WHERE singleton FOR UPDATE`))
	if err != nil {
		return err
	}
	if err := prunePelicanShowcase(ctx, tx, settings, nil, 0); err != nil {
		return err
	}
	return tx.Commit()
}

func scanPelicanSettings(row *sql.Row) (routing.PelicanShowcaseSettings, error) {
	var settings routing.PelicanShowcaseSettings
	err := row.Scan(&settings.Enabled, &settings.APIEnabled, &settings.MaxItems, &settings.AutoCleanup, &settings.RetentionDays)
	return settings, err
}

// lockPelicanPublication 在收录前锁定分组，让后提交的计划能看到同组刚收录的作品。
func lockPelicanPublication(ctx context.Context, tx *sql.Tx, plan routing.PelicanPlan) (int64, routing.PelicanShowcaseSettings, error) {
	var groupID int64
	var settings routing.PelicanShowcaseSettings
	err := tx.QueryRowContext(ctx, `
		SELECT g.id, s.max_items, s.auto_cleanup, s.retention_days FROM pelican_showcase_settings s
		CROSS JOIN pelican_test_plans p JOIN groups g ON g.id=p.group_id
		WHERE s.singleton AND p.id=$1 AND p.lease_token=$2
		FOR SHARE OF s FOR NO KEY UPDATE OF g`, plan.ID, plan.LeaseToken).Scan(
		&groupID, &settings.MaxItems, &settings.AutoCleanup, &settings.RetentionDays)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, settings, routing.ErrPelicanConflict
	}
	return groupID, settings, err
}

// prunePelicanShowcase 按分组撤下作品，再从受影响计划中删除最近三十次作答以外的未收录记录。
func prunePelicanShowcase(ctx context.Context, tx *sql.Tx, settings routing.PelicanShowcaseSettings, groupID *int64, finishedPlan int64) error {
	var affected pq.Int64Array
	err := tx.QueryRowContext(ctx, `
		WITH ranked AS (
			SELECT r.id, r.tested_at, row_number() OVER (
				PARTITION BY p.group_id ORDER BY r.tested_at DESC, r.id DESC
			) AS position
			FROM pelican_test_results r JOIN pelican_test_plans p ON p.id=r.plan_id
			WHERE r.published AND ($1::bigint IS NULL OR p.group_id=$1)
		), withdrawn AS (
			UPDATE pelican_test_results r SET published=false FROM ranked WHERE r.id=ranked.id
				AND (ranked.position>$2 OR ($3 AND ranked.tested_at < statement_timestamp() - $4::int * interval '24 hours'))
			RETURNING r.plan_id
		)
		SELECT ARRAY(SELECT plan_id FROM withdrawn UNION SELECT $5::bigint WHERE $5::bigint>0)`,
		groupID, settings.MaxItems, settings.AutoCleanup, settings.RetentionDays, finishedPlan).Scan(&affected)
	if err != nil || len(affected) == 0 {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		DELETE FROM pelican_test_results r WHERE NOT r.published AND r.id IN (
			SELECT older.id FROM unnest($1::bigint[]) AS changed(plan_id)
			CROSS JOIN LATERAL (
				SELECT id FROM pelican_test_results WHERE plan_id=changed.plan_id
				ORDER BY tested_at DESC, id DESC OFFSET 30
			) older
		)`, affected)
	return err
}
