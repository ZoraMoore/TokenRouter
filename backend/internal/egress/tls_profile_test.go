package egress

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTLSFingerprintProfileFingerprintSHA256IgnoresDisplayFields(t *testing.T) {
	profile := &TLSFingerprintProfile{
		Name:                "first",
		CipherSuites:        []uint16{0x1301, 0x1302},
		SignatureAlgorithms: []uint16{0x0403},
		Extensions:          []uint16{0, 13, 43},
	}
	first := profile.FingerprintSHA256()
	profile.Name = "renamed"
	description := "changed"
	profile.Description = &description
	require.Equal(t, first, profile.FingerprintSHA256())
	profile.CipherSuites = append(profile.CipherSuites, 0x1303)
	require.NotEqual(t, first, profile.FingerprintSHA256())
}

func TestTLSFingerprintProfileVerifiedRequiresControlledMatchingEvidence(t *testing.T) {
	now := time.Now()
	profile := &TLSFingerprintProfile{Name: "verified"}
	profile.Verification = TLSFingerprintProfileVerification{
		SourceKind:       TLSFingerprintProfileSourceControlledConnect,
		CaptureTarget:    "chatgpt.com",
		ValidationStatus: TLSFingerprintProfileValidationVerified,
		ValidatorVersion: TLSFingerprintProfileValidatorVersion,
		ObservedJA3Hash:  "same",
		ReplayedJA3Hash:  "same",
		ApprovedForUse:   true,
		ValidatedAt:      &now,
		ValidatedTargets: []TLSFingerprintTargetValidation{{
			Host:        "chatgpt.com",
			Success:     true,
			ValidatedAt: now,
		}},
	}
	profile.Verification.FingerprintSHA256 = profile.FingerprintSHA256()
	require.NoError(t, profile.Validate())
	require.True(t, profile.IsOutboundReady())
	require.True(t, profile.IsOutboundReadyForTarget("chatgpt.com"))
	require.False(t, profile.IsOutboundReadyForTarget("api.openai.com"))

	profile.Verification.SourceKind = TLSFingerprintProfileSourceManual
	require.Error(t, profile.Validate())
	require.False(t, profile.IsOutboundReady())
}

func TestTLSFingerprintProfileVerifiedAcceptsAuthenticatedIngressEvidence(t *testing.T) {
	now := time.Now()
	profile := &TLSFingerprintProfile{Name: "authenticated-codex"}
	profile.Verification = TLSFingerprintProfileVerification{
		SourceKind:             TLSFingerprintProfileSourceAuthenticatedIngress,
		ClientKind:             "codex",
		ClientVersion:          "0.200.1",
		CaptureTarget:          "chatgpt.com",
		ObservedHost:           "api.scienceedu.me",
		IndependentConnections: 3,
		ValidationStatus:       TLSFingerprintProfileValidationVerified,
		ValidatorVersion:       TLSFingerprintProfileValidatorVersion,
		ObservedJA3Hash:        "same",
		ReplayedJA3Hash:        "same",
		ValidatedAt:            &now,
		ValidatedTargets: []TLSFingerprintTargetValidation{{
			Host:        "chatgpt.com",
			Success:     true,
			ValidatedAt: now,
		}},
	}
	profile.Verification.FingerprintSHA256 = profile.FingerprintSHA256()
	require.NoError(t, profile.Validate())
	require.True(t, profile.IsVerifiedForTarget("chatgpt.com"))

	profile.Verification.IndependentConnections = 2
	require.ErrorContains(t, profile.Validate(), "three independent")
}

func TestTLSFingerprintProfileMarkTLSFieldsChangedMakesVerificationStale(t *testing.T) {
	profile := &TLSFingerprintProfile{Name: "profile", Verification: TLSFingerprintProfileVerification{
		SourceKind:        TLSFingerprintProfileSourceControlledConnect,
		ValidationStatus:  TLSFingerprintProfileValidationVerified,
		FingerprintSHA256: "old",
	}}
	profile.MarkTLSFieldsChanged()
	require.Equal(t, TLSFingerprintProfileValidationStale, profile.Verification.ValidationStatus)
	require.Empty(t, profile.Verification.FingerprintSHA256)
	require.NotEmpty(t, profile.Verification.Issues)
}
