//go:build integration

package postgres

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/routing"
	"github.com/TokenFlux/TokenRouter/internal/testutil/postgrescontainer"
	"github.com/TokenFlux/TokenRouter/migrations"
)

// TestPelicanShowcaseGroupLimit 覆盖同组多个计划收录、修改上限和管理员历史留存。
func TestPelicanShowcaseGroupLimit(t *testing.T) {
	db := postgrescontainer.New(t)
	store := NewPelicanTestStore(db)
	ctx := t.Context()
	settings, err := store.ShowcaseSettings(ctx)
	require.NoError(t, err)
	require.Equal(t, routing.PelicanShowcaseSettings{MaxItems: 20, AutoCleanup: true, RetentionDays: 7}, settings)
	settings.MaxItems = 100
	saved, err := store.SaveShowcaseSettings(ctx, settings)
	require.NoError(t, err)
	require.Equal(t, settings, saved)
	invalid := settings
	invalid.MaxItems = 101
	_, err = store.SaveShowcaseSettings(ctx, invalid)
	require.ErrorIs(t, err, routing.ErrPelicanInvalid)
	currentSettings, err := store.ShowcaseSettings(ctx)
	require.NoError(t, err)
	require.Equal(t, saved, currentSettings)

	groupID := newPelicanGroup(t, db, "capacity")
	otherGroup := newPelicanGroup(t, db, "separate-capacity")
	first := newShowcasePlan(t, store, groupID, "first", "pelican")
	second := newShowcasePlan(t, store, groupID, "second", "pelican")
	other := newShowcasePlan(t, store, otherGroup, "other", "pelican")
	started := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	drawing := routing.PelicanResult{
		PelicanSummary: routing.PelicanSummary{Status: "pending_review", TestedAt: started},
		Answer:         `<svg xmlns="http://www.w3.org/2000/svg"><rect width="4" height="4"/></svg>`,
	}
	for i := range 101 {
		drawing.TestedAt = started.Add(time.Duration(i) * time.Second)
		claimed := claimShowcasePlan(t, store, first)
		require.NoError(t, store.Finish(ctx, claimed, drawing, nil))
	}
	rows, err := store.ListResults(ctx, first.ID)
	require.NoError(t, err)
	require.Len(t, rows, 30)
	art := findShowcasePlan(t, store, first).Artworks
	require.Len(t, art, 100)
	oldID := art[99].ID
	_, err = store.Result(ctx, oldID, []int64{groupID})
	require.NoError(t, err)
	require.NoError(t, store.Review(ctx, oldID, "passed"))
	require.Equal(t, "passed", findShowcasePlan(t, store, first).Artworks[99].Status)
	require.NoError(t, store.Finish(ctx, claimShowcasePlan(t, store, other), drawing, nil))

	// 两个计划同时发布并收紧设置后，整组只留下测试时间较晚的作品。
	claimA := claimShowcasePlan(t, store, first)
	claimB := claimShowcasePlan(t, store, second)
	b := drawing
	b.TestedAt = started.Add(2 * time.Minute)
	settings.MaxItems = 1
	ops := []func() error{
		func() error { return store.Finish(ctx, claimA, drawing, nil) },
		func() error { return store.Finish(ctx, claimB, b, nil) },
		func() error {
			_, err := store.SaveShowcaseSettings(ctx, settings)
			return err
		},
	}
	start := make(chan struct{})
	failures := make(chan error, len(ops))
	var workers sync.WaitGroup
	for _, op := range ops {
		workers.Go(func() {
			<-start
			failures <- op()
		})
	}
	close(start)
	workers.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	require.Empty(t, findShowcasePlan(t, store, first).Artworks)
	require.Nil(t, findShowcasePlan(t, store, first).Latest)
	require.Len(t, findShowcasePlan(t, store, second).Artworks, 1)
	require.Len(t, findShowcasePlan(t, store, other).Artworks, 1)
	_, err = store.Result(ctx, oldID, nil)
	require.ErrorIs(t, err, routing.ErrPelicanNotFound)
	rows, err = store.ListResults(ctx, first.ID)
	require.NoError(t, err)
	require.Len(t, rows, 30)
	evicted := rows[0].ID
	_, err = store.Result(ctx, evicted, nil)
	require.NoError(t, err)
	require.NoError(t, store.Review(ctx, evicted, "passed"))
	_, err = store.Result(ctx, evicted, []int64{groupID})
	require.ErrorIs(t, err, routing.ErrPelicanNotFound)
	settings.MaxItems = 100
	_, err = store.SaveShowcaseSettings(ctx, settings)
	require.NoError(t, err)
	require.Empty(t, findShowcasePlan(t, store, first).Artworks)
}

// TestPelicanShowcaseExpiry 检查设置保存、后台清理以及清理前的用户读取。
func TestPelicanShowcaseExpiry(t *testing.T) {
	db := postgrescontainer.New(t)
	store := NewPelicanTestStore(db)
	ctx := t.Context()
	groupID := newPelicanGroup(t, db, "expiry")
	plan := newShowcasePlan(t, store, groupID, "drawing", "pelican")
	settings := routing.PelicanShowcaseSettings{Enabled: true, APIEnabled: true, MaxItems: 5, RetentionDays: 1}
	_, err := store.SaveShowcaseSettings(ctx, settings)
	require.NoError(t, err)
	now := time.Now().UTC().Truncate(time.Second)
	drawing := routing.PelicanResult{PelicanSummary: routing.PelicanSummary{
		Status: "pending_review", TestedAt: now.Add(-48 * time.Hour),
	}, Answer: `<svg><circle r="4"/></svg>`}
	require.NoError(t, store.Finish(ctx, claimShowcasePlan(t, store, plan), drawing, nil))
	old := findShowcasePlan(t, store, plan).Latest
	require.NotNil(t, old)
	settings.AutoCleanup = true
	_, err = store.SaveShowcaseSettings(ctx, settings)
	require.NoError(t, err)
	require.Nil(t, findShowcasePlan(t, store, plan).Latest)
	require.Empty(t, findShowcasePlan(t, store, plan).Artworks)
	_, err = store.Result(ctx, old.ID, []int64{groupID})
	require.ErrorIs(t, err, routing.ErrPelicanNotFound)
	_, err = store.Result(ctx, old.ID, nil)
	require.NoError(t, err)

	drawing.TestedAt = now
	require.NoError(t, store.Finish(ctx, claimShowcasePlan(t, store, plan), drawing, nil))
	before := findShowcasePlan(t, store, plan).Latest
	require.NotNil(t, before)
	// 把已收录的作品推进到期时刻和后台任务之间的窗口。
	_, err = db.ExecContext(ctx, "UPDATE pelican_test_results SET tested_at=$1 WHERE id=$2", now.Add(-48*time.Hour), before.ID)
	require.NoError(t, err)
	isPublished := func() bool {
		t.Helper()
		var published bool
		require.NoError(t, db.QueryRowContext(ctx, "SELECT published FROM pelican_test_results WHERE id=$1", before.ID).Scan(&published))
		return published
	}
	require.True(t, isPublished())
	require.Empty(t, findShowcasePlan(t, store, plan).Artworks)
	require.Nil(t, findShowcasePlan(t, store, plan).Latest)
	_, err = store.Result(ctx, before.ID, []int64{groupID})
	require.ErrorIs(t, err, routing.ErrPelicanNotFound)
	require.NoError(t, store.PruneShowcase(ctx))
	require.False(t, isPublished())
	failed := routing.PelicanResult{PelicanSummary: routing.PelicanSummary{
		Status: "inconclusive", TestedAt: now.Add(-36 * time.Hour),
	}}
	require.NoError(t, store.Finish(ctx, claimShowcasePlan(t, store, plan), failed, nil))
	require.Equal(t, "inconclusive", findShowcasePlan(t, store, plan).Latest.Status)
	textPlan := newShowcasePlan(t, store, groupID, "text", "candy")
	failed.Status = "passed"
	failed.TestedAt = now
	failed.Answer = "21"
	require.NoError(t, store.Finish(ctx, claimShowcasePlan(t, store, textPlan), failed, nil))
	text := findShowcasePlan(t, store, textPlan)
	require.Empty(t, text.Artworks)
	require.NotNil(t, text.Latest)
	detail, err := store.Result(ctx, text.Latest.ID, []int64{groupID})
	require.NoError(t, err)
	require.Equal(t, "21", detail.Answer)
}

// TestPelicanShowcaseCandyLimit 让同组糖果计划共享上限，并验证旧详情退出用户展示。
func TestPelicanShowcaseCandyLimit(t *testing.T) {
	db := postgrescontainer.New(t)
	store := NewPelicanTestStore(db)
	ctx := t.Context()
	firstGroup := newPelicanGroup(t, db, "candy-limits")
	otherGroup := newPelicanGroup(t, db, "separate-candy-limits")
	settings := routing.PelicanShowcaseSettings{Enabled: true, APIEnabled: true, MaxItems: 100, RetentionDays: 7}
	_, err := store.SaveShowcaseSettings(ctx, settings)
	require.NoError(t, err)
	first := newShowcasePlan(t, store, firstGroup, "first", "candy")
	second := newShowcasePlan(t, store, firstGroup, "second", "candy")
	drawing := newShowcasePlan(t, store, firstGroup, "drawing", "pelican")
	other := newShowcasePlan(t, store, otherGroup, "other", "candy")
	sample := routing.PelicanResult{
		PelicanSummary: routing.PelicanSummary{Status: "passed", TestedAt: time.Now().UTC()},
		Answer:         "21",
	}
	for range 31 {
		require.NoError(t, store.Finish(ctx, claimShowcasePlan(t, store, first), sample, nil))
	}
	firstAnswers := findShowcasePlan(t, store, first).Answers
	require.Len(t, firstAnswers, 31)
	require.NoError(t, store.Finish(ctx, claimShowcasePlan(t, store, second), sample, nil))
	require.NoError(t, store.Finish(ctx, claimShowcasePlan(t, store, other), sample, nil))
	sample.Status, sample.Answer = "pending_review", "<svg><circle r='4'/></svg>"
	require.NoError(t, store.Finish(ctx, claimShowcasePlan(t, store, drawing), sample, nil))

	settings.MaxItems = 1
	_, err = store.SaveShowcaseSettings(ctx, settings)
	require.NoError(t, err)
	require.Empty(t, findShowcasePlan(t, store, first).Answers)
	require.Nil(t, findShowcasePlan(t, store, first).Latest)
	require.Len(t, findShowcasePlan(t, store, second).Answers, 1)
	require.Len(t, findShowcasePlan(t, store, other).Answers, 1)
	require.Len(t, findShowcasePlan(t, store, drawing).Artworks, 1)
	_, err = store.Result(ctx, firstAnswers[0].ID, []int64{firstGroup})
	require.ErrorIs(t, err, routing.ErrPelicanNotFound)
	_, err = store.Result(ctx, firstAnswers[0].ID, nil)
	require.NoError(t, err)
}

// TestPelicanShowcaseCandyBackfill 验证升级时同组历史候选只收录最新的一条。
func TestPelicanShowcaseCandyBackfill(t *testing.T) {
	db := postgrescontainer.New(t)
	store := NewPelicanTestStore(db)
	ctx := t.Context()
	group := newPelicanGroup(t, db, "candy-backfill")
	plan := newShowcasePlan(t, store, group, "candy", "candy")
	settings := routing.PelicanShowcaseSettings{Enabled: true, MaxItems: 1, AutoCleanup: true, RetentionDays: 7}
	_, err := store.SaveShowcaseSettings(ctx, settings)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		INSERT INTO pelican_test_results
			(plan_id, model_id, question_kind, status, prompt, criteria, answer, tested_at, latency_ms, plan_revision)
		VALUES ($1,'candy','candy','passed','','[]','21',NOW()-interval '8 days',0,1),
		       ($1,'candy','candy','suspected','','[]','20',NOW()-interval '5 minutes',0,1),
		       ($1,'candy','candy','passed','','[]','21',NOW()-interval '1 minute',0,1),
		       ($1,'candy','candy','suspected','','[]','22',NOW(),0,0)`, plan.ID)
	require.NoError(t, err)
	require.Empty(t, findShowcasePlan(t, store, plan).Answers)
	backfill, err := migrations.FS.ReadFile("304_pelican_publish_candy_results.sql")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(backfill))
	require.NoError(t, err)
	candy := findShowcasePlan(t, store, plan).Answers
	require.Len(t, candy, 1)
	require.Equal(t, "21", candy[0].AnswerPreview)
	require.Equal(t, "passed", candy[0].Status)
}

// TestPelicanShowcaseCanvasForWeb 确保完整画布可以在网页复核，作品接口发布独立 SVG。
func TestPelicanShowcaseCanvasForWeb(t *testing.T) {
	db := postgrescontainer.New(t)
	store := NewPelicanTestStore(db)
	ctx := t.Context()
	group := newPelicanGroup(t, db, "html-canvas")
	plan := newShowcasePlan(t, store, group, "canvas", "pelican")
	now := time.Now()
	sample := routing.PelicanResult{
		PelicanSummary: routing.PelicanSummary{Status: "pending_review", TestedAt: now},
		RawAnswer:      "<html><body><canvas></canvas></body></html>",
	}
	require.NoError(t, store.Finish(ctx, claimShowcasePlan(t, store, plan), sample, nil))
	web := findShowcasePlan(t, store, plan).Artworks
	require.Len(t, web, 1)
	detail, err := store.Result(ctx, web[0].ID, []int64{group})
	require.NoError(t, err)
	require.Equal(t, sample.RawAnswer, detail.RawAnswer)
	require.NoError(t, store.Review(ctx, detail.ID, "passed"))
	api, err := store.LatestDrawings(ctx, []int64{group})
	require.NoError(t, err)
	require.Len(t, api, 1)
	require.Empty(t, api[0].Artworks)
}

func newShowcasePlan(t *testing.T, store *PelicanTestStore, groupID int64, model, kind string) routing.PelicanPlan {
	t.Helper()
	plan, err := store.SavePlan(t.Context(), routing.PelicanPlan{
		GroupID: groupID, ModelID: model, QuestionKind: kind, CronExpression: "30 9 * * *",
	})
	require.NoError(t, err)
	return *plan
}

func claimShowcasePlan(t *testing.T, store *PelicanTestStore, plan routing.PelicanPlan) routing.PelicanPlan {
	t.Helper()
	now := time.Now()
	require.NoError(t, store.QueuePlan(t.Context(), plan.ID))
	claimed, err := store.ClaimNext(t.Context(), now, now.Add(time.Minute), plan.ModelID)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	require.Equal(t, plan.ID, claimed.ID)
	return *claimed
}

func findShowcasePlan(t *testing.T, store *PelicanTestStore, plan routing.PelicanPlan) routing.PelicanLatest {
	t.Helper()
	list, err := store.Latest(t.Context(), []int64{plan.GroupID})
	require.NoError(t, err)
	for _, item := range list {
		if item.ModelID == plan.ModelID {
			return item
		}
	}
	t.Fatalf("分组 %d 缺少模型 %s 的计划", plan.GroupID, plan.ModelID)
	return routing.PelicanLatest{}
}
