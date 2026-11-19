package routing

import (
	"context"
	"slices"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/provider"
)

const (
	// 忙碌的账号继续服务用户请求。轮次等待期间释放 worker。
	pelicanCandyBusyWait = 5 * time.Minute
	pelicanCandyRetry    = 30 * time.Second

	PelicanScheduleDisabled        = "disabled"
	PelicanScheduleAlreadyDisabled = "already_disabled"
	PelicanScheduleAccountChanged  = "account_changed"
)

// PelicanCandyRound 保存在服务重启和 worker 交接后仍要遵循的候选和进度。
type PelicanCandyRound struct {
	ID           string
	CandidateIDs []int64
	TriedIDs     []int64
	BusySince    *time.Time
}

// PelicanCandyRepository 按计划的领取凭证记录尝试和账号调度变更。
type PelicanCandyRepository interface {
	OpenCandyRound(context.Context, PelicanPlan, []int64) (PelicanCandyRound, error)
	BeginCandyAttempt(context.Context, PelicanPlan, string, PelicanTarget) (bool, error)
	FinishCandyAttempt(context.Context, PelicanPlan, string, PelicanResult, *time.Time) (string, error)
	DeferCandyRound(context.Context, PelicanPlan, string, time.Time) error
}

func (p PelicanPlan) standardAutoCandy() bool {
	return p.QuestionKind == "candy" && p.ProviderID == nil && provider.IsStandardPelicanCandy(p.QuestionPrompt, p.ExpectedAnswer)
}

// runCandyAttempt 每次领取测一个账号，保存后释放租约供任意节点继续本轮。
// @project-doc docs/operations/provider_maintenance.md#pelican_group_tests
func (s *PelicanService) runCandyAttempt(ctx context.Context, plan PelicanPlan) error {
	if !plan.standardAutoCandy() || s.options.Candy == nil || s.options.CandyCandidates == nil {
		return ErrPelicanInvalid
	}
	readCtx, stop := context.WithTimeout(ctx, 15*time.Second)
	live, err := s.options.CandyCandidates(readCtx, plan)
	if err != nil {
		stop()
		return err
	}
	round, err := s.options.Candy.OpenCandyRound(readCtx, plan, live)
	stop()
	if err != nil {
		return err
	}
	excluded := make(map[int64]struct{}, len(round.TriedIDs))
	for _, id := range round.TriedIDs {
		excluded[id] = struct{}{}
	}
	candidates := make(map[int64]struct{}, len(round.CandidateIDs))
	for _, id := range round.CandidateIDs {
		candidates[id] = struct{}{}
	}
	plan.CandidateIDs = make([]int64, 0, len(live))
	for _, id := range live {
		_, tried := excluded[id]
		if _, inRound := candidates[id]; inRound && !tried {
			plan.CandidateIDs = append(plan.CandidateIDs, id)
		}
	}
	started := s.options.Now()
	if len(plan.CandidateIDs) == 0 {
		return s.finishCandyEvidence(ctx, plan, round.ID, provider.PelicanEvidence{Status: "inconclusive", DetailCode: "candy_exhausted"}, nil, started)
	}

	runCtx, cancel := context.WithTimeout(ctx, provider.PelicanQuestionTimeout("candy"))
	defer cancel()
	target, release, err := s.options.Acquire(runCtx, plan)
	defer func() {
		if release != nil {
			release()
		}
	}()
	if err == nil && release != nil && slices.Contains(plan.CandidateIDs, target.ProviderID) {
		registered, err := s.options.Candy.BeginCandyAttempt(runCtx, plan, round.ID, target)
		if err != nil {
			return err
		}
		if registered {
			evidence := s.options.Run(runCtx, target.ProviderID, target.ModelID, plan.QuestionKind, plan.RoundAt.UTC().Format(time.RFC3339Nano), target.Protocol, provider.PelicanQuestionConfig{
				Prompt: plan.QuestionPrompt, ExpectedAnswer: plan.ExpectedAnswer, ReasoningEffort: plan.ReasoningEffort,
			})
			if runCtx.Err() != nil {
				evidence = provider.PelicanEvidence{Status: "inconclusive", DetailCode: "incomplete"}
			}
			release()
			release = nil
			cancel()
			return s.finishCandyEvidence(ctx, plan, round.ID, evidence, &target.ProviderID, started)
		}
	}
	if round.BusySince != nil && !started.Before(round.BusySince.Add(pelicanCandyBusyWait)) {
		return s.finishCandyEvidence(ctx, plan, round.ID, provider.PelicanEvidence{Status: "inconclusive", DetailCode: "candy_busy"}, nil, started)
	}
	saveCtx, done := context.WithTimeout(ctx, 15*time.Second)
	defer done()
	return s.options.Candy.DeferCandyRound(saveCtx, plan, round.ID, started.Add(pelicanCandyRetry))
}

func (s *PelicanService) finishCandyEvidence(ctx context.Context, plan PelicanPlan, roundID string, evidence provider.PelicanEvidence, id *int64, started time.Time) error {
	finished := s.options.Now()
	next, err := pelicanNextRound(plan, finished)
	if err != nil {
		return err
	}
	result := pelicanResult(plan, evidence, id, started, finished)
	saveCtx, done := context.WithTimeout(ctx, 15*time.Second)
	defer done()
	action, err := s.options.Candy.FinishCandyAttempt(saveCtx, plan, roundID, result, next)
	if err == nil && id != nil && action == PelicanScheduleDisabled && s.options.CandyChanged != nil {
		s.options.CandyChanged(ctx, *id)
	}
	return err
}
