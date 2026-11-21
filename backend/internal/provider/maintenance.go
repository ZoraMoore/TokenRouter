package provider

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

const (
	MaintenanceRelogin = "relogin"
	MaintenanceIQ      = "iq"

	MaintenanceSchedule = "schedule"
	MaintenanceManual   = "manual"
)

var (
	ErrMaintenanceInvalid  = apperror.BadRequest("INVALID_MAINTENANCE_PLAN", "定时任务的名称、范围、模型或时间设置无效")
	ErrMaintenanceNotFound = apperror.NotFound("MAINTENANCE_NOT_FOUND", "定时任务或运行记录不存在")
	ErrMaintenanceChanged  = apperror.Conflict("MAINTENANCE_CHANGED", "定时任务已变更，请刷新后重试")
	ErrMaintenanceIdle     = errors.New("没有待执行的智能运维任务")

	maintenanceCronFields = regexp.MustCompile(`^[0-9A-Za-z*/,?\-]+$`)
)

// MaintenanceDraft 定义每轮扫描的范围。GroupIDs 的顺序决定跨组账号的测试分组。
type MaintenanceDraft struct {
	Name           string  `json:"name"`
	Kind           string  `json:"kind"`
	Scope          string  `json:"scope"`
	GroupIDs       []int64 `json:"group_ids"`
	ProviderIDs    []int64 `json:"provider_ids"`
	Model          string  `json:"model"`
	CronExpression string  `json:"cron_expression"`
	Enabled        bool    `json:"enabled"`
}

// MaintenancePlan 记录计划的版本和最近一次运行。
type MaintenancePlan struct {
	MaintenanceDraft
	ID            int64      `json:"id"`
	Revision      int64      `json:"-"`
	NextRunAt     *time.Time `json:"next_run_at,omitempty"`
	LastRunID     int64      `json:"last_run_id,omitempty"`
	LastRunAt     *time.Time `json:"last_run_at,omitempty"`
	LastRunStatus string     `json:"last_run_status,omitempty"`
}

// MaintenanceTarget 保存本轮每个账号选定的分组和模型。
type MaintenanceTarget struct {
	ProviderID   int64  `json:"provider_id"`
	ProviderName string `json:"provider_name"`
	GroupID      int64  `json:"group_id,omitempty"`
	Model        string `json:"model,omitempty"`
	ActualModel  string `json:"actual_model,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

// MaintenancePreview 随计划预览返回选中和具备资格的账号数。
type MaintenancePreview struct {
	Items    []MaintenanceTarget `json:"items"`
	Total    int                 `json:"total"`
	Eligible int                 `json:"eligible"`
}

// MaintenanceRun 保存一轮的时间和账号进度。
type MaintenanceRun struct {
	ID          int64      `json:"id"`
	PlanID      int64      `json:"plan_id"`
	Status      string     `json:"status"`
	Trigger     string     `json:"trigger"`
	ScheduledAt time.Time  `json:"scheduled_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	Total       int        `json:"total"`
	Completed   int        `json:"completed"`
	Running     int        `json:"running"`
	Remaining   int        `json:"remaining"`
}

// MaintenanceRunDetail 包含运行页按账号展示的证据。
type MaintenanceRunDetail struct {
	MaintenanceRun
	Items []MaintenanceItem `json:"items"`
}

// MaintenanceItem 记录本轮账号状态和重登操作编号。
type MaintenanceItem struct {
	MaintenanceTarget
	MaintenanceResult
	State         string     `json:"state"`
	OperationID   string     `json:"operation_id,omitempty"`
	ReplacementID int64      `json:"replacement_id,omitempty"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
}

// MaintenanceResult 保存管理员可见的探测结果。续接状态值留在探针内部。
type MaintenanceResult struct {
	Stage          string            `json:"stage,omitempty"`
	Outcome        string            `json:"outcome,omitempty"`
	Detail         string            `json:"detail,omitempty"`
	Probe          *StateProbeResult `json:"probe,omitempty"`
	Answer         string            `json:"answer,omitempty"`
	HTTPStatus     int               `json:"http_status,omitempty"`
	AuthHTTPStatus int               `json:"auth_http_status,omitempty"`
}

// MaintenancePlanStore 提供计划、轮次及历史的持久化。
type MaintenancePlanStore interface {
	Plans(context.Context) ([]MaintenancePlan, error)
	Plan(context.Context, int64) (MaintenancePlan, error)
	SavePlan(context.Context, MaintenancePlan) (MaintenancePlan, error)
	DuePlans(context.Context, time.Time) ([]MaintenancePlan, error)
	QueueRun(context.Context, MaintenancePlan, string, time.Time, *time.Time, []MaintenanceTarget) (int64, error)
	Runs(context.Context, int64) ([]MaintenanceRun, error)
	Run(context.Context, int64) (MaintenanceRunDetail, error)
}

// MaintenancePlanOptions 绑定服务器时区、Cron 解析和账号名单查询。
type MaintenancePlanOptions struct {
	Location *time.Location
	Now      func() time.Time
	NextRun  func(string, time.Time) (time.Time, error)
	Targets  func(context.Context, MaintenanceDraft) ([]MaintenanceTarget, error)
}

// MaintenanceService 管理智能运维的计划及手动运行。
// @project-doc docs/operations/provider_maintenance.md#maintenance_schedules
type MaintenanceService struct {
	store   MaintenancePlanStore
	options MaintenancePlanOptions
}

func NewMaintenanceService(store MaintenancePlanStore, options MaintenancePlanOptions) *MaintenanceService {
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Location == nil {
		options.Location = time.Local
	}
	return &MaintenanceService{store: store, options: options}
}

func (s *MaintenanceService) Timezone() string {
	return s.options.Location.String()
}

func (s *MaintenanceService) Plans(ctx context.Context) ([]MaintenancePlan, error) {
	return s.store.Plans(ctx)
}

func (s *MaintenanceService) Save(ctx context.Context, id int64, draft MaintenanceDraft) (MaintenancePlan, error) {
	draft, next, err := s.validate(draft)
	if err != nil {
		return MaintenancePlan{}, err
	}
	plan := MaintenancePlan{ID: id, MaintenanceDraft: draft}
	if draft.Enabled {
		plan.NextRunAt = &next
	}
	return s.store.SavePlan(ctx, plan)
}

func (s *MaintenanceService) Preview(ctx context.Context, draft MaintenanceDraft) (MaintenancePreview, error) {
	draft, _, err := s.validate(draft)
	if err != nil {
		return MaintenancePreview{}, err
	}
	targets, err := s.options.Targets(ctx, draft)
	if err != nil {
		return MaintenancePreview{}, err
	}
	preview := MaintenancePreview{Items: targets, Total: len(targets)}
	if preview.Items == nil {
		preview.Items = []MaintenanceTarget{}
	}
	for _, target := range targets {
		if target.Reason == "" {
			preview.Eligible++
		}
	}
	return preview, nil
}

func (s *MaintenanceService) RunNow(ctx context.Context, id int64) (int64, error) {
	plan, err := s.store.Plan(ctx, id)
	if err != nil {
		return 0, err
	}
	if plan.LastRunStatus == "running" || plan.LastRunStatus == "queued" {
		return plan.LastRunID, nil
	}
	return s.queue(ctx, plan, MaintenanceManual, s.options.Now().In(s.options.Location))
}

func (s *MaintenanceService) Runs(ctx context.Context, id int64) ([]MaintenanceRun, error) {
	return s.store.Runs(ctx, id)
}

func (s *MaintenanceService) Run(ctx context.Context, id int64) (MaintenanceRunDetail, error) {
	return s.store.Run(ctx, id)
}

// QueueDue 以当前时钟计算下次执行时间。停机期间错过的周期合并为一轮。
func (s *MaintenanceService) QueueDue(ctx context.Context) error {
	now := s.options.Now().In(s.options.Location)
	plans, err := s.store.DuePlans(ctx, now)
	if err != nil {
		return err
	}
	var failed error
	for _, plan := range plans {
		if _, err := s.queue(ctx, plan, MaintenanceSchedule, now); err != nil && !errors.Is(err, ErrMaintenanceChanged) {
			failed = errors.Join(failed, err)
		}
	}
	return failed
}

func (s *MaintenanceService) queue(ctx context.Context, plan MaintenancePlan, trigger string, now time.Time) (int64, error) {
	var targets []MaintenanceTarget
	if plan.LastRunStatus != "running" && plan.LastRunStatus != "queued" {
		var err error
		targets, err = s.options.Targets(ctx, plan.MaintenanceDraft)
		if err != nil {
			return 0, err
		}
	}
	scheduledAt := now
	var next *time.Time
	if plan.NextRunAt != nil && !plan.NextRunAt.After(now) {
		nextAt, err := s.options.NextRun(plan.CronExpression, now)
		if err != nil {
			return 0, err
		}
		next = &nextAt
		if trigger == MaintenanceSchedule {
			scheduledAt = *plan.NextRunAt
		}
	}
	return s.store.QueueRun(ctx, plan, trigger, scheduledAt, next, targets)
}

func (s *MaintenanceService) validate(draft MaintenanceDraft) (MaintenanceDraft, time.Time, error) {
	draft.Name = strings.TrimSpace(draft.Name)
	draft.Model = strings.TrimSpace(draft.Model)
	draft.CronExpression = strings.Join(strings.Fields(draft.CronExpression), " ")
	if draft.Name == "" || !utf8.ValidString(draft.Name) || utf8.RuneCountInString(draft.Name) > 80 || strings.ContainsFunc(draft.Name, unicode.IsControl) {
		return draft, time.Time{}, ErrMaintenanceInvalid
	}
	if draft.Kind != MaintenanceIQ && draft.Kind != MaintenanceRelogin ||
		draft.Kind == MaintenanceIQ && draft.Model == "" ||
		draft.Model != "" && !SupportsStateProbeModel(draft.Model) {
		return draft, time.Time{}, ErrMaintenanceInvalid
	}
	var valid bool
	draft.GroupIDs, valid = maintenanceIDs(draft.GroupIDs, 100)
	if !valid {
		return draft, time.Time{}, ErrMaintenanceInvalid
	}
	draft.ProviderIDs, valid = maintenanceIDs(draft.ProviderIDs, 1000)
	if !valid {
		return draft, time.Time{}, ErrMaintenanceInvalid
	}
	switch draft.Scope {
	case "all":
		valid = draft.Kind == MaintenanceRelogin && len(draft.GroupIDs) == 0 && len(draft.ProviderIDs) == 0
	case "groups":
		valid = len(draft.GroupIDs) > 0 && len(draft.ProviderIDs) == 0
	case "providers":
		valid = len(draft.ProviderIDs) > 0 && len(draft.GroupIDs) == 0
	default:
		valid = false
	}
	fields := strings.Fields(draft.CronExpression)
	if !valid || len(draft.CronExpression) > 120 || len(fields) != 5 {
		return draft, time.Time{}, ErrMaintenanceInvalid
	}
	for _, field := range fields {
		if !maintenanceCronFields.MatchString(field) {
			return draft, time.Time{}, ErrMaintenanceInvalid
		}
	}
	next, err := s.options.NextRun(draft.CronExpression, s.options.Now().In(s.options.Location))
	if err != nil || next.IsZero() {
		return draft, time.Time{}, ErrMaintenanceInvalid
	}
	return draft, next, nil
}

func maintenanceIDs(ids []int64, limit int) ([]int64, bool) {
	if len(ids) > limit {
		return nil, false
	}
	result := make([]int64, 0, len(ids))
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, false
		}
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result, true
}

// AllowsMaintenanceAuthCheck 判断账号状态是否允许定向复检。记录持久错误时系统会关闭调度开关。
func (r *Record) AllowsMaintenanceAuthCheck() bool {
	return r.Status == StatusError || r.Status == StatusActive && r.Schedulable
}

// HasMaintenance401 识别账号状态里记录的 OpenAI OAuth 认证故障。
func HasMaintenance401(record *Record) bool {
	if record == nil || record.Platform != PlatformOpenAI || record.Type != ProviderTypeOAuth || record.IsCredentialShadow() {
		return false
	}
	return verifiedRelogin401(record) || record.Status == StatusActive && HasMaintenance401Cooldown(record)
}

// HasMaintenance401Cooldown 区分认证冷却和其他原因的临时停调。
func HasMaintenance401Cooldown(record *Record) bool {
	return record.TempUnschedulableUntil != nil && relogin401Pattern.MatchString(strings.ToLower(strings.TrimSpace(record.TempUnschedulableReason)))
}
