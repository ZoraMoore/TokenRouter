package identity

import (
	"time"
)

// InitialAdminInput 包含安装时创建管理员的邮箱、密码、并发数和时钟。
type InitialAdminInput struct {
	Email, Password string
	Concurrency     int
	Now             func() time.Time
}

func DecideAdminBootstrap(total, admins int64) (bool, string) {
	if admins > 0 {
		return false, "admin_exists"
	}
	if total > 0 {
		return false, "users_exist_without_admin"
	}
	return true, "empty_database"
}
