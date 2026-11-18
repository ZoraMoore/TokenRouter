//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/routing"
	"github.com/TokenFlux/TokenRouter/internal/testutil/postgrescontainer"
)

// TestPelicanTestStore 使用完整迁移后的数据库检查抢占、读权限和历史留存。
func TestPelicanTestStore(t *testing.T) {
	db := postgrescontainer.New(t)
	t.Run("leases", func(t *testing.T) { testPelicanLeases(t, db) })
	t.Run("expired_config", func(t *testing.T) { testPelicanExpiredConfig(t, db) })
	t.Run("visibility", func(t *testing.T) { testPelicanVisibility(t, db) })
	t.Run("retention", func(t *testing.T) { testPelicanRetention(t, db) })
	t.Run("custom_questions", func(t *testing.T) { testPelicanCustomQuestions(t, db) })
	t.Run("candy_answers", func(t *testing.T) { testPelicanCandyAnswers(t, db) })
	t.Run("state_probe", func(t *testing.T) { testPelicanStateProbe(t, db) })
}

// TestPelicanRawAnswerStore 覆盖完整原文的保存、组内读取、历史兼容和数据库大小限制。
func TestPelicanRawAnswerStore(t *testing.T) {
	db := postgrescontainer.New(t)
	store := NewPelicanTestStore(db)
	ctx := t.Context()
	groupID := newPelicanGroup(t, db, "raw-artwork")
	plan, err := store.SavePlan(ctx, routing.PelicanPlan{
		GroupID: groupID, ModelID: "drawing", QuestionKind: "pelican", CronExpression: "0 * * * *",
	})
	require.NoError(t, err)
	raw := " \n<!doctype html><style>.wheel{stroke:red}</style><svg><circle r=\"4\"/></svg><script>window.frame=1</script>\n"
	for _, tc := range []struct{ raw, answer, status string }{
		{raw, `<svg><circle r="4"/></svg>`, "pending_review"},
		{raw, "", "inconclusive"},
		{"", `<svg><circle r="4"/></svg>`, "pending_review"},
	} {
		require.NoError(t, store.QueuePlan(ctx, plan.ID))
		now := time.Now().UTC()
		claimed, err := store.ClaimNext(ctx, now, now.Add(15*time.Minute), "raw-test")
		require.NoError(t, err)
		require.NotNil(t, claimed)
		require.NoError(t, store.Finish(ctx, *claimed, routing.PelicanResult{
			PelicanSummary: routing.PelicanSummary{Status: tc.status, TestedAt: now}, Answer: tc.answer, RawAnswer: tc.raw,
		}, nil))
		history, err := store.ListResults(ctx, plan.ID)
		require.NoError(t, err)
		require.NotEmpty(t, history)
		require.Empty(t, history[0].RawAnswer)
		for _, scope := range [][]int64{nil, {groupID}} {
			sample, err := store.Result(ctx, history[0].ID, scope)
			require.NoError(t, err)
			require.Equal(t, tc.raw, sample.RawAnswer)
			require.Equal(t, tc.answer, sample.Answer)
		}
		_, err = store.Result(ctx, history[0].ID, []int64{-1})
		require.ErrorIs(t, err, routing.ErrPelicanNotFound)
		_, err = db.ExecContext(ctx, `UPDATE pelican_test_results SET raw_answer=$1 WHERE id=$2`, strings.Repeat("x", (2<<20)+1), history[0].ID)
		require.Error(t, err)
	}
}

func testPelicanCandyAnswers(t *testing.T, db *sql.DB) {
	ctx := t.Context()
	store := NewPelicanTestStore(db)
	group := newPelicanGroup(t, db, "candy-answers")
	other := newPelicanGroup(t, db, "candy-other")
	makePlan := func(groupID int64, kind string) *routing.PelicanPlan {
		t.Helper()
		plan, err := store.SavePlan(ctx, routing.PelicanPlan{
			GroupID: groupID, ModelID: "model-a", QuestionKind: kind, QuestionName: "分糖果",
			QuestionPrompt: "分完剩几颗？", ExpectedAnswer: "21", ReasoningEffort: "high", CronExpression: "0 * * * *",
		})
		require.NoError(t, err)
		return plan
	}
	plan := makePlan(group, "candy")
	now := time.Now().UTC().Truncate(time.Microsecond)
	add := func(p *routing.PelicanPlan, status, answer string, target *int64) int64 {
		t.Helper()
		var id int64
		require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO pelican_test_results
			(plan_id, model_id, question_kind, question_name, plan_revision, target_provider_id, status,
				detail_code, prompt, criteria, answer, tested_at, latency_ms, reasoning_effort)
			SELECT id, model_id, question_kind, question_name, plan_revision, $2, $3,
				CASE WHEN $3='suspected' THEN 'answer_mismatch' ELSE '' END, question_prompt, '[]', $4, $5, 37, 'medium'
			FROM pelican_test_plans WHERE id=$1 RETURNING id`, p.ID, target, status, answer, now).Scan(&id))
		return id
	}
	type sample struct {
		routing.PelicanSummary
		AnswerPreview string `json:"answer_preview"`
	}
	answers := func(p *routing.PelicanPlan) []sample {
		t.Helper()
		rows, err := store.Latest(ctx, []int64{p.GroupID})
		require.NoError(t, err)
		var body []byte
		for _, row := range rows {
			require.Equal(t, p.GroupID, row.GroupID)
			if row.QuestionKind == p.QuestionKind && row.ReasoningEffort == p.ReasoningEffort {
				body, err = json.Marshal(row)
				require.NoError(t, err)
			}
		}
		require.NotEmpty(t, body)
		var card struct {
			Answers []sample `json:"answers"`
		}
		require.NoError(t, json.Unmarshal(body, &card))
		return card.Answers
	}
	// 放入超过三十次的原始历史，其中一部分早于作品展示期限。
	_, err := db.ExecContext(ctx, `INSERT INTO pelican_test_results
		(plan_id, model_id, question_kind, question_name, status, prompt, criteria, answer, tested_at, latency_ms)
		SELECT $1, 'model-a','candy','分糖果','passed','','[]', n::text, $2::timestamptz + n * interval '1 minute', 1
		FROM generate_series(1,33) AS series(n) ORDER BY series.n DESC`, plan.ID, now.Add(-40*24*time.Hour))
	require.NoError(t, err)
	long := strings.Repeat("🍬", 256)
	passed := add(plan, "passed", long+" 最后有 21 颗。", nil)
	add(plan, "passed", strings.Repeat("糖", 256), nil)
	wrong := add(plan, "suspected", "最后还剩 20 颗。", nil)
	add(plan, "inconclusive", "未作答完", nil)
	add(plan, "pending_review", "未判定", nil)
	add(plan, "passed", "", nil)
	add(plan, "suspected", " \t\n", nil)
	oldTarget := int64(991)
	add(plan, "passed", "旧目标", &oldTarget)
	hidden := add(makePlan(other, "candy"), "passed", "其他组的答案", nil)
	custom := makePlan(group, "custom_text")
	add(custom, "passed", "自定义文本题的答案", nil)
	sibling := *plan
	sibling.ID, sibling.ReasoningEffort = 0, "low"
	otherPlan, err := store.SavePlan(ctx, sibling)
	require.NoError(t, err)
	add(otherPlan, "passed", "同组另一强度的回答", nil)

	got := answers(plan)
	require.Len(t, got, 25)
	require.Equal(t, wrong, got[0].ID)
	require.Equal(t, "最后还剩 20 颗。", got[0].AnswerPreview)
	require.Equal(t, "suspected", got[0].Status)
	require.Equal(t, "answer_mismatch", got[0].DetailCode)
	require.Equal(t, "分糖果", got[0].QuestionName)
	require.Equal(t, "medium", got[0].ReasoningEffort)
	require.Equal(t, int64(37), got[0].LatencyMs)
	require.True(t, now.Equal(got[0].TestedAt))
	require.Equal(t, strings.Repeat("糖", 256), got[1].AnswerPreview)
	require.Equal(t, passed, got[2].ID)
	require.Equal(t, long+"…", got[2].AnswerPreview)
	require.Equal(t, "33", got[3].AnswerPreview)
	require.Equal(t, "12", got[len(got)-1].AnswerPreview)
	require.Empty(t, answers(custom))
	otherAnswers := answers(otherPlan)
	require.Len(t, otherAnswers, 1)
	require.Equal(t, "同组另一强度的回答", otherAnswers[0].AnswerPreview)
	drawings, err := store.LatestDrawings(ctx, []int64{group})
	require.NoError(t, err)
	require.Empty(t, drawings)
	detail, err := store.Result(ctx, passed, []int64{group})
	require.NoError(t, err)
	require.Equal(t, long+" 最后有 21 颗。", detail.Answer)
	_, err = store.Result(ctx, hidden, []int64{group})
	require.ErrorIs(t, err, routing.ErrPelicanNotFound)
	plan.QuestionPrompt = "分完还剩多少？"
	_, err = store.SavePlan(ctx, *plan)
	require.NoError(t, err)
	require.Empty(t, answers(plan))
	_, err = store.Result(ctx, passed, []int64{group})
	require.ErrorIs(t, err, routing.ErrPelicanNotFound)
}

func testPelicanLeases(t *testing.T, db *sql.DB) {
	ctx := t.Context()
	store := NewPelicanTestStore(db)
	groupID := newPelicanGroup(t, db, "pelican-leases")
	now := time.Now().UTC().Truncate(time.Second)
	due := now.Add(-time.Minute)
	until := now.Add(time.Minute)
	input := routing.PelicanPlan{
		GroupID: groupID, ModelID: "model-a", QuestionKind: "candy",
		CronExpression: "* * * * *", Enabled: true, NextRunAt: &due,
	}
	plan, err := store.SavePlan(ctx, input)
	require.NoError(t, err)
	require.Equal(t, "pelican-leases", plan.GroupName)
	_, err = store.SavePlan(ctx, input)
	require.ErrorIs(t, err, routing.ErrPelicanConflict)
	moved := *plan
	moved.GroupID = newPelicanGroup(t, db, "pelican-other")
	_, err = store.SavePlan(ctx, moved)
	require.ErrorIs(t, err, routing.ErrPelicanInvalid)

	// 被其他事务锁住的计划应让当前 worker 继续轮询。
	locked, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = locked.Rollback() })
	_, err = locked.ExecContext(ctx, "SELECT id FROM pelican_test_plans WHERE id=$1 FOR UPDATE", plan.ID)
	require.NoError(t, err)
	waitCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	skipped, err := store.ClaimNext(waitCtx, now, until, "busy")
	cancel()
	require.NoError(t, err)
	require.Nil(t, skipped)
	require.NoError(t, locked.Rollback())

	var wg sync.WaitGroup
	start := make(chan struct{})
	claims := make([]*routing.PelicanPlan, 2)
	claimErrors := make([]error, 2)
	for i, token := range []string{"one", "two"} {
		wg.Go(func() {
			<-start
			claims[i], claimErrors[i] = store.ClaimNext(ctx, now, until, token)
		})
	}
	close(start)
	wg.Wait()
	var winner *routing.PelicanPlan
	for i, claim := range claims {
		require.NoError(t, claimErrors[i])
		if claim != nil {
			require.Nil(t, winner)
			winner = claim
		}
	}
	require.NotNil(t, winner)
	require.True(t, due.Equal(winner.RoundAt))
	require.Equal(t, "running", winner.RunState)
	require.ErrorIs(t, store.QueuePlan(ctx, plan.ID), routing.ErrPelicanConflict)
	_, err = store.SavePlan(ctx, *plan)
	require.ErrorIs(t, err, routing.ErrPelicanConflict)

	reclaimed, err := store.ClaimNext(ctx, until, until.Add(time.Minute), "three")
	require.NoError(t, err)
	require.NotNil(t, reclaimed)
	require.Equal(t, "three", reclaimed.LeaseToken)
	require.True(t, due.Equal(reclaimed.RoundAt))
	result := routing.PelicanResult{PelicanSummary: routing.PelicanSummary{
		Status: "inconclusive", TestedAt: until, LatencyMs: 18,
	}}
	next := until.Add(time.Hour)
	require.ErrorIs(t, store.Finish(ctx, *winner, result, &next), routing.ErrPelicanConflict)
	history, err := store.ListResults(ctx, plan.ID)
	require.NoError(t, err)
	require.Empty(t, history)
	missingProvider := int64(-1)
	result.ProviderID = &missingProvider
	require.ErrorIs(t, store.Finish(ctx, *reclaimed, result, &next), routing.ErrPelicanInvalid)
	result.ProviderID = nil
	require.NoError(t, store.Finish(ctx, *reclaimed, result, &next))
	require.ErrorIs(t, store.Finish(ctx, *reclaimed, result, &next), routing.ErrPelicanConflict)
	plans, err := store.ListPlans(ctx)
	require.NoError(t, err)
	require.Len(t, plans, 1)
	require.Equal(t, "idle", plans[0].RunState)
	require.True(t, until.Equal(*plans[0].LastRunAt))
	require.True(t, next.Equal(*plans[0].NextRunAt))
	require.Empty(t, plans[0].LeaseToken)

	// 领取前还不知道题型。绘图可以占用完整租期，文本题仍按五分钟回收。
	for _, tt := range []struct {
		kind  string
		lease time.Duration
	}{{"candy", 5 * time.Minute}, {"pelican", 15 * time.Minute}} {
		plan.QuestionKind = tt.kind
		plan, err = store.SavePlan(ctx, *plan)
		require.NoError(t, err)
		require.NoError(t, store.QueuePlan(ctx, plan.ID))
		claimed, err := store.ClaimNext(ctx, now, now.Add(15*time.Minute), "lease-by-kind")
		require.NoError(t, err)
		require.NotNil(t, claimed)
		var expires time.Time
		require.NoError(t, db.QueryRowContext(ctx, "SELECT lease_until FROM pelican_test_plans WHERE id=$1", plan.ID).Scan(&expires))
		require.Equal(t, tt.lease, expires.Sub(now))
		require.NoError(t, store.Finish(ctx, *claimed, result, &next))
	}
}

func testPelicanExpiredConfig(t *testing.T, db *sql.DB) {
	ctx := t.Context()
	store := NewPelicanTestStore(db)
	groupID := newPelicanGroup(t, db, "pelican-expired")
	due := time.Now().UTC().Truncate(time.Second).Add(-2 * time.Hour)
	until := due.Add(time.Minute)
	plan, err := store.SavePlan(ctx, routing.PelicanPlan{
		GroupID: groupID, ModelID: "model-a", QuestionKind: "candy",
		CronExpression: "* * * * *", Enabled: true, NextRunAt: &due,
	})
	require.NoError(t, err)
	old, err := store.ClaimNext(ctx, due, until, "before-edit")
	require.NoError(t, err)
	require.NotNil(t, old)
	plan.ModelID = "model-b"
	_, err = store.SavePlan(ctx, *plan)
	require.NoError(t, err)
	stale := routing.PelicanResult{PelicanSummary: routing.PelicanSummary{Status: "passed", TestedAt: until}}
	require.ErrorIs(t, store.Finish(ctx, *old, stale, nil), routing.ErrPelicanConflict)
	latest, err := store.Latest(ctx, []int64{groupID})
	require.NoError(t, err)
	require.Len(t, latest, 1)
	require.Equal(t, "model-b", latest[0].ModelID)
	require.Nil(t, latest[0].Latest)

	// 手动入队也会替换已过期的定时任务领取凭证。
	expired, err := store.ClaimNext(ctx, due, until, "before-queue")
	require.NoError(t, err)
	require.NotNil(t, expired)
	require.NoError(t, store.QueuePlan(ctx, plan.ID))
	require.ErrorIs(t, store.Finish(ctx, *expired, stale, nil), routing.ErrPelicanConflict)
	history, err := store.ListResults(ctx, plan.ID)
	require.NoError(t, err)
	require.Empty(t, history)
}

func testPelicanVisibility(t *testing.T, db *sql.DB) {
	ctx := t.Context()
	store := NewPelicanTestStore(db)
	groupID := newPelicanGroup(t, db, "pelican-visibility")
	var providerID int64
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO providers(name, platform, type)
		VALUES ('pelican-account', 'openai', 'apikey') RETURNING id`).Scan(&providerID))
	plan, err := store.SavePlan(ctx, routing.PelicanPlan{
		GroupID: groupID, ModelID: "model-a", QuestionKind: "pelican", CronExpression: "0 * * * *",
	})
	require.NoError(t, err)
	allowed := []int64{groupID}
	readLatest := func() routing.PelicanLatest {
		t.Helper()
		latest, err := store.Latest(ctx, allowed)
		require.NoError(t, err)
		require.Len(t, latest, 1)
		return latest[0]
	}
	require.Nil(t, readLatest().Latest)
	ready, err := store.ClaimNext(ctx, time.Now(), time.Now().Add(time.Minute), "scheduled")
	require.NoError(t, err)
	require.Nil(t, ready)
	require.NoError(t, store.QueuePlan(ctx, plan.ID))
	require.ErrorIs(t, store.QueuePlan(ctx, plan.ID), routing.ErrPelicanConflict)
	_, err = store.SavePlan(ctx, *plan)
	require.ErrorIs(t, err, routing.ErrPelicanConflict)
	require.Equal(t, "queued", readLatest().RunState)
	now := time.Now().UTC().Truncate(time.Microsecond)
	claimed, err := store.ClaimNext(ctx, now, now.Add(time.Minute), "manual")
	require.NoError(t, err)
	require.NotNil(t, claimed)
	require.False(t, claimed.RoundAt.IsZero())
	result := routing.PelicanResult{
		PelicanSummary: routing.PelicanSummary{Status: "pending_review", TestedAt: now, LatencyMs: 25},
		ProviderID:     &providerID, Prompt: "绘制鹈鹕", Criteria: []string{"有一只鸟"}, Answer: `<svg xmlns="http://www.w3.org/2000/svg"><circle r="1"/></svg>`,
	}
	require.NoError(t, store.Finish(ctx, *claimed, result, nil))
	history, err := store.ListResults(ctx, plan.ID)
	require.NoError(t, err)
	require.Len(t, history, 1)
	id := history[0].ID
	require.Equal(t, "pelican-account", history[0].ProviderName)
	require.Empty(t, history[0].Prompt)
	require.Empty(t, history[0].Criteria)
	require.Empty(t, history[0].Answer)
	detail, err := store.Result(ctx, id, allowed)
	require.NoError(t, err)
	require.Equal(t, result.Answer, detail.Answer)
	require.Equal(t, result.Criteria, detail.Criteria)
	_, err = store.Result(ctx, id, []int64{groupID + 1000})
	require.ErrorIs(t, err, routing.ErrPelicanNotFound)
	require.NoError(t, store.Review(ctx, id, "passed"))
	require.NoError(t, store.Review(ctx, id, "passed"))
	require.ErrorIs(t, store.Review(ctx, id, "suspected"), routing.ErrPelicanConflict)
	require.ErrorIs(t, store.Review(ctx, -1, "passed"), routing.ErrPelicanNotFound)
	require.Equal(t, "passed", readLatest().Latest.Status)

	// 切换选号方式后，计划等待下一次作答，已发布作品仍归原分组。
	plan.ProviderID = &providerID
	plan, err = store.SavePlan(ctx, *plan)
	require.NoError(t, err)
	require.Nil(t, readLatest().Latest)
	require.Len(t, readLatest().Artworks, 1)
	detail, err = store.Result(ctx, id, allowed)
	require.NoError(t, err)
	require.Equal(t, result.Answer, detail.Answer)
	require.Nil(t, detail.ProviderID)
	require.Empty(t, detail.ProviderName)
	_, err = store.Result(ctx, id, nil)
	require.NoError(t, err)
	require.NoError(t, store.QueuePlan(ctx, plan.ID))
	later := now.Add(time.Minute)
	claimed, err = store.ClaimNext(ctx, later, later.Add(time.Minute), "pinned")
	require.NoError(t, err)
	require.NotNil(t, claimed)
	var wrongProviderID int64
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO providers(name, platform, type)
		VALUES ('pelican-another-account', 'openai', 'apikey') RETURNING id`).Scan(&wrongProviderID))
	mismatched := result
	mismatched.ProviderID = &wrongProviderID
	require.ErrorIs(t, store.Finish(ctx, *claimed, mismatched, nil), routing.ErrPelicanInvalid)
	noSample := routing.PelicanResult{PelicanSummary: routing.PelicanSummary{Status: "inconclusive", TestedAt: later}}
	require.NoError(t, store.Finish(ctx, *claimed, noSample, nil))
	noSampleID := readLatest().Latest.ID
	require.NotEqual(t, id, noSampleID)
	require.ErrorIs(t, store.Review(ctx, noSampleID, "passed"), routing.ErrPelicanConflict)
	plan.ProviderID = nil
	plan, err = store.SavePlan(ctx, *plan)
	require.NoError(t, err)
	require.Equal(t, id, readLatest().Latest.ID)
	require.Len(t, readLatest().Artworks, 1)
	require.Equal(t, id, readLatest().Artworks[0].ID)
	_, err = store.Result(ctx, noSampleID, allowed)
	require.ErrorIs(t, err, routing.ErrPelicanNotFound)
	plan.ModelID = "model-b"
	_, err = store.SavePlan(ctx, *plan)
	require.NoError(t, err)
	require.Nil(t, readLatest().Latest)
	detail, err = store.Result(ctx, id, allowed)
	require.NoError(t, err)
	require.Equal(t, "model-a", detail.ModelID)
	require.NoError(t, store.QueuePlan(ctx, plan.ID))
	_, err = db.ExecContext(ctx, "UPDATE groups SET status='inactive' WHERE id=$1", groupID)
	require.NoError(t, err)
	require.ErrorIs(t, store.QueuePlan(ctx, plan.ID), routing.ErrPelicanNotFound)
	ready, err = store.ClaimNext(ctx, later, later.Add(time.Minute), "inactive")
	require.NoError(t, err)
	require.Nil(t, ready)
	latest, err := store.Latest(ctx, allowed)
	require.NoError(t, err)
	require.Empty(t, latest)
	_, err = store.Result(ctx, id, allowed)
	require.ErrorIs(t, err, routing.ErrPelicanNotFound)
	_, err = db.ExecContext(ctx, "UPDATE groups SET status='active', deleted_at=NOW() WHERE id=$1", groupID)
	require.NoError(t, err)
	plans, err := store.ListPlans(ctx)
	require.NoError(t, err)
	require.Empty(t, plans)
	ready, err = store.ClaimNext(ctx, later, later.Add(time.Minute), "deleted")
	require.NoError(t, err)
	require.Nil(t, ready)
}

func testPelicanRetention(t *testing.T, db *sql.DB) {
	ctx := t.Context()
	store := NewPelicanTestStore(db)
	groupID := newPelicanGroup(t, db, "pelican-retention")
	plan, err := store.SavePlan(ctx, routing.PelicanPlan{
		GroupID: groupID, ModelID: "model-a", QuestionKind: "candy", CronExpression: "0 * * * *",
	})
	require.NoError(t, err)
	now := time.Now().UTC().Truncate(time.Microsecond)
	_, err = db.ExecContext(ctx, `INSERT INTO pelican_test_results
		(plan_id, model_id, question_kind, status, detail_code, prompt, criteria, answer, tested_at, latency_ms)
		SELECT $1,'model-a','candy','passed','','', '[]'::jsonb, '',$2,0 FROM generate_series(1,30)`, plan.ID, now)
	require.NoError(t, err)
	// 旧样本使用默认版本，管理员调整时间后仍可从用户页查看。
	plan.CronExpression = "30 * * * *"
	_, err = store.SavePlan(ctx, *plan)
	require.NoError(t, err)
	latest, err := store.Latest(ctx, []int64{groupID})
	require.NoError(t, err)
	require.Len(t, latest, 1)
	require.NotNil(t, latest[0].Latest)
	require.Empty(t, latest[0].Latest.QuestionName)
	require.Empty(t, latest[0].Latest.ReasoningEffort)
	require.Empty(t, latest[0].Artworks)
	var oldest int64
	require.NoError(t, db.QueryRowContext(ctx, "SELECT MIN(id) FROM pelican_test_results WHERE plan_id=$1", plan.ID).Scan(&oldest))
	require.ErrorIs(t, store.Review(ctx, oldest, "passed"), routing.ErrPelicanConflict)
	require.NoError(t, store.QueuePlan(ctx, plan.ID))
	claimed, err := store.ClaimNext(ctx, now, now.Add(time.Minute), "retention")
	require.NoError(t, err)
	require.NotNil(t, claimed)
	result := routing.PelicanResult{PelicanSummary: routing.PelicanSummary{Status: "passed", TestedAt: now}}
	require.NoError(t, store.Finish(ctx, *claimed, result, nil))
	history, err := store.ListResults(ctx, plan.ID)
	require.NoError(t, err)
	require.Len(t, history, 30)
	_, err = store.Result(ctx, oldest, nil)
	require.ErrorIs(t, err, routing.ErrPelicanNotFound)
	var retained int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM pelican_test_results WHERE plan_id=$1", plan.ID).Scan(&retained))
	require.Equal(t, 30, retained)
	require.NoError(t, store.DeletePlan(ctx, plan.ID))
	require.ErrorIs(t, store.DeletePlan(ctx, plan.ID), routing.ErrPelicanNotFound)
	_, err = store.Result(ctx, history[0].ID, nil)
	require.ErrorIs(t, err, routing.ErrPelicanNotFound)
}

func testPelicanCustomQuestions(t *testing.T, db *sql.DB) {
	ctx := t.Context()
	store := NewPelicanTestStore(db)
	groupID := newPelicanGroup(t, db, "pelican-custom")
	input := routing.PelicanPlan{
		GroupID: groupID, ModelID: "model-a", QuestionKind: "custom_svg", QuestionName: "飞鸟",
		QuestionPrompt: "画一只鸟", ReasoningEffort: "high", CronExpression: "0 * * * *",
	}
	plan, err := store.SavePlan(ctx, input)
	require.NoError(t, err)
	require.Equal(t, input.QuestionName, plan.QuestionName)
	require.Equal(t, input.QuestionPrompt, plan.QuestionPrompt)
	require.Equal(t, input.ReasoningEffort, plan.ReasoningEffort)
	_, err = store.SavePlan(ctx, input)
	require.ErrorIs(t, err, routing.ErrPelicanConflict)
	// 同一个模型可以用多个推理强度并行作答，列表保留各自的计划。
	otherEffort := input
	otherEffort.ReasoningEffort = "low"
	alternate, err := store.SavePlan(ctx, otherEffort)
	require.NoError(t, err)
	rows, err := store.Latest(ctx, []int64{groupID})
	require.NoError(t, err)
	var efforts []string
	for _, row := range rows {
		efforts = append(efforts, row.ReasoningEffort)
	}
	require.ElementsMatch(t, []string{"high", "low"}, efforts)
	require.NoError(t, store.DeletePlan(ctx, alternate.ID))
	input.QuestionName = "流水"
	second, err := store.SavePlan(ctx, input)
	require.NoError(t, err)
	require.NoError(t, store.DeletePlan(ctx, second.ID))
	current := func() routing.PelicanLatest {
		t.Helper()
		latest, err := store.Latest(ctx, []int64{groupID})
		require.NoError(t, err)
		require.Len(t, latest, 1)
		return latest[0]
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	addResult := func(r routing.PelicanResult) {
		t.Helper()
		require.NoError(t, store.QueuePlan(ctx, plan.ID))
		claimed, err := store.ClaimNext(ctx, now, now.Add(time.Minute), "custom-run")
		require.NoError(t, err)
		require.NotNil(t, claimed)
		require.Equal(t, plan.QuestionPrompt, claimed.QuestionPrompt)
		require.Equal(t, plan.ExpectedAnswer, claimed.ExpectedAnswer)
		require.NoError(t, store.Finish(ctx, *claimed, r, nil))
	}
	art := routing.PelicanResult{
		PelicanSummary: routing.PelicanSummary{Status: "pending_review", TestedAt: now, ReasoningEffort: "medium"},
		Answer:         `<svg xmlns="http://www.w3.org/2000/svg"><circle r="1"/></svg>`,
	}
	// 相同的测试时间按结果 ID 排序，最新一次失败仍可看到分组内已收录的作品。
	for i := range 12 {
		art.TestedAt = now.Add(-time.Duration(i%2) * time.Hour)
		addResult(art)
	}
	addResult(routing.PelicanResult{PelicanSummary: routing.PelicanSummary{Status: "inconclusive", TestedAt: now}})
	history, err := store.ListResults(ctx, plan.ID)
	require.NoError(t, err)
	require.Len(t, history, 13)
	require.Equal(t, "medium", history[1].ReasoningEffort)
	require.Equal(t, plan.QuestionName, history[1].QuestionName)
	latest := current()
	require.Equal(t, plan.QuestionName, latest.QuestionName)
	require.Equal(t, history[0].ID, latest.Latest.ID)
	require.Empty(t, latest.Latest.ReasoningEffort)
	require.Len(t, latest.Artworks, 12)
	for i, sample := range latest.Artworks {
		expected := history[i+1].PelicanSummary
		expected.TestedAt = expected.TestedAt.UTC()
		sample.TestedAt = sample.TestedAt.UTC()
		require.Equal(t, expected, sample)
	}
	id := latest.Artworks[0].ID
	require.NoError(t, store.Review(ctx, id, "passed"))
	plan.CronExpression = "30 * * * *"
	plan, err = store.SavePlan(ctx, *plan)
	require.NoError(t, err)
	require.Equal(t, "passed", current().Artworks[0].Status)
	originalPrompt := plan.QuestionPrompt
	for _, prompt := range []string{"画两只鸟", originalPrompt} {
		plan.QuestionPrompt = prompt
		plan, err = store.SavePlan(ctx, *plan)
		require.NoError(t, err)
		entry := current()
		require.Nil(t, entry.Latest)
		require.Len(t, entry.Artworks, 12)
		require.Equal(t, id, entry.Artworks[0].ID)
		detail, err := store.Result(ctx, id, []int64{groupID})
		require.NoError(t, err)
		require.Equal(t, art.Answer, detail.Answer)
		_, err = store.Result(ctx, history[0].ID, []int64{groupID})
		require.ErrorIs(t, err, routing.ErrPelicanNotFound)
	}
	historical, err := store.Result(ctx, id, nil)
	require.NoError(t, err)
	require.Equal(t, "medium", historical.ReasoningEffort)
	require.Equal(t, "飞鸟", historical.QuestionName)

	plan.QuestionKind = "custom_text"
	plan.QuestionName = "算术"
	plan.QuestionPrompt = "一加一等于几"
	plan.ExpectedAnswer = "2"
	plan, err = store.SavePlan(ctx, *plan)
	require.NoError(t, err)
	require.Equal(t, "2", plan.ExpectedAnswer)
	textAnswer := routing.PelicanResult{
		PelicanSummary: routing.PelicanSummary{Status: "passed", TestedAt: now, ReasoningEffort: "low"},
		Answer:         "2",
	}
	addResult(textAnswer)
	textResult := current()
	require.Equal(t, "low", textResult.Latest.ReasoningEffort)
	require.Len(t, textResult.Artworks, 12)
	require.Equal(t, "飞鸟", textResult.Artworks[0].QuestionName)
	require.Equal(t, "custom_svg", textResult.Artworks[0].QuestionKind)
	drawings, err := store.LatestDrawings(ctx, []int64{groupID})
	require.NoError(t, err)
	require.Len(t, drawings, 1)
	require.Equal(t, textResult.Artworks, drawings[0].Artworks)
	require.ErrorIs(t, store.Review(ctx, textResult.Latest.ID, "passed"), routing.ErrPelicanConflict)
	plan.ExpectedAnswer = "二"
	plan, err = store.SavePlan(ctx, *plan)
	require.NoError(t, err)
	require.Nil(t, current().Latest)
	addResult(textAnswer)
	for _, name := range []string{"心算", "算术"} {
		plan.QuestionName = name
		plan, err = store.SavePlan(ctx, *plan)
		require.NoError(t, err)
		require.Nil(t, current().Latest)
	}
	addResult(textAnswer)
	plan.ReasoningEffort = "medium"
	_, err = store.SavePlan(ctx, *plan)
	require.NoError(t, err)
	require.Nil(t, current().Latest)

	plan.QuestionKind = "candy"
	plan, err = store.SavePlan(ctx, *plan)
	require.NoError(t, err)
	addResult(textAnswer)
	require.Len(t, current().Answers, 1)
	drawings, err = store.LatestDrawings(ctx, []int64{groupID})
	require.NoError(t, err)
	require.Len(t, drawings, 1)
	require.Len(t, drawings[0].Artworks, 12)
	require.Empty(t, drawings[0].Answers)
}

// testPelicanStateProbe 核对管理历史、分组状态、作品 API 和更换指定账号后的读权限。
func testPelicanStateProbe(t *testing.T, db *sql.DB) {
	ctx := t.Context()
	store := NewPelicanTestStore(db)
	group := newPelicanGroup(t, db, "probe-group")
	other := newPelicanGroup(t, db, "probe-other")
	var account, replacement int64
	for _, id := range []*int64{&account, &replacement} {
		require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO providers(name, platform, type)
			VALUES ('private-probe-account','openai','oauth') RETURNING id`).Scan(id))
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	plan, err := store.SavePlan(ctx, routing.PelicanPlan{
		GroupID: group, ProviderID: &account, ModelID: "public-model", QuestionKind: routing.PelicanStateProbe,
		Enabled: true, NextRunAt: &now, CronExpression: "0 * * * *",
	})
	require.NoError(t, err)
	claimed, err := store.ClaimNext(ctx, now, now.Add(time.Minute), "probe-worker")
	require.NoError(t, err)
	require.NotNil(t, claimed)
	require.Equal(t, plan.ID, claimed.ID)
	next := now.Add(time.Hour)
	result := routing.PelicanResult{PelicanSummary: routing.PelicanSummary{Status: "suspected", DetailCode: "state_changed", TestedAt: now, LatencyMs: 52}, ProviderID: &account, Answer: "两次会话状态不同。"}
	require.NoError(t, store.Finish(ctx, *claimed, result, &next))

	history, err := store.ListResults(ctx, plan.ID)
	require.NoError(t, err)
	require.Len(t, history, 1)
	id := history[0].ID
	require.Equal(t, "private-probe-account", history[0].ProviderName)
	var published bool
	require.NoError(t, db.QueryRowContext(ctx, "SELECT published FROM pelican_test_results WHERE id=$1", id).Scan(&published))
	require.False(t, published)
	rows, err := store.Latest(ctx, []int64{group})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "state_changed", rows[0].Latest.DetailCode)
	require.Empty(t, rows[0].Artworks)
	require.Empty(t, rows[0].Answers)
	gallery, err := store.LatestDrawings(ctx, []int64{group})
	require.NoError(t, err)
	require.Empty(t, gallery)
	require.ErrorIs(t, store.Review(ctx, id, "passed"), routing.ErrPelicanConflict)
	detail, err := store.Result(ctx, id, []int64{group})
	require.NoError(t, err)
	require.Equal(t, result.Answer, detail.Answer)
	require.Nil(t, detail.ProviderID)
	require.Empty(t, detail.ProviderName)
	_, err = store.Result(ctx, id, []int64{other})
	require.ErrorIs(t, err, routing.ErrPelicanNotFound)

	plan.ProviderID = &replacement
	_, err = store.SavePlan(ctx, *plan)
	require.NoError(t, err)
	rows, err = store.Latest(ctx, []int64{group})
	require.NoError(t, err)
	require.Nil(t, rows[0].Latest)
	_, err = store.Result(ctx, id, []int64{group})
	require.ErrorIs(t, err, routing.ErrPelicanNotFound)
	admin, err := store.Result(ctx, id, nil)
	require.NoError(t, err)
	require.Equal(t, account, *admin.ProviderID)
}
