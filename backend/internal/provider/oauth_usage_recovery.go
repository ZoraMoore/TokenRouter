package provider

import (
	"context"
	"errors"
	"strings"
)

// UsageRecoveryVersion 限定本轮成功查询可恢复的身份及原错误，不代表任意管理员恢复操作。
type UsageRecoveryVersion struct {
	CredentialVersion
	ErrorMessage string `json:"-"`
}
type UsageRecoveryWriter interface {
	ClearUsageErrorIfUnchanged(context.Context, UsageRecoveryVersion) (bool, error)
}

// RecoverUsageProviderError 在可恢复错误的条件写入成功后更新返回数据。
func RecoverUsageProviderError(ctx context.Context, value *Record, writer UsageRecoveryWriter) (bool, error) {
	if value == nil || value.Status != StatusError {
		return false, nil
	}
	msg := strings.ToLower(strings.TrimSpace(value.ErrorMessage))
	if msg == "" {
		return false, nil
	}
	if !strings.Contains(msg, "token refresh failed") && !strings.Contains(msg, "invalid_client") && !strings.Contains(msg, "missing_project_id") && !strings.Contains(msg, "unauthenticated") {
		return false, nil
	}
	if writer == nil {
		return false, errors.New("usage recovery conditional writer is not configured")
	}
	version := UsageRecoveryVersion{CredentialVersion: FailureVersion(value).CredentialVersion, ErrorMessage: value.ErrorMessage}
	applied, err := writer.ClearUsageErrorIfUnchanged(ctx, version)
	if err != nil {
		return false, err
	}
	if applied {
		value.Status = StatusActive
		value.ErrorMessage = ""
	}
	return applied, nil
}
