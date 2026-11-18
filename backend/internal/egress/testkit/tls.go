package testkit

import (
	"time"

	"github.com/TokenFlux/TokenRouter/internal/egress"
)

// ApprovedTLSProfile 创建带验证证据的路由测试模板。
func ApprovedTLSProfile(value *egress.TLSFingerprintProfile) *egress.TLSFingerprintProfile {
	result := egress.CloneTLSFingerprintProfile(value)
	now := time.Now()
	result.Verification = egress.TLSFingerprintProfileVerification{SourceKind: egress.TLSFingerprintProfileSourceControlledConnect, ValidationStatus: egress.TLSFingerprintProfileValidationVerified, ValidatorVersion: egress.TLSFingerprintProfileValidatorVersion, ApprovedForUse: true, ValidatedAt: &now, ObservedJA3Hash: "fixture", ReplayedJA3Hash: "fixture", CaptureTarget: "chatgpt.com", ValidatedTargets: []egress.TLSFingerprintTargetValidation{{Host: "chatgpt.com", Success: true}}}
	result.Verification.FingerprintSHA256 = result.FingerprintSHA256()
	return result
}
