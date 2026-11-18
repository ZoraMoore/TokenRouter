package provider

// 邀请重置结果使用公开的数据格式，交换操作由注入的执行接口完成。
type CodexInviteResetStatus struct {
	AvailableInvites *int     `json:"available_invites"`
	ReferralKey      string   `json:"referral_key"`
	EligibilityRules []string `json:"eligibility_rules,omitempty"`
	// ShouldShow 表示上游当前是否开放此邀请活动。
	ShouldShow *bool `json:"should_show,omitempty"`
	// GrantAction 是识别出的 Codex 奖励动作。
	GrantAction string `json:"grant_action,omitempty"`
	// GrantAmount 表示活动发给邀请人的奖励数量。
	GrantAmount *int `json:"grant_amount,omitempty"`
	// HasRewards 表示当前邀请活动是否会发放奖励，nil 表示上游没有返回该字段。
	HasRewards *bool `json:"has_rewards,omitempty"`
	// GrantType 是管理端使用的稳定奖励类型枚举。
	GrantType string `json:"grant_type,omitempty"`
	// InviteAvailable 表示当前提供商是否还能继续通过推荐入口发送 Codex 邀请。
	InviteAvailable bool `json:"invite_available"`
	// InviteUnavailableReason 是推荐入口不可用时返回给前端判断的稳定原因码。
	InviteUnavailableReason string `json:"invite_unavailable_reason,omitempty"`
	// InviteUnavailableMessage 是推荐入口不可用时展示给管理员的非致命提示。
	InviteUnavailableMessage string                   `json:"invite_unavailable_message,omitempty"`
	RequiresConsent          bool                     `json:"requires_consent"`
	AvailableCount           int                      `json:"available_count"`
	Credits                  []CodexInviteResetCredit `json:"credits"`
}

type CodexInviteResetCredit struct {
	ID          string `json:"id"`
	Status      string `json:"status,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	ResetType   string `json:"reset_type,omitempty"`
	GrantedAt   string `json:"granted_at,omitempty"`
	ExpiresAt   string `json:"expires_at,omitempty"`
}

// CodexInviteRequest 记录管理员选择的活动、收件人和本次发送的同意状态。
type CodexInviteRequest struct {
	Emails    []string `json:"emails" binding:"required,min=1,max=5,dive,required,max=254"`
	ProgramID string   `json:"program_id" binding:"required,max=64"`
	Confirmed bool     `json:"confirmed"`
}

type CodexInviteResetInviteResult struct {
	SentCount    int      `json:"sent_count"`
	FailedEmails []string `json:"failed_emails,omitempty"`
}

type CodexInviteResetConsumeResult struct {
	Code            string `json:"code,omitempty"`
	CreditID        string `json:"credit_id,omitempty"`
	RedeemRequestID string `json:"redeem_request_id"`
	WindowsReset    int    `json:"windows_reset"`
	AvailableCount  *int   `json:"available_count,omitempty"`
}
