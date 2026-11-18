package bootstrap

import (
	"database/sql"
	"time"

	dbent "github.com/TokenFlux/TokenRouter/ent"
	"github.com/TokenFlux/TokenRouter/internal/config"
	"github.com/TokenFlux/TokenRouter/internal/identity"
	identitypostgres "github.com/TokenFlux/TokenRouter/internal/identity/postgres"
)

// JWTIdentity 提供维护命令需要的用户读取和访问令牌签发功能。
type JWTIdentity struct {
	Users  *identitypostgres.UserStore
	Tokens *identity.SessionService
}

// NewJWTIdentity 使用已完成引导的数据库连接，命令返回前负责关闭连接。
func NewJWTIdentity(client *dbent.Client, db *sql.DB, cfg *config.Config) JWTIdentity {
	users := identitypostgres.NewUserStore(client, db)
	options := identity.SessionOptions{Now: time.Now, Secret: cfg.JWT.Secret, ExpireHour: cfg.JWT.ExpireHour, AccessTokenExpireMinutes: cfg.JWT.AccessTokenExpireMinutes, RefreshTokenExpireDays: cfg.JWT.RefreshTokenExpireDays}
	return JWTIdentity{Users: users, Tokens: identity.NewSessionService(options, users, nil, nil, nil)}
}
