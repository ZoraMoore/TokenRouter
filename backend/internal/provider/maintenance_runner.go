package provider

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
	"github.com/TokenFlux/TokenRouter/internal/provider/operation"
)

const (
	maintenancePollInterval = 3 * time.Second
	maintenanceBusyWait     = 15 * time.Minute

	MaintenanceWorkTimeout = 4 * time.Minute
	MaintenanceWorkers     = 3
)

// MaintenanceGuard 确认测试开始时的账号与分组仍然有效。AuthVersion 是认证状态摘要。
type MaintenanceGuard struct {
	Revision     int64
	GroupVersion time.Time
	MembershipAt time.Time
	AuthVersion  string
}

// MaintenanceWork 包含一项处理所需的冻结计划、领取凭证和上一步证据。
type MaintenanceWork struct {
	ID        int64
	RunID     int64
	Plan      MaintenancePlan
	Trigger   string
	Item      MaintenanceItem
	StartedAt time.Time
	Lease     string
	Guard     MaintenanceGuard
}

// MaintenanceUpdate 在同一事务内更新阶段、证据及提供商状态。
type MaintenanceUpdate struct {
	MaintenanceResult
	Action            string
	Guard             MaintenanceGuard
	CredentialVersion string
	RetryAt           time.Time
	ReplacementID     int64
}

// MaintenanceExecutionStore 按领取凭证保存每个账号的处置。
type MaintenanceExecutionStore interface {
	Claim(context.Context) (MaintenanceWork, error)
	Guard(context.Context, int64, int64) (MaintenanceGuard, error)
	Finish(context.Context, MaintenanceWork, MaintenanceUpdate) error
}

// MaintenanceRunnerOptions 绑定定向探测、并发槽和共享重登进度。
type MaintenanceRunnerOptions struct {
	Now              func() time.Time
	Resolve          func(context.Context, MaintenanceDraft, MaintenanceTarget) (MaintenanceTarget, error)
	Load             func(context.Context, int64) (*Record, error)
	Acquire          func(context.Context, *Record) (func(), error)
	Observe          func(context.Context, int64, string) (int, bool, error)
	Probe            func(context.Context, int64, string) (StateProbeResult, error)
	Candy            func(context.Context, int64, string) PelicanEvidence
	ActiveOperation  func(context.Context, int64) (operation.Job, error)
	Operation        func(context.Context, string) (operation.Job, error)
	ReloginAvailable bool
}

// MaintenanceRunner 调度到期计划，并用有限数量的执行者领取账号明细。
type MaintenanceRunner struct {
	plans   *MaintenanceService
	store   MaintenanceExecutionStore
	options MaintenanceRunnerOptions
	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
}

func (g MaintenanceGuard) Matches(other MaintenanceGuard) bool {
	return g.Revision > 0 && g.Revision == other.Revision && g.AuthVersion == other.AuthVersion &&
		g.GroupVersion.Equal(other.GroupVersion) && g.MembershipAt.Equal(other.MembershipAt)
}

func NewMaintenanceRunner(plans *MaintenanceService, store MaintenanceExecutionStore, options MaintenanceRunnerOptions) *MaintenanceRunner {
	if options.Now == nil {
		options.Now = time.Now
	}
	return &MaintenanceRunner{plans: plans, store: store, options: options}
}

func (r *MaintenanceRunner) Start(context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done != nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.done = make(chan struct{})
	go func() {
		defer close(r.done)
		var workers sync.WaitGroup
		workers.Go(func() { r.queueDue(ctx) })
		for range MaintenanceWorkers {
			workers.Go(func() { r.runWorker(ctx) })
		}
		workers.Wait()
	}()
	return nil
}

// Stop 取消待领取和定向请求。已有重登任务从其检查点继续。
func (r *MaintenanceRunner) Stop(ctx context.Context) error {
	r.mu.Lock()
	cancel, done := r.cancel, r.done
	r.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *MaintenanceRunner) queueDue(ctx context.Context) {
	ticker := time.NewTicker(maintenancePollInterval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		queueCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := r.plans.QueueDue(queueCtx)
		cancel()
		if err != nil && ctx.Err() == nil {
			slog.Warn("登记到期的智能运维计划失败")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *MaintenanceRunner) runWorker(ctx context.Context) {
	ticker := time.NewTicker(maintenancePollInterval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		claimCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		work, err := r.store.Claim(claimCtx)
		cancel()
		if err == nil {
			r.execute(ctx, work)
			continue
		}
		if !errors.Is(err, ErrMaintenanceIdle) && ctx.Err() == nil {
			slog.Warn("领取智能运维任务失败")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *MaintenanceRunner) execute(ctx context.Context, work MaintenanceWork) {
	runCtx, cancel := context.WithTimeout(ctx, MaintenanceWorkTimeout)
	update, err := r.process(runCtx, work)
	cancel()
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		slog.Warn("执行智能运维任务失败", "run_id", work.RunID, "provider_id", work.Item.ProviderID, "stage", work.Item.Stage)
		update = maintenanceOutcome("inconclusive", "execution_error")
		if work.Item.Stage == "relogin" {
			update = r.retry("operation_wait", maintenancePollInterval)
		}
	}
	saveCtx, done := context.WithTimeout(ctx, 10*time.Second)
	defer done()
	if err := r.store.Finish(saveCtx, work, update); err != nil && !errors.Is(err, ErrMaintenanceChanged) && ctx.Err() == nil {
		slog.Warn("保存智能运维任务结果失败", "run_id", work.RunID, "provider_id", work.Item.ProviderID)
	}
}

func (r *MaintenanceRunner) process(ctx context.Context, work MaintenanceWork) (MaintenanceUpdate, error) {
	item := work.Item
	if item.Stage == "relogin" {
		return r.waitRelogin(ctx, item)
	}
	replacement := item.Stage == "verify"
	if replacement {
		item.ProviderID = item.ReplacementID
		item.GroupID = 0
	}
	guard, err := r.store.Guard(ctx, item.ProviderID, item.GroupID)
	if errors.Is(err, ErrMaintenanceNotFound) {
		return maintenanceOutcome("skipped", "changed"), nil
	}
	if err != nil {
		return MaintenanceUpdate{}, err
	}
	if item.Stage == "candy" && !work.Guard.Matches(guard) {
		return maintenanceOutcome("skipped", "changed"), nil
	}
	refreshCleared401 := false
	if !replacement {
		fresh, err := r.options.Resolve(ctx, work.Plan.MaintenanceDraft, item.MaintenanceTarget)
		if err != nil {
			return MaintenanceUpdate{}, err
		}
		refreshCleared401 = item.Stage == "auth" && fresh.Reason == "no_401"
		if fresh.Reason != "" && !refreshCleared401 {
			return maintenanceOutcome("skipped", fresh.Reason), nil
		}
	}
	record, err := r.options.Load(ctx, item.ProviderID)
	if apperror.IsNotFound(err) || err == nil && record == nil {
		return maintenanceOutcome("skipped", "changed"), nil
	}
	if err != nil {
		return MaintenanceUpdate{}, err
	}
	if !SupportsStateProbe(record) {
		return maintenanceOutcome("skipped", "unsupported"), nil
	}
	if !replacement {
		job, err := r.options.ActiveOperation(ctx, item.ProviderID)
		if err == nil {
			if work.Plan.Kind == MaintenanceRelogin && job.Kind == operation.Relogin {
				return MaintenanceUpdate{Action: "join_relogin", Guard: guard}, nil
			}
			return r.retryBusy(work, "operation_wait"), nil
		}
		if !errors.Is(err, operation.ErrNotFound) {
			return MaintenanceUpdate{}, err
		}
	}
	if item.Stage == "auth" && record.Status == StatusActive && HasMaintenance401Cooldown(record) &&
		record.TempUnschedulableUntil.After(r.options.Now()) {
		return MaintenanceUpdate{
			MaintenanceResult: MaintenanceResult{Detail: "refresh_wait"},
			RetryAt:           *record.TempUnschedulableUntil,
		}, nil
	}
	release, err := r.options.Acquire(ctx, record)
	if err != nil {
		return MaintenanceUpdate{}, err
	}
	if release == nil {
		return r.retryBusy(work, "busy"), nil
	}
	defer release()
	var update MaintenanceUpdate
	switch item.Stage {
	case "auth", "verify":
		status, complete, err := r.options.Observe(ctx, item.ProviderID, item.Model)
		update = maintenanceAuthResult(item.Stage, status, complete && err == nil, r.options.ReloginAvailable)
		if refreshCleared401 {
			switch status {
			case 200:
				update.Action = ""
			case 401:
				update.Outcome, update.Detail, update.Action = "inconclusive", "upstream_401", "record_401"
			}
		}
		if update.Detail == "auth_unconfirmed" || update.Detail == "replacement_unconfirmed" {
			update.Detail = maintenanceAuthFailure(status, err)
		}
	case "probe":
		sample, err := r.options.Probe(ctx, item.ProviderID, item.Model)
		switch {
		case errors.Is(err, ErrIQBusy):
			return r.retryBusy(work, "busy"), nil
		case err != nil:
			return maintenanceOutcome("inconclusive", "probe_failed"), nil
		case sample.AccountID != item.ProviderID || sample.Model != "" && sample.Model != item.ActualModel:
			return maintenanceOutcome("skipped", "changed"), nil
		}
		update = maintenanceProbeResult(sample)
	case "candy":
		evidence := r.options.Candy(ctx, item.ProviderID, item.Model)
		update = maintenanceCandyResult(evidence, evidence.HTTPStatus)
		if update.Action == "suspend" {
			// 白名单可以在作答期间变更，停调前用当前模型规则再核对一次。
			fresh, err := r.options.Resolve(ctx, work.Plan.MaintenanceDraft, item.MaintenanceTarget)
			if err != nil {
				return MaintenanceUpdate{}, err
			}
			if fresh.Reason != "" {
				return maintenanceOutcome("skipped", "changed"), nil
			}
		}
	default:
		return maintenanceOutcome("failed", "unknown_stage"), nil
	}
	update.Guard = guard
	update.CredentialVersion = RefreshCredentialIdentity(record)
	return update, nil
}

func (r *MaintenanceRunner) waitRelogin(ctx context.Context, item MaintenanceItem) (MaintenanceUpdate, error) {
	job, err := r.options.Operation(ctx, item.OperationID)
	if errors.Is(err, operation.ErrNotFound) {
		return maintenanceOutcome("needs_action", "operation_missing"), nil
	}
	if err != nil {
		return MaintenanceUpdate{}, err
	}
	switch job.State {
	case operation.Succeeded:
		if job.Result.ProviderID <= 0 || job.Result.ProviderID == item.ProviderID {
			return maintenanceOutcome("needs_action", "replacement_missing"), nil
		}
		return MaintenanceUpdate{
			MaintenanceResult: MaintenanceResult{Stage: "verify"},
			ReplacementID:     job.Result.ProviderID,
		}, nil
	case operation.RequiresAction:
		update := maintenanceOutcome("needs_action", job.Message)
		update.ReplacementID = job.Result.ProviderID
		return update, nil
	case operation.Failed:
		return maintenanceOutcome("failed", "relogin_failed"), nil
	default:
		return r.retry(job.Message, maintenancePollInterval), nil
	}
}

func (r *MaintenanceRunner) retryBusy(work MaintenanceWork, detail string) MaintenanceUpdate {
	if !r.options.Now().Before(work.StartedAt.Add(maintenanceBusyWait)) {
		return maintenanceOutcome("inconclusive", detail)
	}
	return r.retry(detail, 15*time.Second)
}

func (r *MaintenanceRunner) retry(detail string, delay time.Duration) MaintenanceUpdate {
	return MaintenanceUpdate{MaintenanceResult: MaintenanceResult{Detail: detail}, RetryAt: r.options.Now().Add(delay)}
}

func maintenanceOutcome(outcome, detail string) MaintenanceUpdate {
	return MaintenanceUpdate{MaintenanceResult: MaintenanceResult{Outcome: outcome, Detail: detail}}
}

func maintenanceAuthResult(stage string, status int, complete, available bool) MaintenanceUpdate {
	update := maintenanceOutcome("inconclusive", "auth_unconfirmed")
	update.HTTPStatus = status
	switch {
	case stage == "verify" && status == 401:
		update.Outcome, update.Detail, update.Action = "needs_action", "replacement_401", "replacement_401"
	case stage == "verify" && complete && status == 200:
		update.Outcome, update.Detail = "relogin_succeeded", ""
	case stage == "verify":
		update.Outcome, update.Detail = "needs_action", "replacement_unconfirmed"
	case complete && status == 200:
		update.Outcome, update.Detail, update.Action = "auth_recovered", "", "clear_401"
	case status == 401 && available:
		update.Outcome, update.Detail, update.Action = "", "", "begin_relogin"
	case status == 401:
		update.Outcome, update.Detail = "needs_action", "revive_unavailable"
	}
	return update
}

func maintenanceAuthFailure(status int, err error) string {
	switch {
	case errors.Is(err, ErrIQUnsupported):
		return "model_unavailable"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, ErrTestAnswerIncomplete):
		return "incomplete"
	case errors.Is(err, ErrTestAnswerLimit):
		return "output_limit"
	case status == 403:
		return "upstream_forbidden"
	case status == 429:
		return "rate_limited"
	case status >= 500:
		return "upstream_error"
	default:
		return "auth_unconfirmed"
	}
}

func maintenanceProbeResult(sample StateProbeResult) MaintenanceUpdate {
	var update MaintenanceUpdate
	switch {
	case sample.MintStatus == 401 || sample.ContinueStatus == 401:
		update = maintenanceOutcome("inconclusive", "upstream_401")
		update.Action, update.HTTPStatus = "record_401", 401
	case sample.MintStatus != 200 || sample.ContinueStatus != 200 || !sample.MintCompleted || !sample.ContinueCompleted || !sample.Minted || sample.Failure != "":
		update = maintenanceOutcome("inconclusive", sample.Failure)
	case sample.Verdict == "degraded" && sample.NewTicket:
		update.Stage = "candy"
	case sample.Verdict == "healthy" && !sample.NewTicket:
		update = maintenanceOutcome("stable", "")
	default:
		update = maintenanceOutcome("inconclusive", "probe_failed")
	}
	update.Probe = &sample
	return update
}

func maintenanceCandyResult(evidence PelicanEvidence, status int) MaintenanceUpdate {
	update := maintenanceOutcome("inconclusive", evidence.DetailCode)
	update.HTTPStatus = status
	switch {
	case status == 401:
		update.Detail, update.Action = "upstream_401", "record_401"
	case evidence.Status == "passed":
		update.Outcome, update.Detail = "candy_passed", ""
	case evidence.Status == "suspected" && evidence.DetailCode == "answer_mismatch":
		update.Outcome, update.Action = "suspended", "suspend"
	}
	if update.Outcome == "candy_passed" || update.Outcome == "suspended" {
		update.Answer = strings.ToValidUTF8(strings.ReplaceAll(evidence.Answer, "\x00", ""), "")
	}
	return update
}
