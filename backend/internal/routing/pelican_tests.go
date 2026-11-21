package routing

import (
	"context"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/robfig/cron/v3"

	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
	"github.com/TokenFlux/TokenRouter/internal/pkg/locale"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
)

const (
	// PelicanStateProbe 测试指定账号能否继续同一段会话。
	PelicanStateProbe = "state_probe"
)

var (
	// 计划和作品共用资源不存在的响应，用户权限在读取正文前检查。
	ErrPelicanNotFound = apperror.NotFound("PELICAN_NOT_FOUND", "Test plan or result not found.")
	ErrPelicanConflict = apperror.Conflict("PELICAN_CONFLICT", "The test configuration or run state conflicts with this operation.")
	ErrPelicanInvalid  = apperror.BadRequest("PELICAN_INVALID", "Invalid test configuration.")
	pelicanCron        = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
)

// PelicanPlan 指定分组的测试模型、作答账号和运行时间。
type PelicanPlan struct {
	ID                   int64      `json:"id"`
	GroupID              int64      `json:"group_id"`
	GroupName            string     `json:"group_name"`
	ProviderID           *int64     `json:"provider_id"`
	ProviderName         string     `json:"provider_name,omitempty"`
	ModelID              string     `json:"model_id"`
	QuestionKind         string     `json:"question_kind"`
	QuestionName         string     `json:"question_name,omitempty"`
	QuestionPrompt       string     `json:"question_prompt,omitempty"`
	ExpectedAnswer       string     `json:"expected_answer,omitempty"`
	ReasoningEffort      string     `json:"reasoning_effort,omitempty"`
	CronExpression       string     `json:"cron_expression"`
	Enabled              bool       `json:"enabled"`
	NextRunAt            *time.Time `json:"next_run_at,omitempty"`
	LastRunAt            *time.Time `json:"last_run_at,omitempty"`
	RunState             string     `json:"run_state"`
	SuspendOnWrongAnswer bool       `json:"suspend_on_wrong_answer,omitempty"`
	LeaseToken           string     `json:"-"`
	RoundAt              time.Time  `json:"-"`
	Revision             int64      `json:"-"`
	CandidateIDs         []int64    `json:"-"`
}

// PelicanSummary 是列表中展示的一次作答。
type PelicanSummary struct {
	ID              int64     `json:"id"`
	ModelID         string    `json:"model_id"`
	QuestionKind    string    `json:"question_kind"`
	QuestionName    string    `json:"question_name,omitempty"`
	ReasoningEffort string    `json:"reasoning_effort,omitempty"`
	Status          string    `json:"status"`
	DetailCode      string    `json:"detail_code,omitempty"`
	TestedAt        time.Time `json:"tested_at"`
	LatencyMs       int64     `json:"latency_ms"`
	ScheduleAction  string    `json:"schedule_action,omitempty"`
}

// PelicanResult 保存当次的题目、检查要点和回答，作答账号供管理员排查。
type PelicanResult struct {
	PelicanSummary
	PlanID       int64    `json:"plan_id"`
	GroupID      int64    `json:"group_id"`
	ProviderID   *int64   `json:"provider_id,omitempty"`
	ProviderName string   `json:"provider_name,omitempty"`
	Prompt       string   `json:"prompt,omitempty"`
	Criteria     []string `json:"criteria,omitempty"`
	Answer       string   `json:"answer,omitempty"`
	RawAnswer    string   `json:"raw_answer,omitempty"`
	RoundID      string   `json:"round_id,omitempty"`
	RecheckRunID *int64   `json:"recheck_run_id,omitempty"`
}

// PelicanArtwork 是用户打开一次测试时可查看的题目和作答。
type PelicanArtwork struct {
	PelicanSummary
	Prompt    string   `json:"prompt"`
	Criteria  []string `json:"criteria"`
	Answer    string   `json:"answer"`
	RawAnswer string   `json:"raw_answer,omitempty"`
}

// PelicanAnswerPreview 在糖果题历史中显示模型当次的回答，完整正文由详情接口返回。
type PelicanAnswerPreview struct {
	PelicanSummary
	AnswerPreview string `json:"answer_preview"`
}

// PelicanLatest 包含当前测试计划的运行状态和最近一次样本。
type PelicanLatest struct {
	GroupID         int64                  `json:"-"`
	ModelID         string                 `json:"model_id"`
	QuestionKind    string                 `json:"question_kind"`
	QuestionName    string                 `json:"question_name,omitempty"`
	ReasoningEffort string                 `json:"reasoning_effort,omitempty"`
	Artworks        []PelicanSummary       `json:"artworks,omitempty"`
	Answers         []PelicanAnswerPreview `json:"answers,omitempty"`
	Enabled         bool                   `json:"enabled"`
	RunState        string                 `json:"run_state"`
	NextRunAt       *time.Time             `json:"next_run_at,omitempty"`
	Latest          *PelicanSummary        `json:"latest"`
}

// PelicanGroup 按用户有权使用的分组归集样本。
type PelicanGroup struct {
	GroupID   int64           `json:"group_id"`
	GroupName string          `json:"group_name"`
	Models    []PelicanLatest `json:"models"`
}

// PelicanProvider 是管理员在分组中可选择的作答账号。
type PelicanProvider struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	SupportsStateProbe bool   `json:"supports_state_probe"`
}

// PelicanTarget 保存分组映射后的型号以及提供商测试使用的文字端点。
type PelicanTarget struct {
	ProviderID     int64
	ModelID        string
	ActualModel    string
	Protocol       string
	GroupUpdatedAt time.Time
}

// PelicanRepository 持久化分组计划和结果，按领取凭证提交一轮作答。
type PelicanRepository interface {
	ListPlans(context.Context) ([]PelicanPlan, error)
	SavePlan(context.Context, PelicanPlan) (*PelicanPlan, error)
	DeletePlan(context.Context, int64) error
	QueuePlan(context.Context, int64) error
	ClaimNext(context.Context, time.Time, time.Time, string) (*PelicanPlan, error)
	Finish(context.Context, PelicanPlan, PelicanResult, *time.Time) error
	ListResults(context.Context, int64) ([]PelicanResult, error)
	Result(context.Context, int64, []int64) (*PelicanResult, error)
	Latest(context.Context, []int64) ([]PelicanLatest, error)
	LatestDrawings(context.Context, []int64) ([]PelicanLatest, error)
	Review(context.Context, int64, string) error
	ShowcaseSettings(context.Context) (PelicanShowcaseSettings, error)
	SaveShowcaseSettings(context.Context, PelicanShowcaseSettings) (PelicanShowcaseSettings, error)
	PruneShowcase(context.Context) error
}

// PelicanOptions 连接分组权限、选号、提供商测试和服务器时钟。
type PelicanOptions struct {
	Now             func() time.Time
	Validate        func(context.Context, PelicanPlan) error
	Providers       func(context.Context, int64) ([]PelicanProvider, error)
	Acquire         func(context.Context, PelicanPlan) (PelicanTarget, func(), error)
	Run             func(context.Context, int64, string, string, string, string, provider.PelicanQuestionConfig) provider.PelicanEvidence
	RunStateProbe   func(context.Context, int64, string) (provider.StateProbeResult, error)
	UserGroups      func(context.Context, int64) ([]Group, error)
	Candy           PelicanCandyRepository
	CandyCandidates func(context.Context, PelicanPlan) ([]int64, error)
}

// PelicanService 管理测试计划、手动运行和分组展示。
type PelicanService struct {
	repo    PelicanRepository
	options PelicanOptions
	wake    chan struct{}
}

func NewPelicanService(repo PelicanRepository, options PelicanOptions) *PelicanService {
	return &PelicanService{repo: repo, options: options, wake: make(chan struct{}, 1)}
}

// Save 使用同一份日历检查启动和暂停的计划，再校验账号与分组。
func (s *PelicanService) Save(ctx context.Context, plan PelicanPlan) (*PelicanPlan, error) {
	plan.ModelID = strings.TrimSpace(plan.ModelID)
	plan.CronExpression = strings.TrimSpace(plan.CronExpression)
	if plan.GroupID <= 0 || plan.ProviderID != nil && *plan.ProviderID <= 0 ||
		plan.ModelID == "" || len(plan.ModelID) > 128 || !utf8.ValidString(plan.ModelID) ||
		strings.Contains(plan.ModelID, "*") || strings.IndexFunc(plan.ModelID, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) >= 0 ||
		!plan.validQuestion() || plan.SuspendOnWrongAnswer && !plan.standardAutoCandy() {
		return nil, ErrPelicanInvalid
	}
	next, err := nextPelicanRun(plan.CronExpression, s.options.Now())
	if err != nil {
		return nil, err
	}
	if err := s.options.Validate(ctx, plan); err != nil {
		return nil, err
	}
	plan.NextRunAt = nil
	if plan.Enabled {
		plan.NextRunAt = &next
	}
	return s.repo.SavePlan(ctx, plan)
}

// validQuestion 校验题型和判分依据，绘图题交给管理员复核，文本题与预设答案比对。
func (p *PelicanPlan) validQuestion() bool {
	p.QuestionName = strings.TrimSpace(p.QuestionName)
	p.QuestionPrompt = strings.TrimSpace(p.QuestionPrompt)
	p.ExpectedAnswer = strings.TrimSpace(p.ExpectedAnswer)
	switch p.ReasoningEffort {
	case "", "low", "medium", "high", "xhigh":
	default:
		return false
	}
	if !validPelicanText(p.QuestionName, 96, false) ||
		!validPelicanText(p.QuestionPrompt, 8192, true) ||
		!validPelicanText(p.ExpectedAnswer, 1024, true) {
		return false
	}
	switch p.QuestionKind {
	case PelicanStateProbe:
		return p.ProviderID != nil && *p.ProviderID > 0 && p.QuestionPrompt == "" && p.ExpectedAnswer == "" && p.ReasoningEffort == ""
	case "pelican":
		return p.ExpectedAnswer == ""
	case "candy":
		return true
	case "custom_svg":
		return p.QuestionName != "" && p.QuestionPrompt != "" && p.ExpectedAnswer == ""
	case "custom_text":
		return p.QuestionName != "" && p.QuestionPrompt != "" && p.ExpectedAnswer != ""
	default:
		return false
	}
}

// validPelicanText 允许多行题面和答案中使用换行及制表符。
func validPelicanText(text string, limit int, multiline bool) bool {
	if len(text) > limit || !utf8.ValidString(text) {
		return false
	}
	for _, r := range text {
		if multiline && strings.ContainsRune("\r\n\t", r) {
			continue
		}
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// nextPelicanRun 拒绝没有下一次执行时间的日期，例如每年二月三十日。
func nextPelicanRun(expression string, from time.Time) (time.Time, error) {
	if len(expression) > 128 || len(strings.Fields(expression)) != 5 || strings.Contains(expression, "=") {
		return time.Time{}, ErrPelicanInvalid
	}
	schedule, err := pelicanCron.Parse(expression)
	if err != nil {
		return time.Time{}, ErrPelicanInvalid
	}
	next := schedule.Next(from)
	if next.IsZero() {
		return time.Time{}, ErrPelicanInvalid
	}
	return next, nil
}

// Queue 先保存手动执行请求，进程重启后仍可领取。
func (s *PelicanService) Queue(ctx context.Context, id int64) error {
	if err := s.repo.QueuePlan(ctx, id); err != nil {
		return err
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return nil
}

func (s *PelicanService) Plans(ctx context.Context) ([]PelicanPlan, error) {
	return s.repo.ListPlans(ctx)
}

func (s *PelicanService) Providers(ctx context.Context, groupID int64) ([]PelicanProvider, error) {
	return s.options.Providers(ctx, groupID)
}

func (s *PelicanService) Delete(ctx context.Context, id int64) error {
	return s.repo.DeletePlan(ctx, id)
}

func (s *PelicanService) History(ctx context.Context, id int64) ([]PelicanResult, error) {
	return s.repo.ListResults(ctx, id)
}

func (s *PelicanService) AdminResult(ctx context.Context, id int64) (*PelicanResult, error) {
	return s.repo.Result(ctx, id, nil)
}

func (s *PelicanService) Review(ctx context.Context, id int64, status string) error {
	if status != "passed" && status != "suspected" {
		return ErrPelicanInvalid
	}
	return s.repo.Review(ctx, id, status)
}

// Showcase 按分组选择器的权限和显示顺序列出当前计划。
// @project-doc docs/operations/provider_maintenance.md#pelican_group_tests
func (s *PelicanService) Showcase(ctx context.Context, userID int64) ([]PelicanGroup, error) {
	if _, err := s.requireShowcase(ctx, false); err != nil {
		return nil, err
	}
	groups, err := s.options.UserGroups(ctx, userID)
	if err != nil {
		return nil, err
	}
	return pelicanShowcaseGroups(ctx, groups, s.repo.Latest)
}

func pelicanShowcaseGroups(ctx context.Context, groups []Group, list func(context.Context, []int64) ([]PelicanLatest, error)) ([]PelicanGroup, error) {
	out := make([]PelicanGroup, 0, len(groups))
	ids := pelicanGroupIDs(groups)
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := list(ctx, ids)
	if err != nil {
		return nil, err
	}
	models := make(map[int64][]PelicanLatest, len(groups))
	for _, row := range rows {
		models[row.GroupID] = append(models[row.GroupID], row)
	}
	for i := range groups {
		group := &groups[i]
		if !group.IsActive() || len(models[group.ID]) == 0 {
			continue
		}
		display, _ := GroupDisplay(group, locale.FromContext(ctx))
		out = append(out, PelicanGroup{GroupID: group.ID, GroupName: display.DisplayName, Models: models[group.ID]})
	}
	return out, nil
}

// UserResult 在详情请求时重新读取分组权限，再读取该分组的题目与作品。
func (s *PelicanService) UserResult(ctx context.Context, userID, resultID int64) (*PelicanArtwork, error) {
	if _, err := s.requireShowcase(ctx, false); err != nil {
		return nil, err
	}
	groups, err := s.options.UserGroups(ctx, userID)
	if err != nil {
		return nil, err
	}
	ids := pelicanGroupIDs(groups)
	if len(ids) == 0 {
		return nil, ErrPelicanNotFound
	}
	result, err := s.repo.Result(ctx, resultID, ids)
	if err != nil {
		return nil, err
	}
	return &PelicanArtwork{PelicanSummary: result.PelicanSummary, Prompt: result.Prompt, Criteria: result.Criteria, Answer: result.Answer, RawAnswer: result.RawAnswer}, nil
}

func pelicanGroupIDs(groups []Group) []int64 {
	ids := make([]int64, 0, len(groups))
	for i := range groups {
		if groups[i].IsActive() {
			ids = append(ids, groups[i].ID)
		}
	}
	return ids
}

// PelicanProtocols 按当前分组和提供商的模型规则检查输入型号，返回可用于答题的对话协议。
func (p *GroupPolicyView) PelicanProtocols(ctx context.Context, model string, candidate CatalogueProvider) []capability.ProtocolID {
	if p == nil {
		return nil
	}
	query := requestableQuery{policy: p, providers: []CatalogueProvider{candidate}, protocols: []capability.ProviderProtocols{candidate.Protocols()}}
	resolved, ok := query.resolve(ctx, model)
	if !ok {
		return nil
	}
	var protocols []capability.ProtocolID
	for _, protocol := range resolved.Protocols {
		switch protocol {
		case capability.ProtocolOpenAIResponses, capability.ProtocolOpenAIChatCompletions, capability.ProtocolAnthropicMessages, capability.ProtocolGeminiGenerateContent:
			protocols = append(protocols, protocol)
		}
	}
	return protocols
}
