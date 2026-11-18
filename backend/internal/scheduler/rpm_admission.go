package scheduler

import (
	"context"

	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

// RPM 拒绝错误区分分组限额和用户总限额。
var (
	ErrGroupRPMExceeded = apperror.TooManyRequests("GROUP_RPM_EXCEEDED", "group requests-per-minute limit exceeded")
	ErrUserRPMExceeded  = apperror.TooManyRequests("USER_RPM_EXCEEDED", "user requests-per-minute limit exceeded")
)

// RPMUser 保存用户 RPM 限额，override 为 nil 时按需查询用户分组覆盖值。
type RPMUser struct {
	ID                   int64
	RPMLimit             int
	UserGroupRPMOverride *int
}
type RPMGroup struct {
	ID       int64
	RPMLimit int
}
type RPMOverrides interface {
	GetRPMOverrideByUserAndGroup(context.Context, int64, int64) (*int, error)
}

// RPMAdmission 检查用户和分组的 RPM，调用方需要先通过资金检查。
type RPMAdmission struct {
	cache       UserRPMCache
	overrides   RPMOverrides
	diagnostics Diagnostics
}

func NewRPMAdmission(cache UserRPMCache, overrides RPMOverrides, diagnostics Diagnostics) *RPMAdmission {
	return &RPMAdmission{cache: cache, overrides: overrides, diagnostics: diagnostics}
}

// Check 先检查用户在分组内的 RPM，再检查用户总 RPM，任一超限即拒绝。
// 分组内优先使用用户的 rpm_override，零值表示免检；未设置覆盖值时使用 group.rpm_limit。
// user.rpm_limit 为正时，对通过分组检查的请求继续检查用户总限额。
// Redis 故障时记录 warning 并放行。
func (s *RPMAdmission) Check(ctx context.Context, user *RPMUser, group *RPMGroup) error {
	if s == nil || s.cache == nil || user == nil {
		return nil
	}

	// 分组检查优先使用 override，否则使用 group.rpm_limit。
	if group != nil {
		// 解析 override：优先从 auth cache snapshot，nil 时回退 DB。
		var override *int
		if user.UserGroupRPMOverride != nil {
			override = user.UserGroupRPMOverride
		} else if s.overrides != nil {
			dbOverride, err := s.overrides.GetRPMOverrideByUserAndGroup(ctx, user.ID, group.ID)
			if err != nil {
				s.diagnostics.printf(
					"service.billing_cache",
					"Warning: rpm override lookup failed for user=%d group=%d: %v",
					user.ID, group.ID, err,
				)
			} else {
				override = dbOverride
			}
		}

		if override != nil {
			// override 为零时跳过分组检查，随后检查用户总限额。
			if *override > 0 {
				count, incErr := s.cache.IncrementUserGroupRPM(ctx, user.ID, group.ID)
				if incErr != nil {
					s.diagnostics.printf(
						"service.billing_cache",
						"Warning: rpm increment (override) failed for user=%d group=%d: %v",
						user.ID, group.ID, incErr,
					)
					// fail-open
				} else if count > *override {
					return ErrGroupRPMExceeded
				}
			}
			// override 替代 group.rpm_limit，随后继续检查用户总限额。
		} else if group.RPMLimit > 0 {
			// 无 override，检查 group.rpm_limit。
			count, err := s.cache.IncrementUserGroupRPM(ctx, user.ID, group.ID)
			if err != nil {
				s.diagnostics.printf(
					"service.billing_cache",
					"Warning: rpm increment (group) failed for user=%d group=%d: %v",
					user.ID, group.ID, err,
				)
				// fail-open
			} else if count > group.RPMLimit {
				return ErrGroupRPMExceeded
			}
		}
	}

	// 通过分组检查后继续检查用户总限额。
	if user.RPMLimit > 0 {
		count, err := s.cache.IncrementUserRPM(ctx, user.ID)
		if err != nil {
			s.diagnostics.printf(
				"service.billing_cache",
				"Warning: rpm increment (user) failed for user=%d: %v",
				user.ID, err,
			)
			return nil // fail-open
		}
		if count > user.RPMLimit {
			return ErrUserRPMExceeded
		}
	}

	return nil
}
