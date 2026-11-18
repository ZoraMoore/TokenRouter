package provider

import (
	"context"

	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
)

// ClientRejectionObservation 是上游错误分类的只读结果，提供商决定是否停调。
type ClientRejectionObservation struct {
	Message                      string
	OrganizationDisabled         bool
	CreditBalanceExhausted       bool
	IdentityVerificationRequired bool
	WorkspaceDeactivated         bool
}

// ApplyBadRequest 根据已解析的错误输入更新提供商状态和错误消息。
func (s *HealthService) ApplyBadRequest(ctx context.Context, provider *Record, observation ClientRejectionObservation) bool {
	// "organization has been disabled" → 永久禁用
	if observation.OrganizationDisabled {
		msg := "Organization disabled (400): " + observation.Message
		s.ApplyAuthenticationFailure(ctx, provider, msg)
		return true
	} else if provider.Platform == capability.PlatformAnthropic && observation.CreditBalanceExhausted {
		// Anthropic API Key 余额不足时按 402 处理，停止调度。
		msg := "Credit balance exhausted (400): " + observation.Message
		s.ApplyAuthenticationFailure(ctx, provider, msg)
		return true
	} else if observation.IdentityVerificationRequired {
		// KYC 身份验证要求 → 永久禁用，提供商需完成身份验证后才能恢复
		msg := "Identity verification required (400): " + observation.Message
		s.ApplyAuthenticationFailure(ctx, provider, msg)
		return true
	}
	// 其他 400 错误（如参数问题）不处理，不禁用提供商
	return false
}

// ApplyPaymentRequired 根据已解析的欠费错误更新提供商状态和错误消息。
func (s *HealthService) ApplyPaymentRequired(ctx context.Context, provider *Record, observation ClientRejectionObservation) bool {
	// 国产供应商：余额不足是可恢复状态（充值/检测恢复后由周期任务自动解除），
	// 此处将提供商临时停调，冷却结束后可恢复。
	if provider.IsCNProvider() {
		s.ApplyCNInsufficientBalance(ctx, provider, observation.Message)
		return true
	}
	// OpenAI: deactivated_workspace 表示工作区已停用，直接标记 error
	if provider.Platform == capability.PlatformOpenAI && observation.WorkspaceDeactivated {
		msg := "Workspace deactivated (402): workspace has been deactivated"
		s.ApplyAuthenticationFailure(ctx, provider, msg)
		return true
	}
	// 支付要求：余额不足或计费问题，停止调度
	msg := "Payment required (402): insufficient balance or billing issue"
	if observation.Message != "" {
		msg = "Payment required (402): " + observation.Message
	}
	s.ApplyAuthenticationFailure(ctx, provider, msg)
	return true
}
