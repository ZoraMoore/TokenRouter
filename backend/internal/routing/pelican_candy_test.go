package routing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/provider"
)

type pelicanCandyRepoFixture struct {
	PelicanRepository
	PelicanCandyRepository
	tried    []int64
	finished bool
	results  []PelicanResult
}

// TestPelicanCandyRestriction 防止改题、改答案或固定账号后开启自动停调。
func TestPelicanCandyRestriction(t *testing.T) {
	account := int64(8)
	for _, tc := range []struct {
		name, kind, prompt, answer string
		account                    *int64
		valid                      bool
	}{
		{name: "标准题", kind: "candy", answer: "21", valid: true},
		{name: "自定义题面", kind: "candy", prompt: "一加一", answer: "21"},
		{name: "自定义答案", kind: "candy", answer: "20"},
		{name: "固定账号", kind: "candy", account: &account},
		{name: "绘图题", kind: "pelican"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := NewPelicanService(&pelicanCandyRepoFixture{}, PelicanOptions{
				Now: time.Now, Validate: func(context.Context, PelicanPlan) error { return nil },
			})
			_, err := service.Save(t.Context(), PelicanPlan{
				GroupID: 2, ModelID: "model", QuestionKind: tc.kind,
				QuestionPrompt: tc.prompt, ExpectedAnswer: tc.answer, ProviderID: tc.account,
				CronExpression: "0 * * * *", SuspendOnWrongAnswer: true,
			})
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrPelicanInvalid)
			}
		})
	}
}

func (f *pelicanCandyRepoFixture) SavePlan(_ context.Context, plan PelicanPlan) (*PelicanPlan, error) {
	return &plan, nil
}

func (f *pelicanCandyRepoFixture) OpenCandyRound(_ context.Context, _ PelicanPlan, candidates []int64) (PelicanCandyRound, error) {
	return PelicanCandyRound{ID: "round", CandidateIDs: candidates, TriedIDs: f.tried}, nil
}

func (f *pelicanCandyRepoFixture) BeginCandyAttempt(_ context.Context, _ PelicanPlan, _ string, target PelicanTarget) (bool, error) {
	f.tried = append(f.tried, target.ProviderID)
	return true, nil
}

func (f *pelicanCandyRepoFixture) FinishCandyAttempt(_ context.Context, _ PelicanPlan, _ string, result PelicanResult, _ *time.Time) (string, error) {
	f.results = append(f.results, result)
	if result.Status == "passed" {
		f.finished = true
	}
	if result.Status == "suspected" && result.DetailCode == "answer_mismatch" {
		return PelicanScheduleDisabled, nil
	}
	return "", nil
}

// TestPelicanCandyRotatesUntilPassed 覆盖换号、无法判断和收尾，领取时从库里取得已尝试账号。
func TestPelicanCandyRotatesUntilPassed(t *testing.T) {
	repo := &pelicanCandyRepoFixture{}
	var changes []int64
	released := 0
	service := NewPelicanService(repo, PelicanOptions{
		Now: time.Now, Candy: repo,
		CandyCandidates: func(context.Context, PelicanPlan) ([]int64, error) { return []int64{10, 11, 12, 13}, nil },
		Acquire: func(_ context.Context, plan PelicanPlan) (PelicanTarget, func(), error) {
			require.NotEmpty(t, plan.CandidateIDs)
			return PelicanTarget{ProviderID: plan.CandidateIDs[0]}, func() { released++ }, nil
		},
		Run: func(_ context.Context, id int64, _, _, _, _ string, _ provider.PelicanQuestionConfig) provider.PelicanEvidence {
			switch id {
			case 10:
				return provider.PelicanEvidence{Status: "suspected", DetailCode: "answer_mismatch", Answer: "20"}
			case 11:
				return provider.PelicanEvidence{Status: "inconclusive", DetailCode: "incomplete"}
			default:
				return provider.PelicanEvidence{Status: "passed", Answer: "21"}
			}
		},
		CandyChanged: func(_ context.Context, id int64) { changes = append(changes, id) },
	})
	plan := PelicanPlan{ID: 1, GroupID: 2, ModelID: "model", QuestionKind: "candy", SuspendOnWrongAnswer: true}
	for range 3 {
		require.NoError(t, service.finishRound(t.Context(), plan))
	}
	require.True(t, repo.finished)
	require.Equal(t, []int64{10, 11, 12}, repo.tried)
	require.Equal(t, []int64{10}, changes)
	require.Equal(t, 3, released)
	require.Equal(t, "21", repo.results[2].Answer)
}
