package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTLSFingerprintProfileVerificationMigration 校验历史模板按未验证草稿迁移。
func TestTLSFingerprintProfileVerificationMigration(t *testing.T) {
	content, err := FS.ReadFile("259_add_tls_fingerprint_profile_verification.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(content))
	require.Contains(t, sql, "add column if not exists verification jsonb not null")
	require.Contains(t, sql, "add column if not exists signature_algorithms_cert jsonb")
	require.Contains(t, sql, `"source_kind":"legacy"`)
	require.Contains(t, sql, `"validation_status":"draft"`)
}
