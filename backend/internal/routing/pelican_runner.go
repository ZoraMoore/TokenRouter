package routing

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/TokenFlux/TokenRouter/internal/provider"
)

const (
	// 每个实例最多同时占用三个提供商槽位。
	pelicanWorkers = 3

	// PelicanDefaultLease 为文本题和状态探针预留写入结果的时间。
	PelicanDefaultLease = 5 * time.Minute
	pelicanDrawingLease = 15 * time.Minute
)

// PelicanRunner 从 PostgreSQL 领取手动请求和到期计划，进程退出时等待在途请求释放槽位。
type PelicanRunner struct {
	service *PelicanService
	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	stopped bool
}

func NewPelicanRunner(service *PelicanService) *PelicanRunner {
	return &PelicanRunner{service: service}
}

func (r *PelicanRunner) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.stopped || r.done != nil {
		return nil
	}
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	r.cancel = cancel
	r.done = make(chan struct{})
	go func() {
		defer close(r.done)
		r.loop(runCtx)
	}()
	return nil
}

func (r *PelicanRunner) Stop(ctx context.Context) error {
	r.mu.Lock()
	r.stopped = true
	done := r.done
	if r.cancel != nil {
		r.cancel()
	}
	r.mu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// loop 仅在有空闲 worker 时领取任务，使租期覆盖实际执行时间。
func (r *PelicanRunner) loop(ctx context.Context) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	var workers sync.WaitGroup
	defer workers.Wait()
	workers.Go(func() { r.pruneShowcase(ctx) })
	finished := make(chan struct{}, pelicanWorkers)
	active := 0
	for ctx.Err() == nil {
		for active < pelicanWorkers && ctx.Err() == nil {
			now := r.service.options.Now()
			readCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			plan, err := r.service.repo.ClaimNext(readCtx, now, now.Add(pelicanDrawingLease), uuid.NewString())
			cancel()
			if err != nil {
				if ctx.Err() == nil {
					slog.Warn("领取鹈鹕测试计划失败")
				}
				break
			}
			if plan == nil {
				break
			}
			active++
			workers.Go(func() {
				defer func() { finished <- struct{}{} }()
				if err := r.service.finishRound(ctx, *plan); err != nil && ctx.Err() == nil {
					slog.Warn("保存鹈鹕测试结果失败", "plan_id", plan.ID)
				}
			})
		}
		select {
		case <-ctx.Done():
			return
		case <-r.service.wake:
		case <-tick.C:
		case <-finished:
			active--
		}
	}
}

// pruneShowcase 在启动时和之后每小时清理作品，队列空闲时也会执行。
func (r *PelicanRunner) pruneShowcase(ctx context.Context) {
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for ctx.Err() == nil {
		pruneCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := r.service.repo.PruneShowcase(pruneCtx)
		cancel()
		if err != nil && ctx.Err() == nil {
			slog.Warn("清理鹈鹕展示作品失败")
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// finishRound 用计划预定时刻出题，同一时刻到期的账号拿到相同题目。
func (s *PelicanService) finishRound(ctx context.Context, plan PelicanPlan) error {
	started := s.options.Now()
	runCtx, cancel := context.WithTimeout(ctx, provider.PelicanQuestionTimeout(plan.QuestionKind))
	evidence, providerID := s.runRound(runCtx, plan)
	if runCtx.Err() != nil && plan.QuestionKind != PelicanStateProbe {
		evidence.Status, evidence.DetailCode, evidence.Answer = "inconclusive", "incomplete", ""
	}
	cancel()
	if err := ctx.Err(); err != nil {
		// 进程关闭中断的任务在租约到期后重新领取。
		return err
	}
	finished := s.options.Now()
	var next *time.Time
	if plan.Enabled {
		due, err := nextPelicanRun(plan.CronExpression, finished)
		if err != nil {
			return err
		}
		next = &due
	}
	if evidence.Criteria == nil {
		evidence.Criteria = []string{}
	}
	result := PelicanResult{
		PelicanSummary: PelicanSummary{QuestionName: plan.QuestionName, ReasoningEffort: evidence.ReasoningEffort, ModelID: plan.ModelID, QuestionKind: plan.QuestionKind, Status: evidence.Status, DetailCode: evidence.DetailCode, TestedAt: finished, LatencyMs: max(0, finished.Sub(started).Milliseconds())},
		PlanID:         plan.ID, GroupID: plan.GroupID, ProviderID: providerID,
		Prompt: evidence.Prompt, Criteria: evidence.Criteria, Answer: evidence.Answer,
	}
	saveCtx, stop := context.WithTimeout(ctx, 15*time.Second)
	defer stop()
	return s.repo.Finish(saveCtx, plan, result, next)
}

// runRound 持有选定账号的并发槽位，直到该轮的请求全部结束。
func (s *PelicanService) runRound(ctx context.Context, plan PelicanPlan) (provider.PelicanEvidence, *int64) {
	missing := provider.PelicanEvidence{Status: "inconclusive", DetailCode: "no_provider"}
	if plan.QuestionKind == PelicanStateProbe && (plan.ProviderID == nil || *plan.ProviderID <= 0) {
		return missing, nil
	}
	target, release, err := s.options.Acquire(ctx, plan)
	if err != nil || release == nil {
		if plan.QuestionKind == PelicanStateProbe && ctx.Err() != nil {
			return pelicanProbeEvidence(provider.StateProbeResult{}, ctx.Err()), nil
		}
		return missing, nil
	}
	defer release()
	if plan.QuestionKind == PelicanStateProbe {
		if target.ProviderID != *plan.ProviderID || s.options.RunStateProbe == nil {
			return missing, nil
		}
		sample, err := s.options.RunStateProbe(ctx, target.ProviderID, target.ModelID)
		if sample.AccountID != 0 && sample.AccountID != target.ProviderID {
			return missing, nil
		}
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return pelicanProbeEvidence(sample, err), &target.ProviderID
	}
	evidence := s.options.Run(ctx, target.ProviderID, target.ModelID, plan.QuestionKind, plan.RoundAt.UTC().Format(time.RFC3339Nano), target.Protocol, provider.PelicanQuestionConfig{
		Prompt: plan.QuestionPrompt, ExpectedAnswer: plan.ExpectedAnswer, ReasoningEffort: plan.ReasoningEffort,
	})
	return evidence, &target.ProviderID
}
