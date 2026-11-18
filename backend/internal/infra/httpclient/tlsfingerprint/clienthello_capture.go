package tlsfingerprint

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/pkg/tlscapture"
)

const (
	tlsRecordTypeHandshake  = 22
	tlsHandshakeClientHello = 1
	tlsRecordHeaderLength   = 5
	tlsHandshakeHeaderLen   = 4

	// 解析器的硬上限不能由调用方放大，避免不可信采集数据造成无界内存占用。
	hardMaxClientHelloInputBytes   = 512 << 10
	hardMaxClientHelloRecords      = 32
	hardMaxTLSRecordPayloadBytes   = (1 << 14) + 2048
	hardMaxClientHelloPayloadBytes = 256 << 10
	hardMaxClientHelloExtensions   = 256
)

// ErrIncompleteClientHello 表示当前字节流仍缺少后续 TLS record。
// 调用方可以继续追加数据，并通过 errors.Is 判断是否应等待更多字节。
var ErrIncompleteClientHello = errors.New("incomplete TLS ClientHello")

// ClientHelloParseLimits 允许调用方收紧解析上限，但不能超过包内硬上限。
type ClientHelloParseLimits struct {
	MaxInputBytes         int
	MaxRecords            int
	MaxRecordPayloadBytes int
	MaxHandshakeBytes     int
	MaxExtensions         int
}

// CapturedTLSExtension 只保存扩展的类型和长度，不保存扩展原始载荷。
type CapturedTLSExtension = tlscapture.CapturedTLSExtension

// CapturedKeyShare 只保存 key share 的组和公钥长度，不保存公钥字节。
type CapturedKeyShare = tlscapture.CapturedKeyShare

// CapturedClientHello 保存从原始 ClientHello 中提取的安全模板字段。
// 它不会保留 random、session ID、ticket、PSK identity、binder 或 key share 字节。
type CapturedClientHello = tlscapture.CapturedClientHello

// DefaultClientHelloParseLimits 返回适用于公网采集数据的保守解析上限。
func DefaultClientHelloParseLimits() ClientHelloParseLimits {
	return ClientHelloParseLimits{
		MaxInputBytes:         hardMaxClientHelloInputBytes,
		MaxRecords:            hardMaxClientHelloRecords,
		MaxRecordPayloadBytes: hardMaxTLSRecordPayloadBytes,
		MaxHandshakeBytes:     hardMaxClientHelloPayloadBytes,
		MaxExtensions:         hardMaxClientHelloExtensions,
	}
}

// ParseCapturedClientHello 保留单 record 调用方的兼容入口，同时也能解析连续的多 record 数据。
func ParseCapturedClientHello(record []byte) (*CapturedClientHello, error) {
	return ParseCapturedClientHelloRecords(record)
}

// ParseCapturedClientHelloRecords 从连续 TLS records 中有界重组并解析第一个 ClientHello。
func ParseCapturedClientHelloRecords(records []byte) (*CapturedClientHello, error) {
	return ParseCapturedClientHelloRecordsWithLimits(records, DefaultClientHelloParseLimits())
}

// ParseCapturedClientHelloRecordsWithLimits 使用调用方收紧后的上限解析 ClientHello。
func ParseCapturedClientHelloRecordsWithLimits(records []byte, limits ClientHelloParseLimits) (*CapturedClientHello, error) {
	normalized, err := normalizeClientHelloParseLimits(limits)
	if err != nil {
		return nil, err
	}
	if len(records) > normalized.MaxInputBytes {
		return nil, fmt.Errorf("ClientHello input exceeds %d bytes", normalized.MaxInputBytes)
	}
	if len(records) < tlsRecordHeaderLength {
		return nil, fmt.Errorf("%w: TLS record header", ErrIncompleteClientHello)
	}

	offset := 0
	recordCount := 0
	handshake := make([]byte, 0, minInt(len(records), normalized.MaxHandshakeBytes+tlsHandshakeHeaderLen))
	recordVersions := make([]uint16, 0, 2)
	recordLengths := make([]uint16, 0, 2)
	declaredHelloLength := -1

	for offset < len(records) {
		if recordCount >= normalized.MaxRecords {
			return nil, fmt.Errorf("ClientHello spans more than %d TLS records", normalized.MaxRecords)
		}
		if len(records)-offset < tlsRecordHeaderLength {
			return nil, fmt.Errorf("%w: TLS record header", ErrIncompleteClientHello)
		}
		if records[offset] != tlsRecordTypeHandshake {
			return nil, fmt.Errorf("unexpected TLS record type %d while reading ClientHello", records[offset])
		}

		recordVersion := uint16(records[offset+1])<<8 | uint16(records[offset+2])
		recordLength := int(records[offset+3])<<8 | int(records[offset+4])
		if recordLength <= 0 {
			return nil, errors.New("empty TLS handshake record")
		}
		if recordLength > normalized.MaxRecordPayloadBytes {
			return nil, fmt.Errorf("TLS record payload exceeds %d bytes", normalized.MaxRecordPayloadBytes)
		}
		if len(records)-offset-tlsRecordHeaderLength < recordLength {
			return nil, fmt.Errorf("%w: TLS record payload", ErrIncompleteClientHello)
		}

		payloadStart := offset + tlsRecordHeaderLength
		payloadEnd := payloadStart + recordLength
		if len(handshake)+recordLength > normalized.MaxHandshakeBytes+tlsHandshakeHeaderLen {
			return nil, fmt.Errorf("ClientHello handshake exceeds %d bytes", normalized.MaxHandshakeBytes)
		}
		handshake = append(handshake, records[payloadStart:payloadEnd]...)
		recordVersions = append(recordVersions, recordVersion)
		recordLengths = append(recordLengths, uint16(recordLength))
		recordCount++
		offset = payloadEnd

		if len(handshake) > 0 && handshake[0] != tlsHandshakeClientHello {
			return nil, errors.New("TLS handshake does not start with ClientHello")
		}
		if len(handshake) >= tlsHandshakeHeaderLen && declaredHelloLength < 0 {
			declaredHelloLength = int(handshake[1])<<16 | int(handshake[2])<<8 | int(handshake[3])
			if declaredHelloLength <= 0 {
				return nil, errors.New("empty ClientHello payload")
			}
			if declaredHelloLength > normalized.MaxHandshakeBytes {
				return nil, fmt.Errorf("ClientHello payload exceeds %d bytes", normalized.MaxHandshakeBytes)
			}
		}
		if declaredHelloLength >= 0 && len(handshake) >= tlsHandshakeHeaderLen+declaredHelloLength {
			return parseClientHelloBody(
				handshake[tlsHandshakeHeaderLen:tlsHandshakeHeaderLen+declaredHelloLength],
				uint32(declaredHelloLength),
				recordVersions,
				recordLengths,
				normalized.MaxExtensions,
			)
		}
	}

	return nil, fmt.Errorf("%w: ClientHello handshake payload", ErrIncompleteClientHello)
}

func normalizeClientHelloParseLimits(limits ClientHelloParseLimits) (ClientHelloParseLimits, error) {
	defaults := DefaultClientHelloParseLimits()
	var err error
	if limits.MaxInputBytes, err = normalizeClientHelloLimit(limits.MaxInputBytes, defaults.MaxInputBytes, hardMaxClientHelloInputBytes, "input bytes"); err != nil {
		return ClientHelloParseLimits{}, err
	}
	if limits.MaxRecords, err = normalizeClientHelloLimit(limits.MaxRecords, defaults.MaxRecords, hardMaxClientHelloRecords, "records"); err != nil {
		return ClientHelloParseLimits{}, err
	}
	if limits.MaxRecordPayloadBytes, err = normalizeClientHelloLimit(limits.MaxRecordPayloadBytes, defaults.MaxRecordPayloadBytes, hardMaxTLSRecordPayloadBytes, "record payload bytes"); err != nil {
		return ClientHelloParseLimits{}, err
	}
	if limits.MaxHandshakeBytes, err = normalizeClientHelloLimit(limits.MaxHandshakeBytes, defaults.MaxHandshakeBytes, hardMaxClientHelloPayloadBytes, "handshake bytes"); err != nil {
		return ClientHelloParseLimits{}, err
	}
	if limits.MaxExtensions, err = normalizeClientHelloLimit(limits.MaxExtensions, defaults.MaxExtensions, hardMaxClientHelloExtensions, "extensions"); err != nil {
		return ClientHelloParseLimits{}, err
	}
	return limits, nil
}

func normalizeClientHelloLimit(value, defaultValue, hardMax int, name string) (int, error) {
	if value == 0 {
		return defaultValue, nil
	}
	if value < 0 || value > hardMax {
		return 0, fmt.Errorf("invalid ClientHello %s limit %d (max %d)", name, value, hardMax)
	}
	return value, nil
}

func parseClientHelloBody(data []byte, handshakeLength uint32, recordVersions, recordLengths []uint16, maxExtensions int) (*CapturedClientHello, error) {
	offset := 0
	legacyVersion, ok := readUint16(data, &offset)
	if !ok {
		return nil, errors.New("missing legacy version")
	}
	if !skip(data, &offset, 32) {
		return nil, errors.New("missing random")
	}
	sessionIDLen, ok := readUint8(data, &offset)
	if !ok || !skip(data, &offset, int(sessionIDLen)) {
		return nil, errors.New("invalid session id")
	}

	cipherSuitesLen, ok := readUint16(data, &offset)
	if !ok || cipherSuitesLen == 0 || int(cipherSuitesLen)%2 != 0 || offset+int(cipherSuitesLen) > len(data) {
		return nil, errors.New("invalid cipher suites")
	}
	cipherSuitesRaw := readUint16List(data[offset : offset+int(cipherSuitesLen)])
	offset += int(cipherSuitesLen)

	compressionLen, ok := readUint8(data, &offset)
	if !ok || compressionLen == 0 || offset+int(compressionLen) > len(data) {
		return nil, errors.New("invalid compression methods")
	}
	compressionMethods := append([]uint8(nil), data[offset:offset+int(compressionLen)]...)
	offset += int(compressionLen)

	var extensionsLength uint16
	var extensionMetadata []CapturedTLSExtension
	var extensionIDsRaw []uint16
	var curvesRaw []uint16
	var pointFormats []uint16
	var signatureAlgorithms []uint16
	var signatureAlgorithmsCert []uint16
	var alpnProtocols []string
	var supportedVersionsRaw []uint16
	var keyShareGroupsRaw []uint16
	var keyShares []CapturedKeyShare
	var pskModes []uint16
	var paddingLength uint16
	var sessionTicketLength uint16
	var hasPreSharedKey bool
	var preSharedKeyExtensionLength uint16

	if offset < len(data) {
		var ok bool
		extensionsLength, ok = readUint16(data, &offset)
		if !ok || offset+int(extensionsLength) != len(data) {
			return nil, errors.New("invalid extensions block")
		}
		end := offset + int(extensionsLength)
		seenExtensions := make(map[uint16]struct{})
		for offset < end {
			if len(extensionIDsRaw) >= maxExtensions {
				return nil, fmt.Errorf("ClientHello contains more than %d extensions", maxExtensions)
			}
			extType, ok := readUint16(data, &offset)
			if !ok {
				return nil, errors.New("invalid extension type")
			}
			extLen, ok := readUint16(data, &offset)
			if !ok || offset+int(extLen) > end {
				return nil, errors.New("invalid extension payload")
			}
			extData := data[offset : offset+int(extLen)]
			offset += int(extLen)
			if _, exists := seenExtensions[extType]; exists {
				return nil, fmt.Errorf("duplicate extension %d", extType)
			}
			seenExtensions[extType] = struct{}{}
			extensionIDsRaw = append(extensionIDsRaw, extType)
			extensionMetadata = append(extensionMetadata, CapturedTLSExtension{
				Type:   normalizeGREASEValue(extType),
				Length: extLen,
			})

			var parseErr error
			switch extType {
			case 0:
				parseErr = validateSNIStructure(extData)
			case 18, 22, 23:
				if extLen != 0 {
					parseErr = errors.New("extension must have an empty payload")
				}
			case 10:
				curvesRaw, parseErr = parseUint16VectorStrict(extData, "supported groups")
			case 11:
				pointFormats, parseErr = parsePointFormatsStrict(extData)
			case 13:
				signatureAlgorithms, parseErr = parseUint16VectorStrict(extData, "signature algorithms")
			case 16:
				alpnProtocols, parseErr = parseALPNStrict(extData)
			case 21:
				paddingLength = extLen
				for _, value := range extData {
					if value != 0 {
						parseErr = errors.New("invalid non-zero TLS padding")
						break
					}
				}
			case 35:
				sessionTicketLength = extLen
			case 41:
				hasPreSharedKey = true
				preSharedKeyExtensionLength = extLen
				parseErr = validatePreSharedKeyStructure(extData)
				if parseErr == nil && offset != end {
					parseErr = errors.New("pre_shared_key must be the final ClientHello extension")
				}
			case 43:
				supportedVersionsRaw, parseErr = parseSupportedVersionsStrict(extData)
			case 45:
				pskModes, parseErr = parsePSKModesStrict(extData)
			case 50:
				signatureAlgorithmsCert, parseErr = parseUint16VectorStrict(extData, "signature algorithms cert")
			case 51:
				keyShareGroupsRaw, keyShares, parseErr = parseKeySharesStrict(extData)
			case 65281:
				if len(extData) < 1 || int(extData[0])+1 != len(extData) {
					parseErr = errors.New("invalid renegotiation info vector")
				}
			}
			if parseErr != nil {
				return nil, fmt.Errorf("extension %d: %w", extType, parseErr)
			}
		}
	}
	if offset != len(data) {
		return nil, errors.New("trailing bytes after ClientHello")
	}

	enableGREASE := containsGREASE(cipherSuitesRaw) ||
		containsGREASE(extensionIDsRaw) ||
		containsGREASE(curvesRaw) ||
		containsGREASE(supportedVersionsRaw) ||
		containsGREASE(keyShareGroupsRaw)
	captured := &CapturedClientHello{
		RecordVersions:              append([]uint16(nil), recordVersions...),
		RecordLengths:               append([]uint16(nil), recordLengths...),
		HandshakeVersion:            preferredHandshakeVersion(legacyVersion, supportedVersionsRaw),
		LegacyVersion:               legacyVersion,
		HandshakeLength:             handshakeLength,
		SessionIDLength:             sessionIDLen,
		CompressionMethods:          compressionMethods,
		ExtensionsLength:            extensionsLength,
		ExtensionMetadata:           extensionMetadata,
		CipherSuites:                normalizeGREASEList(cipherSuitesRaw),
		Curves:                      normalizeGREASEList(curvesRaw),
		PointFormats:                pointFormats,
		SignatureAlgorithms:         signatureAlgorithms,
		SignatureAlgorithmsCert:     signatureAlgorithmsCert,
		ALPNProtocols:               alpnProtocols,
		SupportedVersions:           normalizeGREASEList(supportedVersionsRaw),
		KeyShareGroups:              normalizeGREASEList(keyShareGroupsRaw),
		KeyShares:                   normalizeGREASEKeyShares(keyShares),
		PSKModes:                    pskModes,
		Extensions:                  normalizeGREASEList(extensionIDsRaw),
		PaddingLength:               paddingLength,
		SessionTicketLength:         sessionTicketLength,
		HasPreSharedKey:             hasPreSharedKey,
		PreSharedKeyExtensionLength: preSharedKeyExtensionLength,
		EnableGREASE:                enableGREASE,
	}
	if len(recordVersions) > 0 {
		captured.RecordVersion = recordVersions[0]
	}
	captured.JA3Raw = buildJA3Raw(legacyVersion, cipherSuitesRaw, extensionIDsRaw, curvesRaw, pointFormats)
	sum := md5.Sum([]byte(captured.JA3Raw))
	captured.JA3Hash = hex.EncodeToString(sum[:])
	return captured, nil
}

func validateSNIStructure(data []byte) error {
	if len(data) < 2 {
		return errors.New("missing SNI name list")
	}
	listLength := int(data[0])<<8 | int(data[1])
	if listLength == 0 || listLength+2 != len(data) {
		return errors.New("invalid SNI name list")
	}
	offset := 2
	for offset < len(data) {
		if offset+3 > len(data) {
			return errors.New("invalid SNI name entry")
		}
		offset++ // 名称类型只参与结构校验，不保留其后真实主机名。
		nameLength := int(data[offset])<<8 | int(data[offset+1])
		offset += 2
		if nameLength == 0 || offset+nameLength > len(data) {
			return errors.New("invalid SNI name length")
		}
		offset += nameLength
	}
	return nil
}

func validatePreSharedKeyStructure(data []byte) error {
	if len(data) < 4 {
		return errors.New("missing PSK identities or binders")
	}
	offset := 0
	identitiesLength, ok := readUint16(data, &offset)
	if !ok || identitiesLength == 0 || offset+int(identitiesLength) > len(data) {
		return errors.New("invalid PSK identities vector")
	}
	identitiesEnd := offset + int(identitiesLength)
	identityCount := 0
	for offset < identitiesEnd {
		identityLength, ok := readUint16(data, &offset)
		if !ok || identityLength == 0 || offset+int(identityLength)+4 > identitiesEnd {
			return errors.New("invalid PSK identity")
		}
		offset += int(identityLength) + 4 // 跳过 identity 与 ticket age，不保存原始内容。
		identityCount++
	}
	bindersLength, ok := readUint16(data, &offset)
	if !ok || bindersLength == 0 || offset+int(bindersLength) != len(data) {
		return errors.New("invalid PSK binders vector")
	}
	bindersEnd := offset + int(bindersLength)
	binderCount := 0
	for offset < bindersEnd {
		binderLength, ok := readUint8(data, &offset)
		if !ok || binderLength == 0 || offset+int(binderLength) > bindersEnd {
			return errors.New("invalid PSK binder")
		}
		offset += int(binderLength) // 只校验长度，不保存 binder 字节。
		binderCount++
	}
	if binderCount != identityCount {
		return errors.New("PSK identity and binder counts differ")
	}
	return nil
}

func parsePointFormatsStrict(data []byte) ([]uint16, error) {
	if len(data) < 1 {
		return nil, errors.New("missing point format vector")
	}
	count := int(data[0])
	if count == 0 || 1+count != len(data) {
		return nil, errors.New("invalid point format vector")
	}
	out := make([]uint16, 0, count)
	for _, value := range data[1:] {
		out = append(out, uint16(value))
	}
	return out, nil
}

func parseALPNStrict(data []byte) ([]string, error) {
	if len(data) < 2 {
		return nil, errors.New("missing ALPN vector")
	}
	listLen := int(data[0])<<8 | int(data[1])
	if listLen == 0 || 2+listLen != len(data) {
		return nil, errors.New("invalid ALPN vector")
	}
	offset := 2
	out := make([]string, 0, 2)
	for offset < len(data) {
		nameLen := int(data[offset])
		offset++
		if nameLen == 0 || offset+nameLen > len(data) {
			return nil, errors.New("invalid ALPN protocol")
		}
		out = append(out, string(data[offset:offset+nameLen]))
		offset += nameLen
	}
	return out, nil
}

func parseSupportedVersionsStrict(data []byte) ([]uint16, error) {
	if len(data) < 1 {
		return nil, errors.New("missing supported versions vector")
	}
	vectorLength := int(data[0])
	if vectorLength == 0 || vectorLength%2 != 0 || 1+vectorLength != len(data) {
		return nil, errors.New("invalid supported versions vector")
	}
	return readUint16List(data[1:]), nil
}

func parsePSKModesStrict(data []byte) ([]uint16, error) {
	if len(data) < 1 {
		return nil, errors.New("missing PSK mode vector")
	}
	count := int(data[0])
	if count == 0 || 1+count != len(data) {
		return nil, errors.New("invalid PSK mode vector")
	}
	out := make([]uint16, 0, count)
	for _, value := range data[1:] {
		out = append(out, uint16(value))
	}
	return out, nil
}

func parseKeySharesStrict(data []byte) ([]uint16, []CapturedKeyShare, error) {
	if len(data) < 2 {
		return nil, nil, errors.New("missing key share vector")
	}
	vectorLength := int(data[0])<<8 | int(data[1])
	if vectorLength == 0 || 2+vectorLength != len(data) {
		return nil, nil, errors.New("invalid key share vector")
	}
	offset := 2
	groups := make([]uint16, 0, 2)
	shares := make([]CapturedKeyShare, 0, 2)
	for offset < len(data) {
		group, ok := readUint16(data, &offset)
		if !ok {
			return nil, nil, errors.New("invalid key share group")
		}
		keyLength, ok := readUint16(data, &offset)
		if !ok || keyLength == 0 || offset+int(keyLength) > len(data) {
			return nil, nil, errors.New("invalid key share length")
		}
		groups = append(groups, group)
		shares = append(shares, CapturedKeyShare{Group: group, KeyExchangeLength: keyLength})
		offset += int(keyLength)
	}
	return groups, shares, nil
}

func parseUint16VectorStrict(data []byte, name string) ([]uint16, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("missing %s vector", name)
	}
	vectorLength := int(data[0])<<8 | int(data[1])
	if vectorLength == 0 || vectorLength%2 != 0 || 2+vectorLength != len(data) {
		return nil, fmt.Errorf("invalid %s vector", name)
	}
	return readUint16List(data[2:]), nil
}

func readUint16List(data []byte) []uint16 {
	out := make([]uint16, 0, len(data)/2)
	for index := 0; index+1 < len(data); index += 2 {
		out = append(out, uint16(data[index])<<8|uint16(data[index+1]))
	}
	return out
}

func buildJA3Raw(version uint16, cipherSuites, extensions, curves, pointFormats []uint16) string {
	return strings.Join([]string{
		strconv.Itoa(int(version)),
		joinJA3Values(cipherSuites),
		joinJA3Values(extensions),
		joinJA3Values(curves),
		joinJA3Values(pointFormats),
	}, ",")
}

func joinJA3Values(values []uint16) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if isGREASEValue(value) {
			continue
		}
		parts = append(parts, strconv.Itoa(int(value)))
	}
	return strings.Join(parts, "-")
}

func normalizeGREASEList(values []uint16) []uint16 {
	if len(values) == 0 {
		return nil
	}
	out := make([]uint16, 0, len(values))
	for _, value := range values {
		out = append(out, normalizeGREASEValue(value))
	}
	return out
}

func normalizeGREASEKeyShares(values []CapturedKeyShare) []CapturedKeyShare {
	if len(values) == 0 {
		return nil
	}
	out := make([]CapturedKeyShare, len(values))
	for index, value := range values {
		out[index] = value
		out[index].Group = normalizeGREASEValue(value.Group)
	}
	return out
}

func normalizeGREASEValue(value uint16) uint16 {
	if isGREASEValue(value) {
		return 0x0a0a
	}
	return value
}

func containsGREASE(values []uint16) bool {
	return slices.ContainsFunc(values, isGREASEValue)
}

func preferredHandshakeVersion(legacyVersion uint16, supportedVersions []uint16) uint16 {
	for _, version := range supportedVersions {
		if !isGREASEValue(version) {
			return version
		}
	}
	return legacyVersion
}

func readUint8(data []byte, offset *int) (uint8, bool) {
	if *offset >= len(data) {
		return 0, false
	}
	value := data[*offset]
	(*offset)++
	return value, true
}

func readUint16(data []byte, offset *int) (uint16, bool) {
	if *offset+2 > len(data) {
		return 0, false
	}
	value := uint16(data[*offset])<<8 | uint16(data[*offset+1])
	*offset += 2
	return value, true
}

func skip(data []byte, offset *int, count int) bool {
	if count < 0 || *offset+count > len(data) {
		return false
	}
	*offset += count
	return true
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
