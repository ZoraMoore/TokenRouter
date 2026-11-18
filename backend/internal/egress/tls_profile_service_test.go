package egress

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestTLSProfileSnapshotIsolation 检查写入来源和返回对象的修改是否影响缓存。
func TestTLSProfileSnapshotIsolation(t *testing.T) {
	source := &TLSFingerprintProfile{ID: 1, Name: "profile", CipherSuites: []uint16{4865}, ALPNProtocols: []string{"h2"}}
	now := time.Now()
	source.Verification = TLSFingerprintProfileVerification{SourceKind: TLSFingerprintProfileSourceControlledConnect, ValidationStatus: TLSFingerprintProfileValidationVerified, ValidatorVersion: TLSFingerprintProfileValidatorVersion, ApprovedForUse: true, ValidatedAt: &now, ObservedJA3Hash: "fixture", ReplayedJA3Hash: "fixture", CaptureTarget: "chatgpt.com", ValidatedTargets: []TLSFingerprintTargetValidation{{Host: "chatgpt.com", Success: true}}}
	source.Verification.FingerprintSHA256 = source.FingerprintSHA256()
	svc := NewTLSFingerprintProfileService(nil, nil)
	svc.setLocalCache([]*TLSFingerprintProfile{source})
	first := svc.GetProfileByID(1)
	first.CipherSuites[0] = 4866
	first.ALPNProtocols[0] = "http/1.1"
	again := svc.GetProfileByID(1)
	require.Equal(t, uint16(4865), again.CipherSuites[0])
	require.Equal(t, "h2", again.ALPNProtocols[0])
	source.CipherSuites[0] = 4867
	require.Equal(t, uint16(4865), svc.GetProfileByID(1).CipherSuites[0])
}
