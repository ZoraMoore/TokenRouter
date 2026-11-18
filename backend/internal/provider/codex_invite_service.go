package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/mail"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

const (
	codexInviteEligibilityUnknown      = "CODEX_INVITE_ELIGIBILITY_UNKNOWN"
	codexReferralConsumer              = "codex_referral_consumer"
	codexReferralWorkspace             = "codex_referral_workspace"
	codexInviteResetMaxEmails          = 5
	codexInviteResetUnavailable        = "CODEX_INVITE_RESET_REFERRAL_UNAVAILABLE"
	codexInviteResetUnavailableMessage = "当前 Codex 推荐邀请入口暂不可用，但已有重置次数仍可使用"
)

// CodexInviteClient 为本次邀请操作提供平台请求接口。
type CodexInviteClient interface {
	GetJSON(context.Context, string, map[string]string) (map[string]any, error)
	PostJSON(context.Context, string, map[string]any) (map[string]any, error)
}

// CodexInviteResetOptions 注入读取、平台交换和随机标识，保持原读取时点。
type CodexInviteResetOptions struct {
	Read   func(context.Context, int64) (*Record, error)
	Client func(context.Context, *Record) (CodexInviteClient, error)
	NewID  func() string
	Warn   func(string, ...any)
}
type (
	CodexInviteResetService         struct{ Options CodexInviteResetOptions }
	codexInviteResetProviderContext struct {
		provider *Record
		client   CodexInviteClient
	}
)

// codexInviteResetInviteState 保存邀请操作结果，重置次数查询单独返回结果。
type codexInviteResetInviteState struct {
	eligibility        map[string]any
	capacity           *int
	available          bool
	unavailableReason  string
	unavailableMessage string
}

// codexInviteResetCreditState 保存 usage 基础数据和尽力获取到的 credit 明细。
type codexInviteResetCreditState struct {
	availableCount int
	credits        []CodexInviteResetCredit
}

func (s *CodexInviteResetService) prepareProvider(ctx context.Context, id int64) (*codexInviteResetProviderContext, error) {
	if id <= 0 {
		return nil, apperror.BadRequest("CODEX_INVITE_INVALID_PROVIDER", "提供商编号无效")
	}
	if s == nil || s.Options.Read == nil || s.Options.Client == nil {
		return nil, apperror.InternalServer("CODEX_INVITE_RESET_SERVICE_NOT_CONFIGURED", "codex invite reset service is not configured")
	}
	value, err := s.Options.Read(ctx, id)
	if err != nil {
		return nil, err
	}
	if value == nil {
		return nil, apperror.NotFound("PROVIDER_NOT_FOUND", "provider not found")
	}
	if !value.IsOpenAIOAuth() {
		return nil, apperror.BadRequest("CODEX_INVITE_RESET_UNSUPPORTED_PROVIDER", "only OpenAI OAuth providers support Codex invite reset")
	}
	if value.IsCredentialShadow() {
		return nil, ErrSparkShadowResetNotSupported
	}
	client, err := s.Options.Client(ctx, value)
	if err != nil {
		return nil, err
	}
	return &codexInviteResetProviderContext{provider: value, client: client}, nil
}

func (s *CodexInviteResetService) getJSON(ctx context.Context, value *codexInviteResetProviderContext, path string, query map[string]string) (map[string]any, error) {
	return value.client.GetJSON(ctx, path, query)
}

func (s *CodexInviteResetService) postJSON(ctx context.Context, value *codexInviteResetProviderContext, path string, body map[string]any) (map[string]any, error) {
	return value.client.PostJSON(ctx, path, body)
}

// GetStatus 查询邀请资格和可用重置次数。
func (s *CodexInviteResetService) GetStatus(ctx context.Context, providerID int64) (*CodexInviteResetStatus, error) {
	providerCtx, err := s.prepareProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}

	inviteState := s.getInviteState(ctx, providerCtx)
	creditState, err := s.getCreditState(ctx, providerCtx)
	if err != nil {
		return nil, err
	}

	hasRewards := codexInviteResetOptionalBoolFromMap(inviteState.eligibility, "has_rewards")
	grantAction := codexInviteResetStringFromMap(inviteState.eligibility, "grant_action")
	grantAmount := codexInviteResetOptionalIntFromMap(inviteState.eligibility, "grant_amount")
	if grants := codexInviteResetMapSliceFromMap(inviteState.eligibility, "grants"); len(grants) > 0 {
		grant := grants[0]
		for _, item := range grants {
			if item["recipient"] == "referrer" {
				grant = item
				break
			}
		}
		grantAction = codexInviteResetStringFromMap(grant, "grant_type")
		grantAmount = codexInviteResetOptionalIntFromMap(grant, "amount")
		rewarded := true
		hasRewards = &rewarded
	}

	grantType := normalizeCodexInviteResetGrantType(hasRewards, grantAction)
	if grantType == "unknown" {
		grantAction = ""
	}
	return &CodexInviteResetStatus{
		ReferralKey:              codexReferralProgram(providerCtx.provider),
		AvailableInvites:         inviteState.capacity,
		EligibilityRules:         normalizeCodexInviteResetRules(inviteState.eligibility),
		ShouldShow:               codexInviteResetOptionalBoolFromMap(inviteState.eligibility, "should_show"),
		GrantAction:              grantAction,
		GrantAmount:              grantAmount,
		HasRewards:               hasRewards,
		GrantType:                grantType,
		InviteAvailable:          inviteState.available,
		InviteUnavailableReason:  inviteState.unavailableReason,
		InviteUnavailableMessage: inviteState.unavailableMessage,
		RequiresConsent:          codexInviteResetBoolFromMapDefault(inviteState.eligibility, "requires_explicit_confirmation", true),
		AvailableCount:           creditState.availableCount,
		Credits:                  creditState.credits,
	}, nil
}

// codexReferralProgram 按凭据区分个人与工作区邀请活动。
func codexReferralProgram(p *Record) string {
	if strings.EqualFold(strings.TrimSpace(p.GetCredential("account_type")), "workspace") {
		return codexReferralWorkspace
	}
	switch strings.ToLower(strings.TrimSpace(p.GetCredential("plan_type"))) {
	case "team", "business", "self_serve_business_prolite", "self_serve_business_usage_based", "free_workspace", "enterprise", "enterprise_cbp_usage_based", "enterprise_cbp_automation", "edu", "education":
		return codexReferralWorkspace
	default:
		return codexReferralConsumer
	}
}

// codexInviteCapacity 有奖励的活动同时受发送名额和奖励名额限制，缺少容量时保留未知。
func codexInviteCapacity(eligibility map[string]any) *int {
	if !codexInviteResetBoolFromMapDefault(eligibility, "should_show", false) {
		zero := 0
		return &zero
	}
	send := codexInviteResetOptionalIntFromMap(eligibility, "remaining_send_capacity")
	if send == nil {
		return nil
	}
	count := max(0, *send)
	offer := codexInviteResetStringFromMap(eligibility, "offer_id")
	grants, _ := eligibility["grants"].([]any)
	if len(grants) > 0 || (offer != "" && offer != "none") {
		reward := codexInviteResetOptionalIntFromMap(eligibility, "remaining_reward_capacity")
		if reward == nil {
			return nil
		}
		count = min(count, max(0, *reward))
	}
	return &count
}

// getInviteState 单独读取活动资格，管理员仍可使用已经获得的重置次数。
func (s *CodexInviteResetService) getInviteState(ctx context.Context, providerCtx *codexInviteResetProviderContext) codexInviteResetInviteState {
	eligibility, err := s.getJSON(ctx, providerCtx, "/referrals/invite/eligibility", map[string]string{
		"program_id": codexReferralProgram(providerCtx.provider), "entrypoint": "persistent",
	})
	state := codexInviteResetInviteState{eligibility: eligibility}
	if err != nil || eligibility == nil {
		state.unavailableReason = codexInviteEligibilityUnknown
		state.unavailableMessage = "暂时无法查询邀请资格，已有重置次数仍可使用"
		s.Options.Warn("codex_invite_reset_eligibility_unavailable", "provider_id", providerCtx.provider.ID)
		return state
	}
	state.capacity = codexInviteCapacity(eligibility)
	state.available = state.capacity != nil && *state.capacity > 0
	if !state.available {
		state.unavailableReason = codexInviteResetUnavailable
		state.unavailableMessage = codexInviteResetUnavailableMessage
	}
	return state
}

// getCreditState 以 usage 的可用次数为基础，明细接口失败时保留基础结果。
func (s *CodexInviteResetService) getCreditState(ctx context.Context, providerCtx *codexInviteResetProviderContext) (codexInviteResetCreditState, error) {
	usage, err := s.getJSON(ctx, providerCtx, "/wham/usage", map[string]string{
		"supports_rewardless_invites": codexInviteResetSupportsRewardless,
	})
	if err != nil {
		return codexInviteResetCreditState{}, err
	}

	usageCredits := codexInviteResetMapFromMap(usage, "rate_limit_reset_credits")
	state := codexInviteResetCreditState{
		availableCount: codexInviteResetIntFromMap(usageCredits, "available_count"),
		credits:        normalizeCodexInviteResetCredits(usageCredits),
	}
	if codexInviteResetOptionalIntFromMap(usageCredits, "available_count") == nil {
		state.availableCount = countAvailableCodexInviteResetCredits(state.credits)
	}
	if state.availableCount <= 0 && len(state.credits) == 0 {
		return state, nil
	}

	details, detailsErr := s.getJSON(ctx, providerCtx, "/wham/rate-limit-reset-credits", nil)
	if detailsErr != nil {
		s.Options.Warn("codex_invite_reset_credit_details_unavailable", "provider_id", providerCtx.provider.ID)
		return state, nil
	}

	state.credits = normalizeCodexInviteResetCredits(details)
	if availableCount := codexInviteResetOptionalIntFromMap(details, "available_count"); availableCount != nil {
		state.availableCount = *availableCount
	}
	return state, nil
}

// SendInvite 发送 Codex 邀请邮件。
func (s *CodexInviteResetService) SendInvite(ctx context.Context, providerID int64, input CodexInviteRequest) (*CodexInviteResetInviteResult, error) {
	providerCtx, err := s.prepareProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	normalized, err := normalizeCodexInviteEmails(input.Emails)
	if err != nil {
		return nil, err
	}

	program := codexReferralProgram(providerCtx.provider)
	if input.ProgramID != program {
		return nil, apperror.Conflict("CODEX_INVITE_PROGRAM_CHANGED", "请刷新邀请资格后再发送")
	}
	state := s.getInviteState(ctx, providerCtx)
	if !state.available || len(normalized) > *state.capacity {
		if state.unavailableReason == codexInviteEligibilityUnknown {
			return nil, apperror.New(502, state.unavailableReason, state.unavailableMessage)
		}
		return nil, apperror.Conflict(codexInviteResetUnavailable, codexInviteResetUnavailableMessage)
	}
	if codexInviteResetBoolFromMapDefault(state.eligibility, "requires_explicit_confirmation", true) && !input.Confirmed {
		return nil, apperror.BadRequest("CODEX_INVITE_CONSENT_REQUIRED", "请先确认收件人同意接收邀请")
	}
	raw, err := s.postJSON(ctx, providerCtx, "/referrals/invite", map[string]any{
		"program_id": program, "entrypoint": "persistent", "emails": normalized,
	})
	if err != nil {
		return nil, err
	}

	invites, ok := raw["invites"].([]any)
	if raw["invites"] != nil && !ok {
		return nil, codexInviteSendUnknown()
	}
	for _, invite := range invites {
		switch v := invite.(type) {
		case map[string]any:
		case string:
			if v == "" {
				return nil, codexInviteSendUnknown()
			}
		default:
			return nil, codexInviteSendUnknown()
		}
	}
	// 只回显本次请求中确实失败的收件人，其他上游内容不进入响应。
	recipients := make(map[string]string, len(normalized))
	for _, email := range normalized {
		recipients[strings.ToLower(email)] = email
	}
	rawFailed, ok := raw["failed_emails"].([]any)
	if raw["failed_emails"] != nil && !ok {
		return nil, codexInviteSendUnknown()
	}
	failed := make([]string, 0, len(rawFailed))
	for _, value := range rawFailed {
		email, ok := value.(string)
		if !ok {
			return nil, codexInviteSendUnknown()
		}
		key := strings.ToLower(strings.TrimSpace(email))
		match, ok := recipients[key]
		if !ok {
			return nil, codexInviteSendUnknown()
		}
		failed = append(failed, match)
		delete(recipients, key)
	}
	if len(invites)+len(failed) != len(normalized) {
		return nil, codexInviteSendUnknown()
	}
	return &CodexInviteResetInviteResult{
		SentCount:    len(invites),
		FailedEmails: failed,
	}, nil
}

func codexInviteSendUnknown() error {
	return apperror.New(502, "CODEX_INVITE_SEND_UNKNOWN", "邀请结果未确认，请先在 Codex 中查看发送记录")
}

// Consume 使用一次可用的 Codex 重置机会。
func (s *CodexInviteResetService) Consume(ctx context.Context, providerID int64, creditID string) (*CodexInviteResetConsumeResult, error) {
	providerCtx, err := s.prepareProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	if len(creditID) > 256 || strings.ContainsAny(creditID, "\r\n\x00") {
		return nil, apperror.BadRequest("CODEX_INVITE_CREDIT_INVALID", "重置机会标识无效")
	}
	creditID = strings.TrimSpace(creditID)
	redeemRequestID := s.Options.NewID()

	payload := map[string]any{
		"redeem_request_id": redeemRequestID,
	}
	// 有明细选择器时精确消费指定 credit；无明细时由新版上游自动选择可用 credit。
	if creditID != "" {
		payload["credit_id"] = creditID
	}
	raw, err := s.postJSON(ctx, providerCtx, "/wham/rate-limit-reset-credits/consume", payload)
	if err != nil {
		return nil, err
	}

	return &CodexInviteResetConsumeResult{
		Code:            codexCreditOutcome(codexInviteResetStringFromMap(raw, "code"), codexInviteResetIntFromMap(raw, "windows_reset")),
		CreditID:        creditID,
		RedeemRequestID: redeemRequestID,
		WindowsReset:    codexInviteResetIntFromMap(raw, "windows_reset"),
		AvailableCount:  codexInviteResetOptionalIntFromMap(raw, "available_count"),
	}, nil
}

func normalizeCodexInviteEmails(emails []string) ([]string, error) {
	if len(emails) > codexInviteResetMaxEmails {
		return nil, apperror.BadRequest("CODEX_INVITE_RESET_EMAIL_LIMIT", "最多一次邀请 5 个邮箱")
	}
	result := make([]string, 0, len(emails))
	seen := make(map[string]struct{}, len(emails))
	for _, raw := range emails {
		for _, part := range splitCodexInviteEmailInput(raw) {
			email := strings.TrimSpace(part)
			if email == "" {
				continue
			}
			key := strings.ToLower(email)
			if _, exists := seen[key]; exists {
				continue
			}
			address, err := mail.ParseAddress(email)
			_, domain, hasAt := strings.Cut(email, "@")
			if len(email) > 254 || err != nil || address.Address != email || !hasAt || !strings.Contains(domain, ".") {
				return nil, apperror.BadRequest("CODEX_INVITE_RESET_INVALID_EMAIL", "收件邮箱格式错误")
			}
			seen[key] = struct{}{}
			result = append(result, email)
			if len(result) > codexInviteResetMaxEmails {
				return nil, apperror.BadRequest("CODEX_INVITE_RESET_EMAIL_LIMIT", fmt.Sprintf("最多一次邀请 %d 个邮箱", codexInviteResetMaxEmails))
			}
		}
	}
	if len(result) == 0 {
		return nil, apperror.BadRequest("CODEX_INVITE_RESET_EMAILS_REQUIRED", "emails are required")
	}
	return result, nil
}

func splitCodexInviteEmailInput(input string) []string {
	return strings.FieldsFunc(input, func(r rune) bool {
		switch r {
		case ',', ';', '\n', '\r', '\t', ' ':
			return true
		default:
			return false
		}
	})
}

func normalizeCodexInviteResetCredits(raw map[string]any) []CodexInviteResetCredit {
	items := firstNonEmptyCodexInviteResetMapSlice(raw, "credits", "rate_limit_reset_credits", "items", "data")
	credits := make([]CodexInviteResetCredit, 0, len(items))
	for _, item := range items {
		id := codexInviteResetStringFromMap(item, "id")
		if id == "" {
			continue
		}
		credits = append(credits, CodexInviteResetCredit{
			ID:          id,
			Status:      codexInviteResetStringFromMap(item, "status"),
			Title:       codexInviteResetStringFromMap(item, "title"),
			Description: codexInviteResetStringFromMap(item, "description"),
			ResetType:   codexInviteResetFirstStringFromMap(item, "reset_type", "resetType"),
			GrantedAt:   codexInviteResetFirstStringFromMap(item, "granted_at", "grantedAt"),
			ExpiresAt:   codexInviteResetFirstStringFromMap(item, "expires_at", "expiresAt"),
		})
	}
	return credits
}

// firstNonEmptyCodexInviteResetMapSlice 兼容不同版本的 credit 明细容器字段。
func firstNonEmptyCodexInviteResetMapSlice(raw map[string]any, keys ...string) []map[string]any {
	for _, key := range keys {
		if items := codexInviteResetMapSliceFromMap(raw, key); len(items) > 0 {
			return items
		}
	}
	return nil
}

// countAvailableCodexInviteResetCredits 在 usage 只返回明细时补算可用次数。
func countAvailableCodexInviteResetCredits(credits []CodexInviteResetCredit) int {
	availableCount := 0
	for _, credit := range credits {
		if strings.EqualFold(credit.Status, "available") {
			availableCount++
		}
	}
	return availableCount
}

func normalizeCodexInviteResetRules(raw map[string]any) []string {
	rulesRaw, ok := raw["rules"].([]any)
	if !ok {
		return nil
	}
	rules := make([]string, 0, len(rulesRaw))
	for _, item := range rulesRaw {
		switch value := item.(type) {
		case string:
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				rules = append(rules, trimmed)
			}
		case map[string]any:
			for _, key := range []string{"text", "description", "message", "title"} {
				if text := codexInviteResetStringFromMap(value, key); text != "" {
					rules = append(rules, text)
					break
				}
			}
		}
	}
	return rules
}

// normalizeCodexInviteResetGrantType 将奖励标记和原始动作归一化为管理端稳定枚举。
func normalizeCodexInviteResetGrantType(hasRewards *bool, action string) string {
	if hasRewards != nil && !*hasRewards {
		return "none"
	}
	switch strings.TrimSpace(action) {
	case "rate_limit_reset_credit":
		return "rate_limit_reset"
	case "workspace_credits":
		return "workspace_credits"
	default:
		return "unknown"
	}
}

func codexInviteResetStringFromMap(raw map[string]any, key string) string {
	value, _ := raw[key].(string)
	return strings.TrimSpace(value)
}

func codexInviteResetFirstStringFromMap(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := codexInviteResetStringFromMap(raw, key); value != "" {
			return value
		}
	}
	return ""
}

// codexInviteResetMapFromMap 安全读取嵌套对象，上游缺失字段时返回 nil。
func codexInviteResetMapFromMap(raw map[string]any, key string) map[string]any {
	if raw == nil {
		return nil
	}
	value, _ := raw[key].(map[string]any)
	return value
}

func codexInviteResetIntFromMap(raw map[string]any, key string) int {
	if n := codexInviteResetOptionalIntFromMap(raw, key); n != nil {
		return *n
	}
	return 0
}

// codexInviteResetOptionalIntFromMap 只接受非负整数，缺失的次数在页面中显示为未知。
func codexInviteResetOptionalIntFromMap(raw map[string]any, key string) *int {
	var count int
	switch n := raw[key].(type) {
	case float64:
		count = int(n)
		if float64(count) != n {
			return nil
		}
	case int:
		count = n
	case int64:
		count = int(n)
		if int64(count) != n {
			return nil
		}
	case json.Number:
		v, err := n.Int64()
		if err != nil {
			return nil
		}
		count = int(v)
		if int64(count) != v {
			return nil
		}
	default:
		return nil
	}
	if count < 0 {
		return nil
	}
	return &count
}

// codexInviteResetOptionalBoolFromMap 区分字段缺失和上游明确返回 false。
func codexInviteResetOptionalBoolFromMap(raw map[string]any, key string) *bool {
	if raw == nil {
		return nil
	}
	value, ok := raw[key]
	if !ok || value == nil {
		return nil
	}
	result := false
	switch v := value.(type) {
	case bool:
		result = v
	case string:
		result = strings.EqualFold(strings.TrimSpace(v), "true")
	default:
		return nil
	}
	return &result
}

func codexInviteResetBoolFromMapDefault(raw map[string]any, key string, fallback bool) bool {
	if raw == nil {
		return fallback
	}
	value, ok := raw[key]
	if !ok || value == nil {
		return fallback
	}
	if b, ok := value.(bool); ok {
		return b
	}
	return fallback
}

func codexInviteResetMapSliceFromMap(raw map[string]any, key string) []map[string]any {
	values, ok := raw[key].([]any)
	if !ok {
		return nil
	}
	result := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if item, ok := value.(map[string]any); ok {
			result = append(result, item)
		}
	}
	return result
}
