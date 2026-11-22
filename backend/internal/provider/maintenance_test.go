package provider

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHasMaintenance401UsesAccountHealthEvidence(t *testing.T) {
	now := time.Now()
	value := &Record{Platform: PlatformOpenAI, Type: ProviderTypeOAuth, Status: StatusError}
	value.ErrorMessage = "Some unrelated request mentioned HTTP 401"
	require.False(t, HasMaintenance401(value))

	value.ErrorMessage = "Token revoked (401): credentials rejected"
	require.True(t, HasMaintenance401(value))

	value.Status = StatusActive
	value.TempUnschedulableUntil = &now
	value.TempUnschedulableReason = "OAuth 401: credentials rejected"
	require.True(t, HasMaintenance401(value))

	value.Platform = PlatformAnthropic
	require.False(t, HasMaintenance401(value))
}

func TestMaintenanceProbeOnlyConfirmsCompleteStateChange(t *testing.T) {
	sample := StateProbeResult{Verdict: "degraded", MintStatus: 200, ContinueStatus: 200, Minted: true, NewTicket: true, MintCompleted: true}
	incomplete := maintenanceProbeResult(sample)
	require.Equal(t, "inconclusive", incomplete.Outcome)

	sample.ContinueCompleted = true
	changed := maintenanceProbeResult(sample)
	require.Equal(t, "candy", changed.Stage)
	require.Empty(t, changed.Outcome)

	sample.ContinueStatus = 401
	auth := maintenanceProbeResult(sample)
	require.Equal(t, 401, auth.HTTPStatus)
	require.Equal(t, "record_401", auth.Action)
}

func TestMaintenanceCandyRequiresAnExplicitWrongAnswer(t *testing.T) {
	tests := []struct {
		name     string
		evidence PelicanEvidence
		outcome  string
		action   string
	}{
		{"pass", PelicanEvidence{Status: "passed", Answer: "21"}, "candy_passed", ""},
		{"wrong", PelicanEvidence{Status: "suspected", DetailCode: "answer_mismatch", Answer: "18"}, "suspended", "suspend"},
		{"rate_limited", PelicanEvidence{Status: "suspected", DetailCode: "answer_mismatch", Answer: "18", HTTPStatus: 429}, "inconclusive", ""},
		{"server_error", PelicanEvidence{Status: "suspected", DetailCode: "answer_mismatch", Answer: "18", HTTPStatus: 500}, "inconclusive", ""},
		{"partial", PelicanEvidence{Status: "inconclusive", DetailCode: "incomplete", Answer: "18"}, "inconclusive", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := maintenanceCandyResult(test.evidence, test.evidence.HTTPStatus)
			require.Equal(t, test.outcome, result.Outcome)
			require.Equal(t, test.action, result.Action)
		})
	}
}
