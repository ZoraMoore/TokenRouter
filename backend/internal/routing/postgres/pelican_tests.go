package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/lib/pq"

	"github.com/TokenFlux/TokenRouter/internal/routing"
)

const (
	// 手动排队时间在完成时清除。领取后先显示租约状态。
	pelicanRunState = `CASE
		WHEN p.lease_until > CURRENT_TIMESTAMP THEN 'running'
		WHEN p.queued_at IS NOT NULL OR EXISTS (SELECT 1 FROM pelican_candy_rounds cr WHERE cr.plan_id=p.id) THEN 'queued'
		ELSE 'idle' END`
	pelicanPlanFields = `p.id, p.group_id, g.name, p.provider_id, COALESCE(v.name,''),
		p.model_id, p.question_kind, p.question_name, p.question_prompt, p.expected_answer, p.reasoning_effort,
		p.cron_expression, p.enabled, p.last_run_at, p.next_run_at,
		` + pelicanRunState + `, COALESCE(p.queued_at, p.next_run_at), p.plan_revision, p.suspend_on_wrong_answer`
	pelicanPlanJoins = ` JOIN groups g ON g.id=p.group_id LEFT JOIN providers v ON v.id=p.provider_id `

	// 历史记录的提供商名称取自实际作答账号，指定账号由计划单独存入结果。
	pelicanSummaryFields = `r.id, r.model_id, r.question_kind, r.question_name, r.reasoning_effort,
		r.status, r.detail_code, r.tested_at, r.latency_ms, r.schedule_action`
	pelicanResultFields     = `r.plan_id, p.group_id, r.provider_id, COALESCE(v.name,''), r.round_id, mr.id, ` + pelicanSummaryFields
	pelicanUserResultFields = `r.plan_id, p.group_id, NULL::bigint, ''::text, ''::text, NULL::bigint, ` + pelicanSummaryFields
	pelicanResultFrom       = ` FROM pelican_test_results r
		JOIN pelican_test_plans p ON p.id=r.plan_id
		JOIN groups g ON g.id=p.group_id `
	pelicanAdminResultFrom = pelicanResultFrom + ` LEFT JOIN providers v ON v.id=r.provider_id
		LEFT JOIN provider_maintenance_runs mr ON mr.pelican_result_id=r.id `
	pelicanCurrentTarget = `r.plan_revision=p.plan_revision AND r.model_id=p.model_id
		AND r.question_kind=p.question_kind AND r.question_name=p.question_name
		AND r.target_provider_id IS NOT DISTINCT FROM p.provider_id`
	pelicanDrawingKinds    = `('pelican', 'custom_svg')`
	pelicanDrawingStatuses = `('pending_review', 'passed', 'suspected')`
	pelicanHasDrawing      = `r.question_kind IN ` + pelicanDrawingKinds + ` AND (r.answer<>'' OR r.raw_answer<>'')`
	pelicanHasCandy        = `r.question_kind='candy' AND r.status IN ('passed','suspected') AND r.answer ~ '[^[:space:]]'`

	// 糖果预览读取分组裁剪后的样本，正文在详情按同一条件读取。
	pelicanCandyAnswers = `CASE WHEN NOT $2 AND p.question_kind='candy' THEN (
		SELECT json_agg(sample ORDER BY sample.tested_at DESC, sample.id DESC)
		FROM (
			SELECT ` + pelicanSummaryFields + `,
				left(r.answer, 256) || CASE WHEN substring(r.answer FROM 257 FOR 1) <> '' THEN '…' ELSE '' END AS answer_preview
			FROM pelican_test_results r
			WHERE r.plan_id=p.id AND ` + pelicanCurrentTarget + ` AND ` + pelicanPublishedCandy + `
		) sample
	) END`
)

// PelicanTestStore 保存分组测智计划、领取凭证和每次作答。
type PelicanTestStore struct {
	db           *sql.DB
	queueRecheck pelicanRecheckQueuer
}

// NewPelicanTestStore 使用应用的 PostgreSQL 连接池保存测智任务。
func NewPelicanTestStore(db *sql.DB) *PelicanTestStore {
	return &PelicanTestStore{db: db}
}

// ListPlans 按分组和模型列出管理员可维护的计划。
func (s *PelicanTestStore) ListPlans(ctx context.Context) ([]routing.PelicanPlan, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+pelicanPlanFields+`
		FROM pelican_test_plans p `+pelicanPlanJoins+`
		WHERE g.deleted_at IS NULL
		ORDER BY p.group_id, p.model_id, p.question_kind, p.question_name, p.id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	plans := []routing.PelicanPlan{}
	for rows.Next() {
		plan, err := scanPelicanPlan(rows)
		if err != nil {
			return nil, err
		}
		plans = append(plans, *plan)
	}
	return plans, rows.Err()
}

// SavePlan 创建或更新计划。修改已经过期的任务会清除旧 worker 的领取凭证。
func (s *PelicanTestStore) SavePlan(ctx context.Context, plan routing.PelicanPlan) (*routing.PelicanPlan, error) {
	mutation := `INSERT INTO pelican_test_plans
		(group_id, provider_id, model_id, question_kind, cron_expression, enabled, next_run_at,
		 question_name, question_prompt, expected_answer, reasoning_effort, suspend_on_wrong_answer)
		SELECT g.id, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12 FROM groups g
		WHERE g.id=$1 AND g.deleted_at IS NULL RETURNING *`
	args := []any{
		plan.GroupID, plan.ProviderID, plan.ModelID, plan.QuestionKind, plan.CronExpression, plan.Enabled, plan.NextRunAt,
		plan.QuestionName, plan.QuestionPrompt, plan.ExpectedAnswer, plan.ReasoningEffort, plan.SuspendOnWrongAnswer,
	}
	if plan.ID != 0 {
		// 改题或调整推理强度会重置计划的最近结论。已发布作品按分组单独保留。
		mutation = `UPDATE pelican_test_plans p SET
			provider_id=$2, model_id=$3, question_kind=$4, cron_expression=$5, enabled=$6,
			next_run_at=$7, lease_token=NULL, lease_until=NULL,
			question_name=$8, question_prompt=$9, expected_answer=$10, reasoning_effort=$11, suspend_on_wrong_answer=$12,
			plan_revision=p.plan_revision + CASE WHEN
				(p.question_kind, p.question_name, p.question_prompt, p.expected_answer, p.reasoning_effort)
					IS DISTINCT FROM ($4, $8, $9, $10, $11)
				OR (p.question_kind='candy' AND (p.provider_id, p.model_id) IS DISTINCT FROM ($2, $3))
				THEN 1 ELSE 0 END
			FROM groups g WHERE p.id=$13 AND p.group_id=$1 AND p.group_id=g.id AND g.deleted_at IS NULL
				AND p.queued_at IS NULL AND (p.lease_until IS NULL OR p.lease_until <= CURRENT_TIMESTAMP)
			RETURNING p.*`
		args = append(args, plan.ID)
	}
	saved, err := scanPelicanPlan(s.db.QueryRowContext(ctx,
		`WITH saved AS (`+mutation+`), canceled AS (
			DELETE FROM pelican_candy_rounds cr USING saved WHERE cr.plan_id=saved.id
		)
		SELECT `+pelicanPlanFields+` FROM saved p `+pelicanPlanJoins, args...))
	if !errors.Is(err, sql.ErrNoRows) {
		if err == nil {
			saved.RunState = "idle"
		}
		return saved, pelicanWriteError(err)
	}
	if plan.ID == 0 {
		return nil, routing.ErrPelicanInvalid
	}
	var groupID int64
	err = s.db.QueryRowContext(ctx, `SELECT p.group_id FROM pelican_test_plans p
		JOIN groups g ON g.id=p.group_id WHERE p.id=$1 AND g.deleted_at IS NULL`, plan.ID).Scan(&groupID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, routing.ErrPelicanNotFound
	case err != nil:
		return nil, err
	case groupID != plan.GroupID:
		return nil, routing.ErrPelicanInvalid
	default:
		return nil, routing.ErrPelicanConflict
	}
}

// DeletePlan 删除计划，数据库按外键一并删除作答记录。
func (s *PelicanTestStore) DeletePlan(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM pelican_test_plans WHERE id=$1", id)
	return checkPelicanAffected(result, err, routing.ErrPelicanNotFound)
}

// QueuePlan 为活跃分组登记一次手动运行，暂停的计划也可登记。
func (s *PelicanTestStore) QueuePlan(ctx context.Context, id int64) error {
	return pelicanMutationState(s.db.QueryRowContext(ctx, `
		WITH target AS (
			SELECT p.id, p.queued_at, p.lease_until
			FROM pelican_test_plans p JOIN groups g ON g.id=p.group_id
			WHERE p.id=$1 AND g.status='active' AND g.deleted_at IS NULL
			FOR UPDATE OF p
		), queued AS (
			UPDATE pelican_test_plans p SET queued_at=CURRENT_TIMESTAMP, lease_token=NULL, lease_until=NULL
			FROM target WHERE p.id=target.id AND target.queued_at IS NULL
				AND (target.lease_until IS NULL OR target.lease_until <= CURRENT_TIMESTAMP)
				AND NOT EXISTS (SELECT 1 FROM pelican_candy_rounds cr WHERE cr.plan_id=p.id)
			RETURNING p.id
		)
		SELECT EXISTS(SELECT 1 FROM target), EXISTS(SELECT 1 FROM queued)`, id))
}

// ClaimNext 领取最早到期的计划。租约过期时，接手的 worker 使用同一出题时间。
func (s *PelicanTestStore) ClaimNext(ctx context.Context, now, until time.Time, token string) (*routing.PelicanPlan, error) {
	if token == "" || !until.After(now) {
		return nil, routing.ErrPelicanInvalid
	}
	plan, err := scanPelicanPlan(s.db.QueryRowContext(ctx, `
		WITH due AS (
			SELECT p.id FROM pelican_test_plans p JOIN groups g ON g.id=p.group_id
			LEFT JOIN pelican_candy_rounds cr ON cr.plan_id=p.id
			WHERE g.status='active' AND g.deleted_at IS NULL
				AND (p.queued_at IS NOT NULL OR (p.enabled AND p.next_run_at <= $1))
				AND (p.lease_until IS NULL OR p.lease_until <= $1)
				AND (cr.retry_at IS NULL OR cr.retry_at <= $1)
			ORDER BY COALESCE(p.queued_at, p.next_run_at), p.id
			LIMIT 1 FOR UPDATE OF p SKIP LOCKED
		), claimed AS (
			UPDATE pelican_test_plans p SET lease_token=$3,
				lease_until=CASE WHEN p.question_kind IN `+pelicanDrawingKinds+`
					THEN $2::timestamptz ELSE LEAST($2::timestamptz, $4::timestamptz) END
			FROM due WHERE p.id=due.id RETURNING p.*
		)
		SELECT `+pelicanPlanFields+` FROM claimed p `+pelicanPlanJoins, now, until, token, now.Add(routing.PelicanDefaultLease)))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	plan.LeaseToken = token
	plan.RunState = "running"
	return plan, nil
}

// Finish 按领取凭证提交作答，和计划进度、历史清理在同一事务内生效。
func (s *PelicanTestStore) Finish(ctx context.Context, plan routing.PelicanPlan, result routing.PelicanResult, next *time.Time) error {
	if plan.LeaseToken == "" {
		return routing.ErrPelicanConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	groupID, settings, err := lockPelicanPublication(ctx, tx, plan)
	if err != nil {
		return err
	}
	if _, err := insertPelicanResult(ctx, tx, plan, result); err != nil {
		return err
	}
	if err := advancePelicanPlan(ctx, tx, plan, result.TestedAt, next); err != nil {
		return err
	}
	if err := prunePelicanShowcase(ctx, tx, settings, &groupID, plan.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func insertPelicanResult(ctx context.Context, tx *sql.Tx, plan routing.PelicanPlan, result routing.PelicanResult) (int64, error) {
	if result.Criteria == nil {
		result.Criteria = []string{}
	}
	criteria, err := json.Marshal(result.Criteria)
	if err != nil {
		return 0, err
	}
	// 结果使用被锁定的计划配置，自动选号时也要保存当次实际作答账号。
	var id int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO pelican_test_results
			(plan_id, target_provider_id, provider_id, model_id, question_kind, question_name, plan_revision,
			 status, detail_code, prompt, criteria, answer, tested_at, latency_ms, reasoning_effort, published, raw_answer,
			 round_id, schedule_action)
		SELECT id, provider_id, $3, model_id, question_kind, question_name, plan_revision,
			$4, $5, $6, $7::jsonb, $8, $9, $10, $11,
			CASE
				WHEN question_kind IN `+pelicanDrawingKinds+` THEN $4::text IN `+pelicanDrawingStatuses+` AND ($8::text<>'' OR $12::text<>'')
				WHEN question_kind='candy' THEN $4::text IN ('passed','suspected') AND $8::text ~ '[^[:space:]]'
				ELSE false
			END, $12, $13, $14
		FROM pelican_test_plans WHERE id=$1 AND lease_token=$2 RETURNING id`, plan.ID, plan.LeaseToken, result.ProviderID, result.Status,
		result.DetailCode, result.Prompt, string(criteria), result.Answer, result.TestedAt, result.LatencyMs,
		result.ReasoningEffort, result.RawAnswer, result.RoundID, result.ScheduleAction).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, routing.ErrPelicanConflict
	}
	return id, pelicanWriteError(err)
}

func advancePelicanPlan(ctx context.Context, tx *sql.Tx, plan routing.PelicanPlan, tested time.Time, next *time.Time) error {
	update, err := tx.ExecContext(ctx, `
		UPDATE pelican_test_plans SET last_run_at=$3, queued_at=NULL, lease_token=NULL, lease_until=NULL,
			next_run_at=CASE WHEN enabled THEN $4::timestamptz END
		WHERE id=$1 AND lease_token=$2`, plan.ID, plan.LeaseToken, tested, next)
	return checkPelicanAffected(update, err, routing.ErrPelicanConflict)
}

// ListResults 返回管理员可查的计划历史，正文在打开详情时读取。
func (s *PelicanTestStore) ListResults(ctx context.Context, planID int64) ([]routing.PelicanResult, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+pelicanResultFields+pelicanAdminResultFrom+`
		WHERE p.id=$1 AND g.deleted_at IS NULL ORDER BY r.tested_at DESC, r.id DESC LIMIT 30`, planID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	results := []routing.PelicanResult{}
	for rows.Next() {
		var result routing.PelicanResult
		if err := rows.Scan(pelicanResultDest(&result)...); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, rows.Err()
}

// Result 读取一份完整作答。组内用户可打开已发布的绘图和当前计划的测试记录。
func (s *PelicanTestStore) Result(ctx context.Context, id int64, allowed []int64) (*routing.PelicanResult, error) {
	fields, from := pelicanResultFields, pelicanAdminResultFrom
	filter := ` WHERE r.id=$1 AND g.deleted_at IS NULL`
	args := []any{id}
	if allowed != nil {
		if len(allowed) == 0 {
			return nil, routing.ErrPelicanNotFound
		}
		fields, from = pelicanUserResultFields, pelicanResultFrom+` CROSS JOIN pelican_showcase_settings s`
		filter += ` AND s.singleton AND g.status='active' AND p.group_id=ANY($2)
			AND ((` + pelicanCurrentTarget + ` AND ` + pelicanPlanSample + `) OR (` + pelicanPublishedDrawing + `))`
		args = append(args, pq.Array(allowed))
	}
	var result routing.PelicanResult
	var criteria []byte
	dest := append(pelicanResultDest(&result), &result.Prompt, &criteria, &result.Answer, &result.RawAnswer)
	query := `SELECT ` + fields + `, r.prompt, r.criteria, r.answer, r.raw_answer` + from + filter
	err := s.db.QueryRowContext(ctx, query, args...).Scan(dest...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, routing.ErrPelicanNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(criteria, &result.Criteria); err != nil {
		return nil, err
	}
	return &result, nil
}

// Latest 返回网页可见的计划结论、绘图摘要和已判定的糖果题回答。
func (s *PelicanTestStore) Latest(ctx context.Context, allowed []int64) ([]routing.PelicanLatest, error) {
	return s.latest(ctx, allowed, false)
}

// LatestDrawings 为 Key 的作品列表读取已发布的作品，按作答时的模型和题型返回摘要。
func (s *PelicanTestStore) LatestDrawings(ctx context.Context, allowed []int64) ([]routing.PelicanLatest, error) {
	return s.latest(ctx, allowed, true)
}

func (s *PelicanTestStore) latest(ctx context.Context, allowed []int64, drawingsOnly bool) ([]routing.PelicanLatest, error) {
	results := []routing.PelicanLatest{}
	if len(allowed) == 0 {
		return results, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.group_id, p.model_id, p.question_kind, p.question_name, p.reasoning_effort, p.enabled, `+pelicanRunState+`,
			p.next_run_at, row_to_json(sample), gallery.artworks, `+pelicanCandyAnswers+`
		FROM pelican_test_plans p JOIN groups g ON g.id=p.group_id
		CROSS JOIN pelican_showcase_settings s
		LEFT JOIN LATERAL (
			SELECT `+pelicanSummaryFields+` FROM pelican_test_results r
			WHERE NOT $2 AND r.plan_id=p.id AND `+pelicanCurrentTarget+`
				AND (`+pelicanPlanSample+` OR (`+pelicanPublishedDrawing+`))
			ORDER BY r.tested_at DESC, r.id DESC LIMIT 1
		) sample ON true
		CROSS JOIN LATERAL (
			SELECT COALESCE(json_agg(drawing ORDER BY drawing.tested_at DESC, drawing.id DESC), '[]') AS artworks
			FROM (
				SELECT `+pelicanSummaryFields+` FROM pelican_test_results r
				WHERE r.plan_id=p.id AND `+pelicanPublishedDrawing+` AND (NOT $2 OR r.answer LIKE '<svg%')
			) drawing
		) gallery
		WHERE s.singleton AND p.group_id=ANY($1) AND g.status='active' AND g.deleted_at IS NULL
			AND (NOT $2 OR p.question_kind IN `+pelicanDrawingKinds+` OR json_array_length(gallery.artworks)>0)
		ORDER BY p.group_id, p.model_id, p.question_kind, p.question_name, p.id`, pq.Array(allowed), drawingsOnly)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var item routing.PelicanLatest
		var sample, artworks, answers []byte
		if err := rows.Scan(&item.GroupID, &item.ModelID, &item.QuestionKind, &item.QuestionName, &item.ReasoningEffort, &item.Enabled,
			&item.RunState, &item.NextRunAt, &sample, &artworks, &answers); err != nil {
			return nil, err
		}
		if sample != nil {
			if err := json.Unmarshal(sample, &item.Latest); err != nil {
				return nil, err
			}
		}
		if err := json.Unmarshal(artworks, &item.Artworks); err != nil {
			return nil, err
		}
		if answers != nil {
			if err := json.Unmarshal(answers, &item.Answers); err != nil {
				return nil, err
			}
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

// Review 保存作品的人工结论，重复提交同一结论时返回成功。
func (s *PelicanTestStore) Review(ctx context.Context, id int64, status string) error {
	if status != "passed" && status != "suspected" {
		return routing.ErrPelicanInvalid
	}
	return pelicanMutationState(s.db.QueryRowContext(ctx, `
		WITH target AS (
			SELECT r.id, r.status, `+pelicanHasDrawing+` AS has_drawing
			FROM pelican_test_results r JOIN pelican_test_plans p ON p.id=r.plan_id
			JOIN groups g ON g.id=p.group_id WHERE r.id=$1 AND g.deleted_at IS NULL
			FOR UPDATE OF r
		), reviewed AS (
			UPDATE pelican_test_results r SET status=$2, detail_code=''
			FROM target WHERE r.id=target.id AND target.has_drawing AND target.status IN ('pending_review', $2)
			RETURNING r.id
		)
		SELECT EXISTS(SELECT 1 FROM target), EXISTS(SELECT 1 FROM reviewed)`, id, status))
}

func scanPelicanPlan(row interface{ Scan(...any) error }) (*routing.PelicanPlan, error) {
	var plan routing.PelicanPlan
	var roundAt *time.Time
	err := row.Scan(&plan.ID, &plan.GroupID, &plan.GroupName, &plan.ProviderID, &plan.ProviderName,
		&plan.ModelID, &plan.QuestionKind, &plan.QuestionName, &plan.QuestionPrompt, &plan.ExpectedAnswer,
		&plan.ReasoningEffort, &plan.CronExpression, &plan.Enabled, &plan.LastRunAt,
		&plan.NextRunAt, &plan.RunState, &roundAt, &plan.Revision, &plan.SuspendOnWrongAnswer)
	if err != nil {
		return nil, err
	}
	if roundAt != nil {
		plan.RoundAt = *roundAt
	}
	return &plan, nil
}

func pelicanResultDest(r *routing.PelicanResult) []any {
	return []any{
		&r.PlanID, &r.GroupID, &r.ProviderID, &r.ProviderName, &r.RoundID, &r.RecheckRunID, &r.ID, &r.ModelID, &r.QuestionKind,
		&r.QuestionName, &r.ReasoningEffort, &r.Status, &r.DetailCode, &r.TestedAt, &r.LatencyMs, &r.ScheduleAction,
	}
}

// pelicanMutationState 区分资源已删除和同一行上的状态冲突。
func pelicanMutationState(row *sql.Row) error {
	var found, updated bool
	if err := row.Scan(&found, &updated); err != nil {
		return pelicanWriteError(err)
	}
	if !found {
		return routing.ErrPelicanNotFound
	}
	if !updated {
		return routing.ErrPelicanConflict
	}
	return nil
}

func checkPelicanAffected(result sql.Result, err, noRows error) error {
	if err != nil {
		return pelicanWriteError(err)
	}
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return noRows
	}
	return err
}

// pelicanWriteError 将写入时的输入和约束错误转换成可公开的原因。
func pelicanWriteError(err error) error {
	pg, ok := errors.AsType[*pq.Error](err)
	if !ok {
		return err
	}
	if pg.Code == "23505" {
		return routing.ErrPelicanConflict
	}
	if pg.Code.Class() == "22" || pg.Code.Class() == "23" {
		return routing.ErrPelicanInvalid
	}
	return err
}
