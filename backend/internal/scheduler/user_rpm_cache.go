package scheduler

import "context"

// UserRPMCache 按用户或（用户、分组）累计 RPM，同一用户的多个 API Key 共用计数。
// key 为 rpm:ug:{userID}:{groupID}:{min} 或 rpm:u:{userID}:{min}。
// 提供商级计数由 RPMCache 管理，key 为 rpm:{providerID}:{min}。
type UserRPMCache interface {
	// IncrementUserGroupRPM 原子递增 (user, group) 级分钟计数并返回最新值。
	// 用于分组 rpm_limit 与 user-group rpm_override 两种命中分支。
	IncrementUserGroupRPM(ctx context.Context, userID, groupID int64) (count int, err error)

	// IncrementUserRPM 原子递增用户级分钟计数并返回最新值。
	// 用于通过分组检查后的用户总限额检查。
	IncrementUserRPM(ctx context.Context, userID int64) (count int, err error)

	// GetUserGroupRPM 读取 (user, group) 当前分钟已用 RPM。
	GetUserGroupRPM(ctx context.Context, userID, groupID int64) (count int, err error)

	// GetUserRPM 读取用户当前分钟已用 RPM。
	GetUserRPM(ctx context.Context, userID int64) (count int, err error)
}
