package routing

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type pelicanPlanFixture struct {
	PelicanRepository
	saved *PelicanPlan
}

func (f *pelicanPlanFixture) SavePlan(_ context.Context, plan PelicanPlan) (*PelicanPlan, error) {
	f.saved = &plan
	return &plan, nil
}

// TestPelicanSaveSchedule 检查暂停计划和服务器时区里的下一次运行时间。
func TestPelicanSaveSchedule(t *testing.T) {
	now := time.Date(2026, 1, 3, 1, 5, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	repo := &pelicanPlanFixture{}
	s := NewPelicanService(repo, PelicanOptions{
		Now:      func() time.Time { return now },
		Validate: func(context.Context, PelicanPlan) error { return nil },
	})
	plan := PelicanPlan{GroupID: 3, ModelID: " example/text ", QuestionKind: "pelican", CronExpression: "0 9 * * *", Enabled: true}
	got, err := s.Save(t.Context(), plan)
	require.NoError(t, err)
	require.Equal(t, "example/text", got.ModelID)
	require.Equal(t, now.Add(7*time.Hour+55*time.Minute), *got.NextRunAt)
	plan.Enabled = false
	got, err = s.Save(t.Context(), plan)
	require.NoError(t, err)
	require.Nil(t, got.NextRunAt)

	for _, change := range []func(*PelicanPlan){
		func(p *PelicanPlan) { p.ModelID = "other\nmodel" },
		func(p *PelicanPlan) { p.ModelID = strings.Repeat("a", 129) },
		func(p *PelicanPlan) { p.QuestionKind = "unknown" },
		func(p *PelicanPlan) { p.CronExpression = "0 9 30 2 *" },
		func(p *PelicanPlan) { p.CronExpression = "@every 1s" },
		func(p *PelicanPlan) { p.GroupID = 0 },
		func(p *PelicanPlan) { id := int64(-1); p.ProviderID = &id },
	} {
		invalid := plan
		change(&invalid)
		repo.saved = nil
		_, err := s.Save(t.Context(), invalid)
		require.ErrorIs(t, err, ErrPelicanInvalid)
		require.Nil(t, repo.saved)
	}
}

// TestPelicanUserWithoutGroups 检查空权限用户无法通过猜测作品 ID 读取账号测试。
func TestPelicanUserWithoutGroups(t *testing.T) {
	repo := &pelicanPlanFixture{}
	s := NewPelicanService(repo, PelicanOptions{UserGroups: func(context.Context, int64) ([]Group, error) {
		return nil, nil
	}})
	_, err := s.UserResult(t.Context(), 12, 34)
	require.ErrorIs(t, err, ErrPelicanNotFound)
	groups, err := s.Showcase(t.Context(), 12)
	require.NoError(t, err)
	require.NotNil(t, groups)
	require.Empty(t, groups)
}

func (*pelicanPlanFixture) ShowcaseSettings(context.Context) (PelicanShowcaseSettings, error) {
	return PelicanShowcaseSettings{Enabled: true, MaxItems: 20, RetentionDays: 7}, nil
}

// TestPelicanCustomQuestion 校验管理员配置的题面、标准答案和请求强度。
func TestPelicanCustomQuestion(t *testing.T) {
	base := PelicanPlan{QuestionKind: "custom_text", QuestionName: " 算术 ", QuestionPrompt: "一加一？", ExpectedAnswer: " 2 ", ReasoningEffort: "high"}
	require.True(t, base.validQuestion())
	require.Equal(t, "算术", base.QuestionName)
	require.Equal(t, "2", base.ExpectedAnswer)
	for _, change := range []func(*PelicanPlan){
		func(p *PelicanPlan) { p.ExpectedAnswer = "" },
		func(p *PelicanPlan) { p.QuestionKind = "custom_svg" },
		func(p *PelicanPlan) { p.QuestionKind = "pelican" },
		func(p *PelicanPlan) { p.ReasoningEffort = "maximum" },
		func(p *PelicanPlan) { p.QuestionName = strings.Repeat("题", 33) },
		func(p *PelicanPlan) { p.QuestionPrompt = "a\x00b" },
		func(p *PelicanPlan) { p.ExpectedAnswer = strings.Repeat("x", 1025) },
	} {
		plan := base
		change(&plan)
		require.False(t, plan.validQuestion())
	}
	base.QuestionKind = "custom_svg"
	base.QuestionPrompt = "绘制鹈鹕\n添加自行车"
	base.ExpectedAnswer = ""
	require.True(t, base.validQuestion())
}

// TestPelicanBuiltinEdits 内置题可按计划单独改名或改题面，旧计划允许继续使用题库。
func TestPelicanBuiltinEdits(t *testing.T) {
	for _, plan := range []PelicanPlan{
		{QuestionKind: "pelican"},
		{QuestionKind: "pelican", QuestionName: "新题名"},
		{QuestionKind: "pelican", QuestionName: "海岸", QuestionPrompt: "画一只海边的鹈鹕"},
		{QuestionKind: "candy", QuestionPrompt: "三颗糖吃一颗，还剩多少？", ExpectedAnswer: "2"},
		{QuestionKind: "candy", QuestionName: "糖果推理"},
	} {
		require.True(t, plan.validQuestion(), plan)
	}
	invalid := PelicanPlan{QuestionKind: "pelican", ExpectedAnswer: "21"}
	require.False(t, invalid.validQuestion())
}

// TestPelicanStateProbePlan 续接测试需要管理员选定账号，题面和推理档位由探针确定。
func TestPelicanStateProbePlan(t *testing.T) {
	id := int64(12)
	p := PelicanPlan{ProviderID: &id, QuestionKind: "state_probe", QuestionName: "会话续接"}
	require.True(t, p.validQuestion())
	for _, change := range []func(*PelicanPlan){
		func(p *PelicanPlan) { p.ProviderID = nil },
		func(p *PelicanPlan) { zero := int64(0); p.ProviderID = &zero },
		func(p *PelicanPlan) { p.ReasoningEffort = "high" },
		func(p *PelicanPlan) { p.ExpectedAnswer = "21" },
		func(p *PelicanPlan) { p.QuestionPrompt = "换一段题面" },
	} {
		invalid := p
		change(&invalid)
		require.False(t, invalid.validQuestion())
	}
}
