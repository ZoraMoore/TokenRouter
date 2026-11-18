package app

import (
	"context"

	"github.com/TokenFlux/TokenRouter/internal/audit"
	audithttp "github.com/TokenFlux/TokenRouter/internal/audit/httpapi"
	"github.com/TokenFlux/TokenRouter/internal/identity"
	identityhttp "github.com/TokenFlux/TokenRouter/internal/identity/httpapi"
	"github.com/TokenFlux/TokenRouter/internal/server/middleware"
)

// identityAuthAudit 将认证事件转换为审计记录，并尝试异步写入。
type identityAuthAudit struct{ audit *audit.AuditLogService }

func (a identityAuthAudit) RecordBindingMismatch(_ context.Context, event identityhttp.BindingMismatchEvent) {
	a.audit.Record(&audit.AuditLog{ActorUserID: &event.UserID, ActorEmail: event.Email, ActorRole: event.Role, AuthMethod: audit.AuditAuthMethodJWT, Action: audit.AuditActionSessionBindingMismatch, Method: event.Method, Path: event.Path, ClientIP: event.ClientIP, UserAgent: event.UserAgent, StatusCode: 401})
}

// provideJWTAuth 为 JWT 中间件绑定身份、设置和活动记录组件。
func provideJWTAuth(graph *identityAuthGraph, users *identity.UserService, settings *identity.RuntimeSettings, recorder *audit.AuditLogService) identityhttp.JWTAuthMiddleware {
	return identityhttp.JWTAuthMiddleware(identityhttp.JWTAuth(graph.Core, users, users, settings, identityAuthAudit{recorder}))
}

// provideAdminAuth 与 JWT 中间件共用身份实例。
func provideAdminAuth(graph *identityAuthGraph, users *identity.UserService, settings *identity.RuntimeSettings, recorder *audit.AuditLogService) identityhttp.AdminAuthMiddleware {
	return identityhttp.AdminAuthMiddleware(identityhttp.AdminAuth(graph.Core, users, settings, identityAuthAudit{recorder}))
}

// provideStepUpAuth 绑定会话凭据、TOTP 和动态开关。
func provideStepUpAuth(totp *identity.TotpService, users *identity.UserService, settings *identity.RuntimeSettings) identityhttp.StepUpAuthMiddleware {
	return identityhttp.StepUpAuthMiddleware(identityhttp.StepUpAuth(totp, users, settings))
}

// provideAuditMiddleware 为审计中间件注入共享的脱敏策略。
func provideAuditMiddleware(recorder *audit.AuditLogService, redactor *audit.Redactor) middleware.AuditLogMiddleware {
	return audithttp.NewAuditLogMiddleware(recorder, redactor)
}
