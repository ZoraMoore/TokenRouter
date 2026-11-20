package provider

import (
	"context"
	"encoding/json"
	"net/mail"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
	"github.com/TokenFlux/TokenRouter/internal/provider/operation"
)

const (
	reloginDeletePending = "relogin_delete_pending"
	reloginWaitDelete    = "relogin_wait_delete"
	reloginSyncPending   = "relogin_sync_pending"
	reloginWaitSync      = "relogin_wait_sync"
	reloginRestore       = "relogin_restore"
)

var (
	reviveIDPattern     = regexp.MustCompile("^[0-9a-f]{32}$")
	relogin401Pattern   = regexp.MustCompile(`^(?:token revoked \(401\)|unauthorized \(401\)|authentication failed \(401\)|oauth 401(?: \(no refresh_token\))?|upstream (?:http |status )?401)(?:$|[: .])`)
	reloginEmailPattern = regexp.MustCompile(`[a-zA-Z0-9.!#$%&'*+/=?^_{|}~-]+@[a-zA-Z0-9-]+(?:\.[a-zA-Z0-9-]+)+`)
)

// ReloginService 从 ReviveToken 移除失效的号池绑定，重新登录并恢复调度配置。
type ReloginService struct {
	providers reloginProviders
	remote    ReviveToken
}

type reloginRepository interface {
	GetByID(context.Context, int64) (*Record, error)
	ListShadowsByParent(context.Context, int64) ([]*Record, error)
	FindReloginProviders(context.Context, string) ([]Record, error)
	RestoreReloginSettings(context.Context, int64, string, string, ReloginSettings, ReloginSettings) (bool, error)
}

type reloginProviders interface {
	reloginRepository
	ValidateGroupIDs(context.Context, []int64) error
}

type adminReloginProviders struct {
	reloginRepository
	*Admin
}

// ReviveAccount 是 ReviveToken 中保存号池绑定和登录资料状态的档案。
type ReviveAccount struct {
	ID              string            `json:"id"`
	AccountID       string            `json:"account_id"`
	Email           string            `json:"email"`
	CredentialSaved bool              `json:"credentials_saved"`
	PoolState       string            `json:"pool_state"`
	ProviderID      *int64            `json:"provider_id"`
	LastOperation   *ReviveOperation  `json:"last_operation"`
	Operations      []ReviveOperation `json:"operations"`
}

// ReviveOperation 是 ReviveToken 接受的移除或重新登录操作。
type ReviveOperation struct {
	ID          string `json:"id"`
	OperationID string `json:"operation_id"`
	AccountID   string `json:"account_id"`
	State       string `json:"state"`
}

// ReviveToken 提供档案读取和异步重登操作。
type ReviveToken interface {
	Configured() bool
	FindAccount(context.Context, string) (string, error)
	GetAccount(context.Context, string) (ReviveAccount, error)
	DeletePoolAccount(context.Context, string, string) (string, error)
	SyncAccount(context.Context, string, string) (string, error)
	GetOperation(context.Context, string) (ReviveOperation, error)
}

type reloginState struct {
	Email               string          `json:"email"`
	AccountID           string          `json:"account_id"`
	CredentialVersion   string          `json:"credential_version"`
	ChatGPTAccountID    string          `json:"chatgpt_account_id,omitempty"`
	ChatGPTUserID       string          `json:"chatgpt_user_id,omitempty"`
	DeleteKey           string          `json:"delete_key"`
	DeleteAttempted     bool            `json:"delete_attempted,omitempty"`
	DeleteOperationID   string          `json:"delete_operation_id,omitempty"`
	SyncKey             string          `json:"sync_key"`
	SyncAttempted       bool            `json:"sync_attempted,omitempty"`
	SyncOperationID     string          `json:"sync_operation_id,omitempty"`
	ReplacementID       int64           `json:"replacement_id,omitempty"`
	Settings            ReloginSettings `json:"settings"`
	ReplacementSettings ReloginSettings `json:"replacement_settings"`
}

// ReloginSettings 是替换提供商时恢复的分组、调度和到期配置。
type ReloginSettings struct {
	GroupIDs           []int64    `json:"group_ids"`
	Concurrency        int        `json:"concurrency"`
	Priority           int        `json:"priority"`
	LoadFactor         *int       `json:"load_factor"`
	ExpiresAt          *time.Time `json:"expires_at"`
	AutoPauseOnExpired bool       `json:"auto_pause_on_expired"`
	Schedulable        bool       `json:"schedulable"`
}

// NewReloginService 使用服务器持有的密钥连接 ReviveToken。
func NewReloginService(admin *Admin, remote ReviveToken) *ReloginService {
	s := &ReloginService{remote: remote}
	if admin != nil {
		if repo, ok := admin.providerRepo.(reloginRepository); ok {
			s.providers = adminReloginProviders{reloginRepository: repo, Admin: admin}
		}
	}
	return s
}

// Preview 核对失效账号和外部档案，返回本次可提交的项目。
func (s *ReloginService) Preview(ctx context.Context, providerID int64) (operation.PreviewItem, error) {
	item := operation.PreviewItem{ProviderID: providerID}
	record, _, err := s.preflight(ctx, providerID)
	if record != nil {
		item.Name = record.Name
	}
	if err != nil {
		if reason, ok := operation.PublicMessage(err); ok {
			item.Reason = reason
			return item, nil
		}
		return item, err
	}
	item.Eligible = true
	return item, nil
}

// Run 执行可续跑的重登步骤，外部写请求发出前保存对应的幂等键。
func (s *ReloginService) Run(ctx context.Context, job operation.Job, checkpoint operation.Checkpoint) (operation.Result, error) {
	if s.providers == nil || s.remote == nil || !s.remote.Configured() {
		return operation.Result{}, s.availability()
	}
	var state reloginState
	stage := job.Stage
	if (len(job.Data) == 0 || string(job.Data) == "{}") && (stage == "" || stage == "queued") {
		record, accountID, err := s.preflight(ctx, job.ProviderID)
		if err != nil {
			return operation.Result{}, err
		}
		state = reloginState{
			Email:             reloginEmail(record),
			AccountID:         accountID,
			CredentialVersion: RefreshCredentialIdentity(record),
			ChatGPTAccountID:  strings.TrimSpace(record.GetCredential("chatgpt_account_id")),
			ChatGPTUserID:     strings.TrimSpace(record.GetCredential("chatgpt_user_id")),
			DeleteKey:         "tokenrouter:" + job.ID + ":delete",
			SyncKey:           "tokenrouter:" + job.ID + ":sync",
			Settings:          reloginSettingsOf(record),
		}
		if state.CredentialVersion == "" || job.ID == "" {
			return operation.Result{}, operation.NeedsAction("无法保存待恢复账号的身份记录")
		}
		stage = reloginDeletePending
		if err := checkpoint(stage, state); err != nil {
			return operation.Result{}, err
		}
	} else if err := json.Unmarshal(job.Data, &state); err != nil || state.Email == "" || !reviveIDPattern.MatchString(state.AccountID) {
		return operation.Result{}, operation.NeedsAction("无法读取重登任务的检查点")
	}

	for {
		if err := ctx.Err(); err != nil {
			return operation.Result{}, err
		}
		switch stage {
		case reloginDeletePending:
			if err := s.submitReloginDelete(ctx, job.ProviderID, &state, checkpoint); err != nil {
				return operation.Result{}, err
			}
			if err := checkpoint(reloginWaitDelete, state); err != nil {
				return operation.Result{}, err
			}
			return operation.Result{}, operation.RetryLater("等待 ReviveToken 完成号池绑定移除")
		case reloginWaitDelete:
			if err := s.awaitReloginOperation(ctx, state.AccountID, state.DeleteOperationID); err != nil {
				return operation.Result{}, err
			}
			account, err := s.verifiedAccount(ctx, state)
			if err != nil {
				return operation.Result{}, err
			}
			if !account.removed() {
				if account.boundTo(job.ProviderID) {
					return operation.Result{}, operation.RetryLater("等待 ReviveToken 确认号池绑定已移除")
				}
				return operation.Result{}, operation.NeedsAction("移除操作之后，ReviveToken 档案的号池绑定已改变")
			}
			if err := s.verifyLocalRemoval(ctx, job.ProviderID); err != nil {
				return operation.Result{}, err
			}
			stage = reloginSyncPending
		case reloginSyncPending:
			if err := s.submitReloginSync(ctx, job.ProviderID, &state, checkpoint); err != nil {
				return operation.Result{}, err
			}
			if err := checkpoint(reloginWaitSync, state); err != nil {
				return operation.Result{}, err
			}
			return operation.Result{}, operation.RetryLater("等待 ReviveToken 完成重新登录")
		case reloginWaitSync:
			if err := s.awaitReloginOperation(ctx, state.AccountID, state.SyncOperationID); err != nil {
				return operation.Result{}, err
			}
			if err := s.captureReloginReplacement(ctx, job.ProviderID, &state); err != nil {
				return operation.Result{ProviderID: state.ReplacementID}, err
			}
			stage = reloginRestore
		case reloginRestore:
			err := s.restoreRelogin(ctx, state)
			return operation.Result{ProviderID: state.ReplacementID}, err
		default:
			return operation.Result{}, operation.NeedsAction("无法识别重登任务的检查点")
		}
		if err := checkpoint(stage, state); err != nil {
			return operation.Result{}, err
		}
	}
}

func (s *ReloginService) availability() error {
	if s.providers == nil {
		return operation.NeedsAction("重登任务缺少提供商配置存储")
	}
	if s.remote == nil || !s.remote.Configured() {
		return operation.NeedsAction("服务端缺少 REVIVE_API_KEY")
	}
	return nil
}

func (s *ReloginService) preflight(ctx context.Context, providerID int64) (*Record, string, error) {
	if err := s.availability(); err != nil {
		return nil, "", err
	}
	record, err := s.current401(ctx, providerID)
	if err != nil {
		return record, "", err
	}
	email := reloginEmail(record)
	if email == "" {
		return record, "", operation.NeedsAction("无法从提供商确认唯一的完整邮箱")
	}
	if err := s.providers.ValidateGroupIDs(ctx, record.GroupIDs); err != nil {
		if apperror.IsNotFound(err) {
			return record, "", operation.NeedsAction("账号的关联分组已经删除")
		}
		return record, "", operation.RetryLater("无法核对账号分组")
	}
	accountID, err := s.remote.FindAccount(ctx, email)
	if err != nil {
		return record, "", err
	}
	state := reloginState{AccountID: accountID, Email: email}
	account, err := s.verifiedAccount(ctx, state)
	if err != nil {
		return record, "", err
	}
	if !account.boundTo(providerID) {
		return record, "", operation.NeedsAction("ReviveToken 当前号池绑定与选中的失效账号不一致")
	}
	if account.operationInFlight() {
		return record, "", operation.RetryLater("ReviveToken 档案已有在途操作")
	}
	return record, accountID, nil
}

func (s *ReloginService) current401(ctx context.Context, id int64) (*Record, error) {
	record, err := s.providers.GetByID(ctx, id)
	if apperror.IsNotFound(err) || (err == nil && record == nil) {
		return nil, operation.NeedsAction("选中的提供商已经移除")
	}
	if err != nil {
		return nil, operation.RetryLater("暂时无法读取失效提供商")
	}
	if !verifiedRelogin401(record) {
		return record, operation.NeedsAction("当前记录没有可确认的 OpenAI OAuth 上游 HTTP 401")
	}
	shadows, err := s.providers.ListShadowsByParent(ctx, id)
	if err != nil {
		return record, operation.RetryLater("暂时无法核对 Spark 影子关联")
	}
	if len(shadows) > 0 {
		return record, operation.NeedsAction("账号有 Spark 影子，移除主账号会一并移除影子")
	}
	return record, nil
}

func (s *ReloginService) submitReloginDelete(ctx context.Context, providerID int64, state *reloginState, checkpoint operation.Checkpoint) error {
	account, err := s.verifiedAccount(ctx, *state)
	if err != nil {
		return err
	}
	switch {
	case account.boundTo(providerID):
		current, err := s.current401(ctx, providerID)
		if err != nil {
			return err
		}
		if reloginEmail(current) != state.Email || RefreshCredentialIdentity(current) != state.CredentialVersion || !state.Settings.Matches(current) {
			return operation.NeedsAction("失效账号的身份或配置已改变")
		}
	case state.DeleteAttempted && account.removed():
		// 响应丢失后可能已经解绑。幂等键用于取回本次操作 ID。
		if err := s.verifyLocalRemoval(ctx, providerID); err != nil {
			return err
		}
	default:
		return operation.NeedsAction("ReviveToken 号池绑定已改变，需核对当前账号")
	}
	if account.operationInFlight() {
		return operation.RetryLater("ReviveToken 档案已有在途操作")
	}
	if !state.DeleteAttempted {
		state.DeleteAttempted = true
		if err := checkpoint(reloginDeletePending, *state); err != nil {
			return err
		}
	}
	state.DeleteOperationID, err = s.remote.DeletePoolAccount(ctx, state.AccountID, state.DeleteKey)
	return err
}

func (s *ReloginService) awaitReloginOperation(ctx context.Context, accountID, operationID string) error {
	op, err := s.remote.GetOperation(ctx, operationID)
	if err != nil {
		return err
	}
	if op.ID != operationID || (op.AccountID != "" && op.AccountID != accountID) {
		return operation.NeedsAction("ReviveToken 返回的操作与本次任务不匹配")
	}
	if revivePending(op.State) {
		return operation.RetryLater("ReviveToken 仍在执行，请等待操作结果")
	}
	switch op.State {
	case "succeeded":
		return nil
	case "failed", "needs_action", "cancelled":
		return operation.NeedsAction("ReviveToken 操作已结束，状态为 " + op.State)
	default:
		return operation.NeedsAction("ReviveToken 返回了无法识别的操作状态")
	}
}

func (s *ReloginService) submitReloginSync(ctx context.Context, oldID int64, state *reloginState, checkpoint operation.Checkpoint) error {
	account, err := s.verifiedAccount(ctx, *state)
	if err != nil {
		return err
	}
	if account.operationInFlight() {
		return operation.RetryLater("ReviveToken 档案已有在途操作")
	}
	if !account.removed() {
		if !state.SyncAttempted || account.ProviderID == nil || *account.ProviderID == oldID || !account.boundTo(*account.ProviderID) {
			return operation.NeedsAction("提交重新登录前，ReviveToken 档案的号池绑定已改变")
		}
		// 已重导入时，重放同一幂等键可取回丢失的同步响应。
		if err := s.verifyReloginReplacement(ctx, *state, *account.ProviderID); err != nil {
			return err
		}
	}
	if err := s.verifyLocalRemoval(ctx, oldID); err != nil {
		return err
	}
	if !state.SyncAttempted {
		state.SyncAttempted = true
		if err := checkpoint(reloginSyncPending, *state); err != nil {
			return err
		}
	}
	state.SyncOperationID, err = s.remote.SyncAccount(ctx, state.AccountID, state.SyncKey)
	return err
}

func (s *ReloginService) verifyLocalRemoval(ctx context.Context, id int64) error {
	record, err := s.providers.GetByID(ctx, id)
	if apperror.IsNotFound(err) || (err == nil && record == nil) {
		return nil
	}
	return operation.RetryLater("等待号池确认失效记录已移除")
}

func (s *ReloginService) verifiedAccount(ctx context.Context, state reloginState) (ReviveAccount, error) {
	account, err := s.remote.GetAccount(ctx, state.AccountID)
	if err != nil {
		return ReviveAccount{}, err
	}
	if account.ID != state.AccountID || normalizeReloginEmail(account.Email) != state.Email {
		return ReviveAccount{}, operation.NeedsAction("ReviveToken 档案与当前账号的邮箱或档案 ID 不一致")
	}
	if !account.CredentialSaved {
		return ReviveAccount{}, operation.NeedsAction("ReviveToken 档案缺少已保存的登录资料")
	}
	return account, nil
}

func (s *ReloginService) captureReloginReplacement(ctx context.Context, oldID int64, state *reloginState) error {
	account, err := s.verifiedAccount(ctx, *state)
	if err != nil {
		return err
	}
	if account.ProviderID == nil {
		return operation.RetryLater("等待 ReviveToken 回写重新登录后的号池绑定")
	}
	id := *account.ProviderID
	if id == oldID || !account.boundTo(id) {
		return operation.NeedsAction("ReviveToken 未返回有效的新号池绑定")
	}
	record, err := s.loadReloginReplacement(ctx, *state, id)
	if err != nil {
		return err
	}
	state.ReplacementID = id
	state.ReplacementSettings = reloginSettingsOf(record)
	return nil
}

func (s *ReloginService) verifyReloginReplacement(ctx context.Context, state reloginState, id int64) error {
	_, err := s.loadReloginReplacement(ctx, state, id)
	return err
}

func (s *ReloginService) loadReloginReplacement(ctx context.Context, state reloginState, id int64) (*Record, error) {
	matches, err := s.providers.FindReloginProviders(ctx, state.Email)
	if err != nil {
		return nil, operation.RetryLater("暂时无法核对重新导入的账号")
	}
	if len(matches) == 0 {
		return nil, operation.RetryLater("等待新账号出现在号池")
	}
	if len(matches) != 1 || matches[0].ID != id || !MatchesReloginEmail(&matches[0], state.Email) {
		return nil, operation.NeedsAction("号池中的完整邮箱没有唯一匹配的新绑定")
	}
	r := &matches[0]
	if (state.ChatGPTAccountID != "" && state.ChatGPTAccountID != strings.TrimSpace(r.GetCredential("chatgpt_account_id"))) ||
		(state.ChatGPTUserID != "" && state.ChatGPTUserID != strings.TrimSpace(r.GetCredential("chatgpt_user_id"))) {
		return nil, operation.NeedsAction("新账号的 OpenAI 身份与失效账号不一致")
	}
	if verifiedRelogin401(r) || (r.Status == StatusActive && r.TempUnschedulableUntil != nil && time.Now().Before(*r.TempUnschedulableUntil) && relogin401Pattern.MatchString(strings.ToLower(strings.TrimSpace(r.TempUnschedulableReason)))) {
		return nil, operation.NeedsAction("重新登录的账号再次出现上游 HTTP 401，请人工检查")
	}
	if r.Status != StatusActive {
		return nil, operation.NeedsAction("重新登录的账号当前不是启用状态")
	}
	return r, nil
}

func (s *ReloginService) restoreRelogin(ctx context.Context, state reloginState) error {
	account, err := s.verifiedAccount(ctx, state)
	if err != nil {
		return err
	}
	if !account.boundTo(state.ReplacementID) {
		return operation.NeedsAction("重登后的号池绑定已改变")
	}
	current, err := s.loadReloginReplacement(ctx, state, state.ReplacementID)
	if err != nil {
		return err
	}
	desired := state.Settings
	desired.Schedulable = desired.Schedulable && (desired.ExpiresAt == nil || time.Now().Before(*desired.ExpiresAt))
	if desired.Matches(current) {
		return nil
	}
	if !state.ReplacementSettings.Matches(current) {
		return operation.NeedsAction("新账号的分组或调度配置已有人修改")
	}
	updated, err := s.providers.RestoreReloginSettings(ctx, current.ID, state.Email, RefreshCredentialIdentity(current), state.ReplacementSettings, desired)
	if err != nil {
		return operation.RetryLater("暂时无法恢复原账号的分组和调度配置")
	}
	if !updated {
		return operation.NeedsAction("新账号或调度配置已改变")
	}
	result, err := s.loadReloginReplacement(ctx, state, current.ID)
	if err != nil {
		return err
	}
	if !desired.Matches(result) {
		return operation.NeedsAction("新账号的分组或调度配置回读不一致")
	}
	return nil
}

func (a ReviveAccount) boundTo(id int64) bool {
	return id > 0 && a.PoolState == "present" && a.ProviderID != nil && *a.ProviderID == id
}

func (a ReviveAccount) removed() bool {
	return a.PoolState == "removed" && a.ProviderID == nil
}

func verifiedRelogin401(p *Record) bool {
	return p != nil && p.Status == StatusError && p.Platform == PlatformOpenAI && p.Type == ProviderTypeOAuth &&
		!p.IsCredentialShadow() && relogin401Pattern.MatchString(strings.ToLower(strings.TrimSpace(p.ErrorMessage)))
}

// MatchesReloginEmail 从独立 OpenAI OAuth 记录读取完整邮箱。
func MatchesReloginEmail(p *Record, email string) bool {
	return p != nil && p.Platform == PlatformOpenAI && p.Type == ProviderTypeOAuth && !p.IsCredentialShadow() && reloginEmail(p) == email
}

func reloginEmail(p *Record) string {
	if p == nil {
		return ""
	}
	if email := p.GetCredential("email"); strings.TrimSpace(email) != "" {
		return normalizeReloginEmail(email)
	}
	if email, _ := p.Extra["email"].(string); strings.TrimSpace(email) != "" {
		return normalizeReloginEmail(email)
	}
	values := reloginEmailPattern.FindAllString(p.Name, -1)
	if len(values) == 1 {
		return normalizeReloginEmail(values[0])
	}
	return ""
}

func normalizeReloginEmail(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	addr, err := mail.ParseAddress(value)
	if err != nil || addr.Address != value || !strings.Contains(addr.Address[strings.LastIndex(addr.Address, "@")+1:], ".") {
		return ""
	}
	return value
}

func reloginSettingsOf(p *Record) ReloginSettings {
	groups := slices.Clone(p.GroupIDs)
	slices.Sort(groups)
	return ReloginSettings{
		GroupIDs:           slices.Compact(groups),
		Concurrency:        p.Concurrency,
		Priority:           p.Priority,
		LoadFactor:         clonePointer(p.LoadFactor),
		ExpiresAt:          clonePointer(p.ExpiresAt),
		AutoPauseOnExpired: p.AutoPauseOnExpired,
		Schedulable:        p.Schedulable,
	}
}

// Matches 比较恢复任务会写入的分组和调度字段。
func (s ReloginSettings) Matches(p *Record) bool {
	if p == nil {
		return false
	}
	actual := reloginSettingsOf(p)
	return s.Concurrency == actual.Concurrency && s.Priority == actual.Priority &&
		reloginLoadEqual(s.LoadFactor, actual.LoadFactor) && reloginExpiryEqual(s.ExpiresAt, actual.ExpiresAt) &&
		s.AutoPauseOnExpired == actual.AutoPauseOnExpired && s.Schedulable == actual.Schedulable &&
		slices.Equal(s.GroupIDs, actual.GroupIDs)
}

func reloginLoadEqual(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func reloginExpiryEqual(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func (a ReviveAccount) operationInFlight() bool {
	if a.LastOperation != nil && revivePending(a.LastOperation.State) {
		return true
	}
	for _, op := range a.Operations {
		if revivePending(op.State) {
			return true
		}
	}
	return false
}

func revivePending(state string) bool {
	switch state {
	case "queued", "running", "waiting_sync":
		return true
	default:
		return false
	}
}
