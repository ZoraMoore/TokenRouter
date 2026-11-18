package moderation

import (
	"context"

	"github.com/TokenFlux/TokenRouter/internal/notification/contract"
)

func riskPolicy(c *ContentModerationConfig) *contract.RiskPolicy {
	if c == nil {
		return nil
	}
	return &contract.RiskPolicy{BanThreshold: c.BanThreshold, CyberBanThreshold: c.CyberBanThreshold}
}

func riskLog(v *ContentModerationLog) *contract.RiskLog {
	if v == nil {
		return nil
	}
	return &contract.RiskLog{ID: v.ID, UserID: v.UserID, UserEmail: v.UserEmail, GroupName: v.GroupName, HighestCategory: v.HighestCategory, HighestScore: v.HighestScore, ViolationCount: v.ViolationCount, AutoBanned: v.AutoBanned, CreatedAt: v.CreatedAt}
}

func riskWarning(v *ContentModerationCyberWarning) *contract.RiskWarning {
	if v == nil {
		return nil
	}
	return &contract.RiskWarning{ID: v.ID, UserID: v.UserID, UserEmail: v.UserEmail, GroupName: v.GroupName, ProviderName: v.ProviderName, ViolationCount: v.ViolationCount, CreatedAt: v.CreatedAt}
}

func (s *ContentModerationService) sendViolationEmail(ctx context.Context, c *ContentModerationConfig, v *ContentModerationLog) error {
	return s.emailService.SendViolationEmail(ctx, riskPolicy(c), riskLog(v))
}

func (s *ContentModerationService) sendAccountDisabledEmail(ctx context.Context, c *ContentModerationConfig, v *ContentModerationLog) error {
	return s.emailService.SendAccountDisabledEmail(ctx, riskPolicy(c), riskLog(v))
}

func (s *ContentModerationService) sendCyberAccountDisabledEmail(ctx context.Context, c *ContentModerationConfig, v *ContentModerationCyberWarning) error {
	return s.emailService.SendCyberAccountDisabledEmail(ctx, riskPolicy(c), riskWarning(v))
}
