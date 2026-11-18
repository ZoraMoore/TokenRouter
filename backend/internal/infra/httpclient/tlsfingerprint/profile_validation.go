package tlsfingerprint

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"

	utls "github.com/refraction-networking/utls" //nolint:revive // uTLS 构造器统一使用 utls 标识。

	"github.com/TokenFlux/TokenRouter/internal/pkg/tlscapture"
)

const localReplayRecordPayloadBytes = 16 << 10

// ProfileValidationFinding 是可稳定序列化的验证发现。
type ProfileValidationFinding = tlscapture.ProfileValidationFinding

// ProfileValidationReport 描述采集样本能否由当前 uTLS Profile 安全复现。
// ExactStructure 仅比较已采集的安全结构字段，不表示随机数或密钥字节相同。
type ProfileValidationReport = tlscapture.ProfileValidationReport

// ValidateProfile 在任何网络拨号前拒绝当前构建器无法准确表达的 Profile。
func ValidateProfile(profile *Profile) error {
	findings := collectProfileIssues(profile)
	if len(findings) == 0 {
		return nil
	}
	errs := make([]error, 0, len(findings))
	for _, finding := range findings {
		errs = append(errs, fmt.Errorf("%s: %s", finding.Code, finding.Message))
	}
	return errors.Join(errs...)
}

// ValidateCapturedProfile 在纯本地构建 uTLS ClientHello 并重新解析，整个过程不建立外部连接。
func ValidateCapturedProfile(capture *CapturedClientHello, profile *Profile) ProfileValidationReport {
	report := ProfileValidationReport{
		Issues:   make([]ProfileValidationFinding, 0),
		Warnings: make([]ProfileValidationFinding, 0),
	}
	if capture == nil {
		addValidationFinding(&report.Issues, "capture_missing", "缺少待验证的 ClientHello")
		return report
	}
	capturedJA3Raw := buildJA3Raw(capture.LegacyVersion, capture.CipherSuites, capture.Extensions, capture.Curves, capture.PointFormats)
	if capture.JA3Raw != "" && capture.JA3Raw != capturedJA3Raw {
		addValidationFinding(&report.Issues, "capture_ja3_raw_inconsistent", "样本携带的 JA3 原文与结构字段不一致")
	}
	report.CapturedJA3Hash = ja3Hash(capturedJA3Raw)
	if capture.JA3Hash != "" && capture.JA3Hash != report.CapturedJA3Hash {
		addValidationFinding(&report.Issues, "capture_ja3_inconsistent", "样本携带的 JA3 hash 与结构字段不一致")
	}

	for _, finding := range collectCaptureIssues(capture) {
		addValidationFinding(&report.Issues, finding.Code, finding.Message)
	}
	profileIssues := collectProfileIssues(profile)
	for _, finding := range profileIssues {
		addValidationFinding(&report.Issues, finding.Code, finding.Message)
	}
	if len(profileIssues) > 0 {
		return report
	}

	serverName, sniWarning := validationServerName(capture)
	if sniWarning != nil {
		addValidationFinding(&report.Warnings, sniWarning.Code, sniWarning.Message)
	}
	records, err := marshalProfileClientHello(profile, serverName)
	if err != nil {
		addValidationFinding(&report.Issues, "replay_build_failed", fmt.Sprintf("本地构建 uTLS ClientHello 失败：%v", err))
		return report
	}
	replayed, err := ParseCapturedClientHelloRecords(records)
	if err != nil {
		addValidationFinding(&report.Issues, "replay_parse_failed", fmt.Sprintf("本地 ClientHello 回读失败：%v", err))
		return report
	}
	report.ReplayedJA3Hash = replayed.JA3Hash
	report.ExactJA3 = report.CapturedJA3Hash != "" && report.CapturedJA3Hash == report.ReplayedJA3Hash
	if !report.ExactJA3 {
		addValidationFinding(&report.Issues, "ja3_mismatch", "本地回放后的 JA3 与采集样本不一致")
	}

	report.ExactStructure = compareCapturedStructure(capture, replayed, &report)
	if hasExtension(capture.Extensions, 65037) {
		addValidationFinding(&report.Warnings, "ech_payload_not_retained", "ECH 只比较扩展位置和长度，采集数据未保留且不会复用原始 ECH 载荷")
	}
	if len(capture.ExtensionMetadata) == 0 && len(capture.Extensions) > 0 {
		addValidationFinding(&report.Warnings, "extension_lengths_missing", "样本缺少扩展长度，无法验证完整结构长度")
	}
	if len(capture.RecordVersions) > 0 && !slices.Equal(capture.RecordVersions, replayed.RecordVersions) {
		addValidationFinding(&report.Warnings, "record_layout_differs", "TLS record 版本或分片不由 Profile 数据模型控制")
	}

	report.Reusable = report.ExactJA3 && report.ExactStructure && len(report.Issues) == 0
	return report
}

func collectProfileIssues(profile *Profile) []ProfileValidationFinding {
	issues := make([]ProfileValidationFinding, 0)
	extensions := effectiveExtensionOrder(profile)
	seen := make(map[uint16]struct{}, len(extensions))
	for _, extensionID := range extensions {
		if isGREASEValue(extensionID) {
			continue
		}
		if _, exists := seen[extensionID]; exists {
			issues = append(issues, ProfileValidationFinding{
				Code:    "duplicate_extension",
				Message: fmt.Sprintf("扩展 %d 重复出现，无法生成确定的 ClientHello", extensionID),
			})
			continue
		}
		seen[extensionID] = struct{}{}
		if code, message, supported := profileExtensionStatus(extensionID); !supported {
			issues = append(issues, ProfileValidationFinding{Code: code, Message: message})
		}
	}

	if profile == nil {
		return issues
	}
	for _, value := range profile.PointFormats {
		if value > 255 {
			issues = append(issues, ProfileValidationFinding{Code: "point_format_overflow", Message: "point format 超过 uint8 范围"})
			break
		}
	}
	for _, value := range profile.PSKModes {
		if value > 255 {
			issues = append(issues, ProfileValidationFinding{Code: "psk_mode_overflow", Message: "PSK mode 超过 uint8 范围"})
			break
		}
	}
	if len(profile.ALPNProtocols) > 0 {
		total := 0
		for _, protocol := range profile.ALPNProtocols {
			if protocol == "" || len(protocol) > 255 {
				issues = append(issues, ProfileValidationFinding{Code: "invalid_alpn", Message: "ALPN 协议不能为空且单项不能超过 255 字节"})
				break
			}
			total += 1 + len(protocol)
		}
		if total > 65535 {
			issues = append(issues, ProfileValidationFinding{Code: "invalid_alpn", Message: "ALPN 列表超过 TLS uint16 长度范围"})
		}
	}
	for _, version := range profile.SupportedVersions {
		if isGREASEValue(version) {
			continue
		}
		if version < utls.VersionTLS10 || version > utls.VersionTLS13 {
			issues = append(issues, ProfileValidationFinding{Code: "unsupported_tls_version", Message: fmt.Sprintf("uTLS 不支持版本 0x%04x", version)})
		}
	}

	checkConfiguredExtension := func(configured bool, extensionID uint16, code, field string) {
		if configured && !hasExtension(extensions, extensionID) {
			issues = append(issues, ProfileValidationFinding{Code: code, Message: field + " 已配置但对应扩展不在扩展顺序中"})
		}
	}
	checkConfiguredExtension(len(profile.Curves) > 0, 10, "supported_groups_not_emitted", "supported groups")
	checkConfiguredExtension(len(profile.PointFormats) > 0, 11, "point_formats_not_emitted", "point formats")
	checkConfiguredExtension(len(profile.SignatureAlgorithms) > 0, 13, "signature_algorithms_not_emitted", "signature algorithms")
	checkConfiguredExtension(len(profile.SignatureAlgorithmsCert) > 0, 50, "signature_algorithms_cert_not_emitted", "signature algorithms cert")
	checkConfiguredExtension(len(profile.ALPNProtocols) > 0, 16, "alpn_not_emitted", "ALPN")
	checkConfiguredExtension(len(profile.SupportedVersions) > 0, 43, "supported_versions_not_emitted", "supported versions")
	checkConfiguredExtension(len(profile.KeyShareGroups) > 0, 51, "key_share_not_emitted", "key share")
	checkConfiguredExtension(len(profile.PSKModes) > 0, 45, "psk_modes_not_emitted", "PSK modes")

	if hasExtension(extensions, 51) && hasExtension(extensions, 10) {
		curves := profile.Curves
		if len(curves) == 0 {
			curves = make([]uint16, len(defaultCurves))
			for index, curve := range defaultCurves {
				curves[index] = uint16(curve)
			}
		}
		shares := profile.KeyShareGroups
		if len(shares) == 0 {
			shares = []uint16{uint16(utls.X25519)}
		}
		for _, share := range shares {
			if !hasExtension(curves, share) {
				issues = append(issues, ProfileValidationFinding{
					Code:    "key_share_group_not_supported",
					Message: fmt.Sprintf("key share 组 %d 不在 supported groups 中", share),
				})
			}
		}
	}
	return issues
}

func collectCaptureIssues(capture *CapturedClientHello) []ProfileValidationFinding {
	issues := make([]ProfileValidationFinding, 0)
	seen := make(map[uint16]struct{}, len(capture.Extensions))
	for _, extensionID := range capture.Extensions {
		if isGREASEValue(extensionID) {
			continue
		}
		if _, exists := seen[extensionID]; exists {
			issues = append(issues, ProfileValidationFinding{Code: "captured_duplicate_extension", Message: fmt.Sprintf("采集样本包含重复扩展 %d", extensionID)})
			continue
		}
		seen[extensionID] = struct{}{}
		if code, message, supported := profileExtensionStatus(extensionID); !supported {
			issues = append(issues, ProfileValidationFinding{Code: "captured_" + code, Message: message})
		}
	}
	if capture.HasPreSharedKey || hasExtension(capture.Extensions, 41) {
		issues = append(issues, ProfileValidationFinding{Code: "captured_psk_resumption", Message: "采集样本包含 PSK/会话恢复状态，禁止复制 identity、binder 或 ticket"})
	}
	if capture.SessionTicketLength > 0 {
		issues = append(issues, ProfileValidationFinding{Code: "captured_non_empty_session_ticket", Message: "采集样本包含非空 session ticket，当前 Profile 不能安全复用"})
	}
	return issues
}

func profileExtensionStatus(extensionID uint16) (string, string, bool) {
	switch extensionID {
	case 0, 5, 10, 11, 13, 16, 18, 22, 23, 35, 43, 45, 50, 51, 65037, 65281:
		return "", "", true
	case 21:
		return "padding_extension_not_supported", "当前 Profile 不能表达固定 padding 长度", false
	case 27:
		return "certificate_compression_not_supported", "当前 Profile 不能表达 certificate compression 参数", false
	case 41:
		return "pre_shared_key_not_supported", "禁止从采集样本构建 pre_shared_key/会话恢复状态", false
	case 17513, 17613:
		return "application_settings_not_supported", "当前 Profile 不能表达 ALPS/application settings 载荷", false
	default:
		return "unknown_extension", fmt.Sprintf("未知扩展 %d 不能用空载荷近似", extensionID), false
	}
}

func effectiveExtensionOrder(profile *Profile) []uint16 {
	if profile != nil && len(profile.Extensions) > 0 {
		return profile.Extensions
	}
	return defaultExtensionOrder
}

func marshalProfileClientHello(profile *Profile, serverName string) ([]byte, error) {
	if err := ValidateProfile(profile); err != nil {
		return nil, err
	}
	clientConn, peerConn := net.Pipe()
	defer func() { _ = clientConn.Close() }()
	defer func() { _ = peerConn.Close() }()

	uconn := utls.UClient(clientConn, &utls.Config{
		ServerName:             serverName,
		SessionTicketsDisabled: true,
	}, utls.HelloCustom)
	if err := uconn.ApplyPreset(buildClientHelloSpecFromProfile(profile)); err != nil {
		return nil, fmt.Errorf("apply TLS preset: %w", err)
	}
	if err := uconn.BuildHandshakeStateWithoutSession(); err != nil {
		return nil, fmt.Errorf("build TLS handshake state: %w", err)
	}
	raw := append([]byte(nil), uconn.HandshakeState.Hello.Raw...)
	if len(raw) == 0 {
		return nil, errors.New("uTLS generated an empty ClientHello")
	}
	if len(raw)-tlsHandshakeHeaderLen > hardMaxClientHelloPayloadBytes {
		return nil, fmt.Errorf("generated ClientHello exceeds %d bytes", hardMaxClientHelloPayloadBytes)
	}
	return frameClientHelloRecords(raw), nil
}

func frameClientHelloRecords(handshake []byte) []byte {
	result := make([]byte, 0, len(handshake)+(len(handshake)/localReplayRecordPayloadBytes+1)*tlsRecordHeaderLength)
	for len(handshake) > 0 {
		chunkLength := minInt(len(handshake), localReplayRecordPayloadBytes)
		result = append(result,
			tlsRecordTypeHandshake,
			byte(utls.VersionTLS10>>8), byte(utls.VersionTLS10&0xff),
			byte(chunkLength>>8), byte(chunkLength),
		)
		result = append(result, handshake[:chunkLength]...)
		handshake = handshake[chunkLength:]
	}
	return result
}

func validationServerName(capture *CapturedClientHello) (string, *ProfileValidationFinding) {
	for _, extension := range capture.ExtensionMetadata {
		if extension.Type != 0 {
			continue
		}
		if extension.Length < 6 || extension.Length > 258 {
			finding := &ProfileValidationFinding{Code: "sni_length_not_replayed", Message: "SNI 扩展长度无法用合法 DNS 名称进行本地回放"}
			return "validation.invalid", finding
		}
		return dnsNameWithLength(int(extension.Length) - 5), nil
	}
	return "validation.invalid", nil
}

func dnsNameWithLength(length int) string {
	if length <= 0 {
		return "validation.invalid"
	}
	// uTLS 的 SNI 序列化只要求非空、非 IP 且无结尾点；这里只需保持采集到的长度。
	return strings.Repeat("a", length)
}

func compareCapturedStructure(capture, replayed *CapturedClientHello, report *ProfileValidationReport) bool {
	exact := true
	compare := func(equal bool, code, message string) {
		if equal {
			return
		}
		exact = false
		addValidationFinding(&report.Issues, code, message)
	}
	compare(capture.LegacyVersion == replayed.LegacyVersion, "legacy_version_mismatch", "ClientHello legacy version 不一致")
	compare(capture.HandshakeVersion == replayed.HandshakeVersion, "handshake_version_mismatch", "ClientHello 首选握手版本不一致")
	compare(capture.SessionIDLength == replayed.SessionIDLength, "session_id_length_mismatch", "session ID 长度不一致且当前 Profile 无法配置该长度")
	compare(slices.Equal(capture.CompressionMethods, replayed.CompressionMethods), "compression_methods_mismatch", "compression methods 不一致")
	compare(slices.Equal(capture.CipherSuites, replayed.CipherSuites), "cipher_suites_mismatch", "cipher suites 或顺序不一致")
	compare(slices.Equal(capture.Curves, replayed.Curves), "supported_groups_mismatch", "supported groups 或顺序不一致")
	compare(slices.Equal(capture.PointFormats, replayed.PointFormats), "point_formats_mismatch", "EC point formats 不一致")
	compare(slices.Equal(capture.SignatureAlgorithms, replayed.SignatureAlgorithms), "signature_algorithms_mismatch", "signature algorithms 或顺序不一致")
	compare(slices.Equal(capture.SignatureAlgorithmsCert, replayed.SignatureAlgorithmsCert), "signature_algorithms_cert_mismatch", "signature algorithms cert 或顺序不一致")
	compare(slices.Equal(capture.ALPNProtocols, replayed.ALPNProtocols), "alpn_mismatch", "ALPN 协议或顺序不一致")
	compare(slices.Equal(capture.SupportedVersions, replayed.SupportedVersions), "supported_versions_mismatch", "supported versions 或顺序不一致")
	compare(slices.Equal(capture.KeyShareGroups, replayed.KeyShareGroups), "key_share_groups_mismatch", "key share groups 或顺序不一致")
	compare(slices.Equal(capture.KeyShares, replayed.KeyShares), "key_share_lengths_mismatch", "key share 组或公钥长度不一致")
	compare(slices.Equal(capture.PSKModes, replayed.PSKModes), "psk_modes_mismatch", "PSK key exchange modes 不一致")
	compare(slices.Equal(capture.Extensions, replayed.Extensions), "extension_order_mismatch", "扩展类型或顺序不一致")
	if len(capture.ExtensionMetadata) > 0 {
		compare(slices.Equal(capture.ExtensionMetadata, replayed.ExtensionMetadata), "extension_lengths_mismatch", "扩展类型或载荷长度不一致")
	}
	compare(capture.HandshakeLength == replayed.HandshakeLength, "handshake_length_mismatch", "ClientHello handshake 长度不一致")
	compare(capture.ExtensionsLength == replayed.ExtensionsLength, "extensions_length_mismatch", "ClientHello extensions 总长度不一致")
	return exact
}

func hasExtension(values []uint16, target uint16) bool {
	return slices.Contains(values, target)
}

func addValidationFinding(target *[]ProfileValidationFinding, code, message string) {
	for _, finding := range *target {
		if finding.Code == code && finding.Message == message {
			return
		}
	}
	*target = append(*target, ProfileValidationFinding{Code: code, Message: message})
}

func ja3Hash(raw string) string {
	// 使用与解析器相同的 MD5 表达，便于兼容现有 JA3 数据。
	sum := md5.Sum([]byte(raw))
	return hex.EncodeToString(sum[:])
}
