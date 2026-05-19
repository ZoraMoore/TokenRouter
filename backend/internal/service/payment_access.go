package service

import (
	"context"
	"strings"
)

// CanUserAccessPayment enforces the self-service payment whitelist.
// 管理员始终可访问；普通用户只允许配置中的指定邮箱访问充值和订单入口。
func (s *PaymentService) CanUserAccessPayment(ctx context.Context, userID int64) (bool, error) {
	if s == nil || s.userRepo == nil {
		return false, nil
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return false, err
	}
	if user.IsAdmin() {
		return true, nil
	}
	if s.configService == nil {
		return false, nil
	}
	cfg, err := s.configService.GetPaymentConfig(ctx)
	if err != nil {
		return false, err
	}
	email := strings.ToLower(strings.TrimSpace(user.Email))
	for _, allowed := range cfg.AllowedEmails {
		if email != "" && email == strings.ToLower(strings.TrimSpace(allowed)) {
			return true, nil
		}
	}
	return false, nil
}
