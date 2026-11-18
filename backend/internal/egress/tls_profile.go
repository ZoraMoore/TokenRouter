package egress

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

const (
	// TLSFingerprintProfileValidatorVersion 变化时历史验证证据必须重新生成。
	TLSFingerprintProfileValidatorVersion = 1

	// TLSFingerprintProfileSourceManual 表示模板由管理员手工或 YAML 创建。
	TLSFingerprintProfileSourceManual = "manual"
	// TLSFingerprintProfileSourceControlledConnect 表示模板来自受控的目标专用 CONNECT 采集。
	TLSFingerprintProfileSourceControlledConnect = "controlled_connect"
	// TLSFingerprintProfileSourceAuthenticatedIngress 表示模板来自已认证 API 请求的同连接入站采集。
	TLSFingerprintProfileSourceAuthenticatedIngress = "authenticated_ingress"
	// TLSFingerprintProfileSourceLegacy 表示迁移前创建且没有可验证来源的历史模板。
	TLSFingerprintProfileSourceLegacy = "legacy"

	// TLSFingerprintProfileValidationDraft 表示模板尚未完成回放与目标握手验证。
	TLSFingerprintProfileValidationDraft = "draft"
	// TLSFingerprintProfileValidationVerified 表示模板通过本地回放和目标 TLS-only 握手。
	TLSFingerprintProfileValidationVerified = "verified"
	// TLSFingerprintProfileValidationRejected 表示模板无法由当前构造器安全表达。
	TLSFingerprintProfileValidationRejected = "rejected"
	// TLSFingerprintProfileValidationStale 表示握手字段在验证后被修改。
	TLSFingerprintProfileValidationStale = "stale"
)

// TLSFingerprintTargetValidation 保存一次无凭据 TLS-only 目标握手结果。
// 这里只记录协商元数据，不保存目标响应、认证头或业务请求。
type TLSFingerprintTargetValidation struct {
	Host              string    `json:"host"`
	Success           bool      `json:"success"`
	NegotiatedVersion uint16    `json:"negotiated_version,omitempty"`
	CipherSuite       uint16    `json:"cipher_suite,omitempty"`
	NegotiatedALPN    string    `json:"negotiated_alpn,omitempty"`
	Error             string    `json:"error,omitempty"`
	ValidatedAt       time.Time `json:"validated_at"`
}

// TLSFingerprintProfileVerification 描述模板的可信来源和验证证据。
// 匿名公网旁路样本不得写成 controlled_connect，也不得直接进入 verified 状态。
type TLSFingerprintProfileVerification struct {
	SourceKind             string                           `json:"source_kind"`
	ClientKind             string                           `json:"client_kind,omitempty"`
	ClientVersion          string                           `json:"client_version,omitempty"`
	Runtime                string                           `json:"runtime,omitempty"`
	RuntimeVersion         string                           `json:"runtime_version,omitempty"`
	OperatingSystem        string                           `json:"operating_system,omitempty"`
	Architecture           string                           `json:"architecture,omitempty"`
	CaptureTarget          string                           `json:"capture_target,omitempty"`
	ObservedHost           string                           `json:"observed_host,omitempty"`
	IndependentConnections int                              `json:"independent_connections,omitempty"`
	ClassifierEvidence     []string                         `json:"classifier_evidence,omitempty"`
	ApprovedForUse         bool                             `json:"approved_for_use"`
	RandomPoolEligible     bool                             `json:"random_pool_eligible"`
	ValidationStatus       string                           `json:"validation_status"`
	ValidatorVersion       int                              `json:"validator_version"`
	ObservedJA3Hash        string                           `json:"observed_ja3_hash,omitempty"`
	ReplayedJA3Hash        string                           `json:"replayed_ja3_hash,omitempty"`
	FingerprintSHA256      string                           `json:"fingerprint_sha256,omitempty"`
	Issues                 []string                         `json:"issues,omitempty"`
	Warnings               []string                         `json:"warnings,omitempty"`
	ValidatedTargets       []TLSFingerprintTargetValidation `json:"validated_targets,omitempty"`
	ValidatedAt            *time.Time                       `json:"validated_at,omitempty"`
}

// TLSFingerprintProfile TLS 指纹配置模板
// 模板包含模拟客户端 TLS 握手的 ClientHello 参数。
type TLSFingerprintProfile struct {
	ID                      int64                             `json:"id"`
	Name                    string                            `json:"name"`
	Description             *string                           `json:"description"`
	EnableGREASE            bool                              `json:"enable_grease"`
	CipherSuites            []uint16                          `json:"cipher_suites"`
	Curves                  []uint16                          `json:"curves"`
	PointFormats            []uint16                          `json:"point_formats"`
	SignatureAlgorithms     []uint16                          `json:"signature_algorithms"`
	SignatureAlgorithmsCert []uint16                          `json:"signature_algorithms_cert"`
	ALPNProtocols           []string                          `json:"alpn_protocols"`
	SupportedVersions       []uint16                          `json:"supported_versions"`
	KeyShareGroups          []uint16                          `json:"key_share_groups"`
	PSKModes                []uint16                          `json:"psk_modes"`
	Extensions              []uint16                          `json:"extensions"`
	Verification            TLSFingerprintProfileVerification `json:"verification"`
	CreatedAt               time.Time                         `json:"created_at"`
	UpdatedAt               time.Time                         `json:"updated_at"`
}

// Validate 检查模板字段和验证证据。
func (p *TLSFingerprintProfile) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return &ValidationError{Field: "name", Message: "name is required"}
	}
	if p.Verification.ValidationStatus == TLSFingerprintProfileValidationVerified {
		if p.Verification.RandomPoolEligible && !p.Verification.ApprovedForUse {
			return &ValidationError{Field: "verification", Message: "random pool eligibility requires administrator approval"}
		}
		switch p.Verification.SourceKind {
		case TLSFingerprintProfileSourceControlledConnect:
		case TLSFingerprintProfileSourceAuthenticatedIngress:
			if strings.TrimSpace(p.Verification.ObservedHost) == "" {
				return &ValidationError{Field: "verification", Message: "authenticated ingress profiles must record the observed host"}
			}
			if p.Verification.IndependentConnections < 3 {
				return &ValidationError{Field: "verification", Message: "authenticated ingress profiles require at least three independent connections"}
			}
			if strings.TrimSpace(p.Verification.ClientKind) == "" || strings.TrimSpace(p.Verification.ClientVersion) == "" {
				return &ValidationError{Field: "verification", Message: "authenticated ingress profiles require classified client kind and version"}
			}
		default:
			return &ValidationError{Field: "verification", Message: "verified profiles must come from a controlled capture source"}
		}
		if p.Verification.ValidatorVersion != TLSFingerprintProfileValidatorVersion {
			return &ValidationError{Field: "verification", Message: "verified profile uses a stale validator version"}
		}
		if p.Verification.ValidatedAt == nil || len(p.Verification.Issues) > 0 {
			return &ValidationError{Field: "verification", Message: "verified profile must have a successful validation report"}
		}
		if p.Verification.FingerprintSHA256 == "" || p.Verification.FingerprintSHA256 != p.FingerprintSHA256() {
			return &ValidationError{Field: "verification", Message: "verified profile fingerprint does not match its TLS fields"}
		}
		if p.Verification.ObservedJA3Hash == "" || p.Verification.ObservedJA3Hash != p.Verification.ReplayedJA3Hash {
			return &ValidationError{Field: "verification", Message: "verified profile must have a matching replayed JA3 hash"}
		}
		if !hasSuccessfulTLSFingerprintTargetValidation(p.Verification.ValidatedTargets, p.Verification.CaptureTarget) {
			return &ValidationError{Field: "verification", Message: "verified profile must pass a TLS-only handshake for its capture target"}
		}
	}
	return nil
}

// FingerprintSHA256 返回仅由握手模板字段决定的稳定摘要。
// 名称、说明和验证元数据不参与摘要，修改这些展示字段不会使验证失效。
func (p *TLSFingerprintProfile) FingerprintSHA256() string {
	if p == nil {
		return ""
	}
	payload := struct {
		EnableGREASE            bool     `json:"enable_grease"`
		CipherSuites            []uint16 `json:"cipher_suites"`
		Curves                  []uint16 `json:"curves"`
		PointFormats            []uint16 `json:"point_formats"`
		SignatureAlgorithms     []uint16 `json:"signature_algorithms"`
		SignatureAlgorithmsCert []uint16 `json:"signature_algorithms_cert"`
		ALPNProtocols           []string `json:"alpn_protocols"`
		SupportedVersions       []uint16 `json:"supported_versions"`
		KeyShareGroups          []uint16 `json:"key_share_groups"`
		PSKModes                []uint16 `json:"psk_modes"`
		Extensions              []uint16 `json:"extensions"`
	}{
		EnableGREASE:            p.EnableGREASE,
		CipherSuites:            nonNilUint16s(p.CipherSuites),
		Curves:                  nonNilUint16s(p.Curves),
		PointFormats:            nonNilUint16s(p.PointFormats),
		SignatureAlgorithms:     nonNilUint16s(p.SignatureAlgorithms),
		SignatureAlgorithmsCert: nonNilUint16s(p.SignatureAlgorithmsCert),
		ALPNProtocols:           nonNilStrings(p.ALPNProtocols),
		SupportedVersions:       nonNilUint16s(p.SupportedVersions),
		KeyShareGroups:          nonNilUint16s(p.KeyShareGroups),
		PSKModes:                nonNilUint16s(p.PSKModes),
		Extensions:              nonNilUint16s(p.Extensions),
	}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// IsVerified 判断模板证据是否完整有效，不包含管理员发布批准语义。
func (p *TLSFingerprintProfile) IsVerified() bool {
	if p == nil || p.Verification.ValidationStatus != TLSFingerprintProfileValidationVerified {
		return false
	}
	return p.Validate() == nil
}

// IsVerifiedForTarget 判断模板证据是否覆盖指定目标，不包含管理员发布批准语义。
func (p *TLSFingerprintProfile) IsVerifiedForTarget(target string) bool {
	return p.IsVerified() && hasSuccessfulTLSFingerprintTargetValidation(p.Verification.ValidatedTargets, target)
}

// IsOutboundReady 判断自定义模板是否已验证并由管理员批准进入运行时出站路径。
func (p *TLSFingerprintProfile) IsOutboundReady() bool {
	return p.IsVerified() && p.Verification.ApprovedForUse
}

// IsOutboundReadyForTarget 额外要求模板已在相同目标完成 TLS-only 握手。
func (p *TLSFingerprintProfile) IsOutboundReadyForTarget(target string) bool {
	return p.IsOutboundReady() && p.IsVerifiedForTarget(target)
}

// MarkTLSFieldsChanged 在握手字段发生编辑后使旧验证证据失效。
func (p *TLSFingerprintProfile) MarkTLSFieldsChanged() {
	if p == nil {
		return
	}
	if p.Verification.SourceKind == "" {
		p.Verification.SourceKind = TLSFingerprintProfileSourceManual
	}
	if p.Verification.ValidationStatus == TLSFingerprintProfileValidationVerified {
		p.Verification.ValidationStatus = TLSFingerprintProfileValidationStale
	} else {
		p.Verification.ValidationStatus = TLSFingerprintProfileValidationDraft
	}
	p.Verification.FingerprintSHA256 = ""
	p.Verification.ReplayedJA3Hash = ""
	p.Verification.ValidatedTargets = nil
	p.Verification.ValidatedAt = nil
	p.Verification.Issues = []string{"TLS profile fields changed after validation"}
}

// NormalizeTLSFingerprintProfileVerification 补齐历史或手工模板的安全默认状态。
func NormalizeTLSFingerprintProfileVerification(v TLSFingerprintProfileVerification, newManual bool) TLSFingerprintProfileVerification {
	if strings.TrimSpace(v.SourceKind) == "" {
		if newManual {
			v.SourceKind = TLSFingerprintProfileSourceManual
		} else {
			v.SourceKind = TLSFingerprintProfileSourceLegacy
		}
	}
	if strings.TrimSpace(v.ValidationStatus) == "" {
		v.ValidationStatus = TLSFingerprintProfileValidationDraft
	}
	if v.Issues == nil {
		v.Issues = []string{}
	}
	if v.Warnings == nil {
		v.Warnings = []string{}
	}
	if v.ValidatedTargets == nil {
		v.ValidatedTargets = []TLSFingerprintTargetValidation{}
	}
	if v.ClassifierEvidence == nil {
		v.ClassifierEvidence = []string{}
	}
	return v
}

func hasSuccessfulTLSFingerprintTargetValidation(values []TLSFingerprintTargetValidation, target string) bool {
	target = strings.TrimSpace(strings.ToLower(target))
	if target == "" {
		return false
	}
	for _, value := range values {
		if value.Success && strings.EqualFold(strings.TrimSpace(value.Host), target) {
			return true
		}
	}
	return false
}

func nonNilUint16s(values []uint16) []uint16 {
	if values == nil {
		return []uint16{}
	}
	return values
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
