package traffic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/provider/operation"
	"github.com/TokenFlux/TokenRouter/internal/routing"
	"github.com/TokenFlux/TokenRouter/internal/scheduler"
)

const (
	trafficObservation = 30 * time.Second
	trafficPoll        = 2 * time.Second
	trafficDeadline    = 10 * time.Minute
	trafficPlanTTL     = 30 * time.Second
	firstBatch         = 2
	laterBatch         = 4
	stageMove          = "migrating"
	stageObserve       = "observing"
)

// Session 是同一个用户和 API Key 下汇总的一条候选会话。
type Session struct {
	ID       string
	SourceID int64
	UserID   int64
	APIKeyID int64
	Models   []string
}

// CompletedRequest 是在目标上完成的请求。StartedAt 用来排除迁移前已经发出的请求。
type CompletedRequest struct {
	GroupID   int64
	SessionID string
	UserID    int64
	APIKeyID  int64
	StartedAt time.Time
}

// Observations 从用量和请求记录读取近期会话及上游错误。
type Observations interface {
	// Candidates 返回所选分组中来源类型和目标不同的候选会话。
	Candidates(context.Context, int64, int64, time.Time) ([]Session, error)
	Observe(context.Context, int64, int64, time.Time) ([]CompletedRequest, int, error)
}

// Stickiness 核对常规会话绑定，并记录每批条件写入的结果。
type Stickiness interface {
	EligibleOpenAISessionMoves(context.Context, int64, []scheduler.OpenAISessionMove, int) ([]scheduler.OpenAISessionMove, error)
	ApplyOpenAISessionMoves(context.Context, scheduler.OpenAISessionMoveBatch) (scheduler.OpenAISessionMoveResult, error)
}

// ModelRules 读取目标提供商生效的模型映射和白名单。
type ModelRules func(*provider.Record) *provider.ModelRulesSnapshot

// QuotaSettings 读取调度器使用的短期和周配额阈值。
type QuotaSettings func(context.Context) provider.QuotaAutoPauseSettings

type providerReader interface {
	GetByID(context.Context, int64) (*provider.Record, error)
}

type groupReader interface {
	GetByIDLite(context.Context, int64) (*routing.Group, error)
}

type concurrencyReader interface {
	GetProviderConcurrency(context.Context, int64) (int, error)
}

// Push 分批切换同组 HTTP Responses 会话，按完成请求和并发决定是否继续。
type Push struct {
	providers     providerReader
	groups        groupReader
	observations  Observations
	sticky        Stickiness
	concurrency   concurrencyReader
	modelRules    ModelRules
	quotaSettings QuotaSettings
	now           func() time.Time
}

type assignment struct {
	GroupID   int64
	Move      scheduler.OpenAISessionMove
	UserID    int64
	At        time.Time
	Confirmed bool `json:",omitempty"`
}

type progress struct {
	Started      time.Time
	Limit        int
	Batch        int
	BatchGroupID int64
	Pending      []assignment
	PlannedAt    time.Time
	Moved        []assignment
	Attempted    []string
	BatchStarted time.Time
	Peak         int
	Completed    int
}

// New 建立由管理任务调用的 HTTP Responses 会话推流用例。
func New(providers providerReader, groups groupReader, observations Observations, sticky Stickiness, concurrency concurrencyReader, rules ModelRules, quota QuotaSettings) *Push {
	return &Push{providers: providers, groups: groups, observations: observations, sticky: sticky, concurrency: concurrency, modelRules: rules, quotaSettings: quota, now: time.Now}
}

// Validate 在排队前检查目标是否具备接收指定分组流量的资格。
func (p *Push) Validate(ctx context.Context, providerID int64, groupIDs []int64) error {
	_, err := p.target(ctx, providerID, groupIDs)
	return err
}

// Run 执行一批迁移或读取一次观察记录。后台任务按返回的间隔再次调用。
func (p *Push) Run(ctx context.Context, job operation.Job, save operation.Checkpoint) (operation.Result, error) {
	state := progress{}
	if len(job.Data) > 0 {
		if err := json.Unmarshal(job.Data, &state); err != nil {
			return job.Result, fmt.Errorf("decode traffic progress: %w", err)
		}
	}
	err := p.run(ctx, job, &state, save)
	result := operation.Result{
		ProviderID:         job.ProviderID,
		MigratedSessions:   len(state.Moved),
		SuccessfulRequests: state.Completed,
		PeakConcurrency:    state.Peak,
		TargetConcurrency:  state.Limit,
	}
	if state.Limit > 0 {
		// 同一会话可以完成多次请求，待确认数按迁移的会话计算。
		unconfirmed := 0
		for _, moved := range state.Moved {
			if !moved.Confirmed {
				unconfirmed++
			}
		}
		result.UnconfirmedSessions = &unconfirmed
	}
	return result, err
}

func (p *Push) run(ctx context.Context, job operation.Job, state *progress, save operation.Checkpoint) error {
	now := p.now().UTC()
	target, targetErr := p.target(ctx, job.ProviderID, job.TrafficGroups())
	if state.Started.IsZero() && targetErr == nil {
		state.Started = now
		state.Limit = target.Concurrency
	}
	if targetErr == nil && target.Concurrency != state.Limit {
		targetErr = operation.NeedsAction("目标提供商的并发上限已变更，已停止推流")
	}

	var current int
	if targetErr == nil {
		count, err := p.concurrency.GetProviderConcurrency(ctx, job.ProviderID)
		if err != nil {
			return err
		}
		current = count
		state.Peak = max(state.Peak, current)
	}

	requests, status, err := p.readObservation(ctx, job, state)
	if err != nil {
		return err
	}
	if status == 401 || status == 429 {
		targetErr = operation.NeedsAction(fmt.Sprintf("目标提供商返回上游 %d，已停止推流", status))
	}
	deadline := !state.Started.IsZero() && now.Sub(state.Started) >= trafficDeadline

	if len(state.Pending) > 0 {
		// 从检查点恢复时先读取回执。未执行的批次需要重新读取候选和目标。
		if err := p.finishMove(ctx, job, state, 0, save); err != nil {
			return err
		}
	}

	currentBatchCompleted := state.countCompletions(job.GroupID, requests)
	if targetErr != nil {
		return targetErr
	}

	if !state.BatchStarted.IsZero() {
		if err := save(stageObserve, state); err != nil {
			return err
		}
		if currentBatchCompleted > 0 && state.Peak >= state.Limit {
			return nil
		}
		if deadline {
			return operation.NeedsAction("推流已达观察时限")
		}
		if now.Before(state.BatchStarted.Add(trafficObservation)) {
			return operation.RetryAfter("等待本批请求完成", trafficPoll)
		}
		if currentBatchCompleted == 0 {
			return operation.NeedsAction("观察期内未发现本批会话在目标提供商完成请求")
		}
	}

	if state.Peak >= state.Limit {
		return nil
	}
	if deadline {
		return operation.NeedsAction("推流已达观察时限")
	}

	size := laterBatch
	if len(state.Moved) == 0 {
		size = firstBatch
	}
	if err := p.planMove(ctx, job, target, state, now, min(size, state.Limit-current), save); err != nil {
		return err
	}
	if err := p.finishMove(ctx, job, state, state.Limit, save); err != nil {
		return err
	}
	return operation.RetryAfter("等待本批请求完成", trafficPoll)
}

func (p *Push) target(ctx context.Context, providerID int64, groupIDs []int64) (*provider.Record, error) {
	if providerID <= 0 || len(groupIDs) == 0 {
		return nil, operation.NeedsAction("请选择目标提供商和分组")
	}
	record, err := p.providers.GetByID(ctx, providerID)
	if errors.Is(err, provider.ErrProviderNotFound) || err == nil && record == nil {
		return nil, operation.NeedsAction("目标提供商已不存在")
	}
	if err != nil {
		return nil, err
	}
	if !record.IsOpenAIOAuth() || !record.SupportsOpenAIEndpointCapability(provider.OpenAIEndpointCapabilityResponses, nil) {
		return nil, operation.NeedsAction("目标提供商需要支持 OpenAI OAuth HTTP Responses")
	}
	if !record.IsSchedulable() || record.Concurrency < 1 {
		return nil, operation.NeedsAction("目标提供商当前无法调度")
	}
	now := p.now()
	if record.ExpiresAt != nil && !now.Before(*record.ExpiresAt) {
		return nil, operation.NeedsAction("目标提供商已到期")
	}
	settings := provider.QuotaAutoPauseSettings{}
	if p.quotaSettings != nil {
		settings = p.quotaSettings(ctx)
	}
	paused, _ := provider.EvaluateQuotaAutoPause(record.Platform, record.Extra, settings, now)
	shortUtilization, _ := provider.ResolveOpenAIQuotaUtilization(record.Extra, "5h", now)
	weeklyUtilization, _ := provider.ResolveOpenAIQuotaUtilization(record.Extra, "7d", now)
	if paused || shortUtilization >= 100 || weeklyUtilization >= 100 {
		return nil, operation.NeedsAction("目标提供商的额度达到停止调度阈值")
	}
	for _, groupID := range groupIDs {
		if !slices.Contains(record.GroupIDs, groupID) {
			return nil, operation.NeedsAction(fmt.Sprintf("目标提供商不属于分组 #%d", groupID))
		}
		group, err := p.groups.GetByIDLite(ctx, groupID)
		if errors.Is(err, routing.ErrGroupNotFound) || err == nil && (group == nil || !group.IsActive()) {
			return nil, operation.NeedsAction(fmt.Sprintf("分组 #%d 当前不可用", groupID))
		}
		if err != nil {
			return nil, err
		}
	}
	return record, nil
}

func (p *Push) readObservation(ctx context.Context, job operation.Job, state *progress) ([]CompletedRequest, int, error) {
	if state.Started.IsZero() {
		return nil, 0, nil
	}
	var requests []CompletedRequest
	var status int
	for _, groupID := range job.TrafficGroups() {
		groupRequests, groupStatus, err := p.observations.Observe(ctx, job.ProviderID, groupID, state.Started)
		if err != nil {
			return nil, 0, err
		}
		if groupStatus != 0 && (status == 0 || groupStatus < status) {
			status = groupStatus
		}
		for _, request := range groupRequests {
			request.GroupID = groupID
			requests = append(requests, request)
		}
	}
	return requests, status, nil
}

func (p *Push) planMove(ctx context.Context, job operation.Job, target *provider.Record, state *progress, now time.Time, count int, save operation.Checkpoint) error {
	// 每批只迁移一个分组。下一批从上次选中的分组之后开始找候选。
	groups := job.TrafficGroups()
	start := slices.Index(groups, state.BatchGroupID) + 1
	rules := p.modelRules(target)
	for offset := range groups {
		groupID := groups[(start+offset)%len(groups)]
		pending, err := p.planGroup(ctx, job.ProviderID, groupID, rules, state.Attempted, now, count)
		if err != nil {
			return err
		}
		if len(pending) == 0 {
			continue
		}
		state.Batch++
		state.BatchGroupID = groupID
		state.PlannedAt = now
		state.Pending = pending
		return save(stageMove, state)
	}
	return operation.NeedsAction("所选分组没有来自另一类账号的可迁移会话")
}

func (p *Push) planGroup(ctx context.Context, providerID, groupID int64, rules *provider.ModelRulesSnapshot, attempted []string, now time.Time, count int) ([]assignment, error) {
	candidates, err := p.observations.Candidates(ctx, providerID, groupID, now)
	if err != nil {
		return nil, err
	}
	known := make(map[string]int, len(candidates))
	for _, candidate := range candidates {
		hash, _ := scheduler.DeriveSessionHashes(candidate.ID)
		known[hash]++
	}
	users := make(map[string]int64, len(candidates))
	moves := make([]scheduler.OpenAISessionMove, 0, len(candidates))
	for _, candidate := range candidates {
		hash, _ := scheduler.DeriveSessionHashes(candidate.ID)
		if hash == "" || known[hash] != 1 || (slices.Contains(attempted, hash) || slices.Contains(attempted, attemptKey(groupID, hash))) ||
			candidate.SourceID <= 0 || candidate.SourceID == providerID || candidate.UserID <= 0 || candidate.APIKeyID <= 0 ||
			!supportsModels(rules, candidate.Models) {
			continue
		}
		users[hash] = candidate.UserID
		moves = append(moves, scheduler.OpenAISessionMove{Hash: hash, SourceID: candidate.SourceID, APIKeyID: candidate.APIKeyID})
	}

	eligible, err := p.sticky.EligibleOpenAISessionMoves(ctx, groupID, moves, count)
	if err != nil {
		return nil, err
	}
	pending := make([]assignment, 0, len(eligible))
	for _, move := range eligible {
		pending = append(pending, assignment{GroupID: groupID, Move: move, UserID: users[move.Hash]})
	}
	return pending, nil
}

func (p *Push) finishMove(ctx context.Context, job operation.Job, state *progress, limit int, save operation.Checkpoint) error {
	moves := make([]scheduler.OpenAISessionMove, 0, len(state.Pending))
	for _, pending := range state.Pending {
		moves = append(moves, pending.Move)
	}
	groupID := state.BatchGroupID
	if groupID == 0 {
		// 旧单组任务的检查点没有保存批次分组。
		groupID = job.GroupID
	}
	result, err := p.sticky.ApplyOpenAISessionMoves(ctx, scheduler.OpenAISessionMoveBatch{
		JobID: job.ID, Number: state.Batch, GroupID: groupID,
		ProviderID: job.ProviderID, Limit: limit, Moves: moves,
	})
	if err != nil {
		if p.now().Sub(state.PlannedAt) < trafficPlanTTL {
			return operation.RetryAfter("正在确认会话迁移结果", trafficPoll)
		}
		return err
	}
	if !result.Available {
		if limit == 0 {
			state.Pending = nil
			return save(stageMove, state)
		}
		return operation.NeedsAction("目标提供商或分组已无法接收推流")
	}
	if len(result.Moved) != len(state.Pending) {
		return errors.New("traffic migration returned an incomplete receipt")
	}
	state.Peak = max(state.Peak, result.Concurrency)
	before := len(state.Moved)
	for index, pending := range state.Pending {
		pending.GroupID = groupID
		state.Attempted = append(state.Attempted, attemptKey(groupID, pending.Move.Hash))
		if result.Moved[index] {
			pending.At = result.AppliedAt
			state.Moved = append(state.Moved, pending)
		}
	}
	state.Pending = nil
	if before == len(state.Moved) {
		if err := save(stageMove, state); err != nil {
			return err
		}
		if state.Peak >= state.Limit {
			return nil
		}
		return operation.RetryAfter("会话绑定已变化，正在重新选择候选", trafficPoll)
	}
	state.BatchStarted = result.AppliedAt
	return save(stageObserve, state)
}

func attemptKey(groupID int64, hash string) string {
	return strconv.FormatInt(groupID, 10) + ":" + hash
}

func (state *progress) countCompletions(legacyGroupID int64, requests []CompletedRequest) int {
	all := 0
	batch := 0
	for _, request := range requests {
		hash, _ := scheduler.DeriveSessionHashes(request.SessionID)
		for index := range state.Moved {
			moved := &state.Moved[index]
			groupID := moved.GroupID
			if groupID == 0 {
				groupID = legacyGroupID
			}
			if request.GroupID == groupID && hash == moved.Move.Hash && request.UserID == moved.UserID &&
				request.APIKeyID == moved.Move.APIKeyID && !request.StartedAt.Before(moved.At) {
				moved.Confirmed = true
				all++
				if moved.At.Equal(state.BatchStarted) {
					batch++
				}
				break
			}
		}
	}
	state.Completed = max(state.Completed, all)
	return batch
}

func supportsModels(rules *provider.ModelRulesSnapshot, models []string) bool {
	if len(models) == 0 {
		return false
	}
	for _, model := range models {
		if !rules.Supports(model) {
			return false
		}
	}
	return true
}
