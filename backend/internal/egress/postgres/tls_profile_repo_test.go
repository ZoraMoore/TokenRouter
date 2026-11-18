package postgres

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/egress"
	"github.com/TokenFlux/TokenRouter/internal/testutil/sqlite"
)

// TestTLSFingerprintProfileListExcludesUnpublishedCaptures 使用迁移 259 的元数据检查旧模板能否进入出站选择列表。
func TestTLSFingerprintProfileListExcludesUnpublishedCaptures(t *testing.T) {
	ctx := t.Context()
	client := sqlite.NewClient(t)
	_, err := client.ExecContext(ctx, "ALTER TABLE tls_fingerprint_profiles ADD COLUMN verification JSON")
	require.NoError(t, err)
	repo := NewTLSFingerprintProfileRepository(client)

	for _, fixture := range []struct {
		name     string
		metadata string
	}{
		{name: "manual"},
		{name: "legacy", metadata: "{\"source_kind\":\"legacy\",\"validation_status\":\"draft\"}"},
		{name: "pending", metadata: "{\"source_kind\":\"authenticated_ingress\",\"validation_status\":\"verified\"}"},
		{name: "published", metadata: "{\"source_kind\":\"authenticated_ingress\",\"validation_status\":\"verified\",\"approved_for_use\":true}"},
		{name: "revoked", metadata: "{\"source_kind\":\"controlled_connect\",\"validation_status\":\"verified\",\"approved_for_use\":false}"},
		{name: "stale", metadata: "{\"source_kind\":\"controlled_connect\",\"validation_status\":\"stale\",\"approved_for_use\":true}"},
		{name: "malformed", metadata: "{\"source_kind\":\"controlled_connect\",\"validation_status\":\"verified\",\"approved_for_use\":\"yes\"}"},
	} {
		profile, err := repo.Create(ctx, &egress.TLSFingerprintProfile{Name: fixture.name})
		require.NoError(t, err)
		if fixture.metadata != "" {
			_, err = client.ExecContext(ctx, "UPDATE tls_fingerprint_profiles SET verification = ? WHERE id = ?", fixture.metadata, profile.ID)
			require.NoError(t, err)
		}
	}

	profiles, err := repo.List(ctx)
	require.NoError(t, err)
	names := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		names = append(names, profile.Name)
	}
	require.Equal(t, []string{"legacy", "manual", "published"}, names)
}
