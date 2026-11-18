package tlsfingerprint

import (
	"errors"
	"fmt"
	"strings"
)

// ExtractCapturedClientHelloServerName 从已捕获的 TLS records 提取首个 host_name SNI。
// 调用方只应用它做即时目标过滤，不应把主机名以外的原始扩展载荷持久化。
func ExtractCapturedClientHelloServerName(records []byte) (string, error) {
	if _, err := ParseCapturedClientHelloRecords(records); err != nil {
		return "", err
	}
	body, err := authenticatedIngressClientHelloBody(records)
	if err != nil {
		return "", err
	}

	offset := 0
	if !skip(body, &offset, 2+32) {
		return "", errors.New("invalid ClientHello fixed fields")
	}
	sessionIDLength, ok := readUint8(body, &offset)
	if !ok || !skip(body, &offset, int(sessionIDLength)) {
		return "", errors.New("invalid ClientHello session id")
	}
	cipherLength, ok := readUint16(body, &offset)
	if !ok || !skip(body, &offset, int(cipherLength)) {
		return "", errors.New("invalid ClientHello cipher suites")
	}
	compressionLength, ok := readUint8(body, &offset)
	if !ok || !skip(body, &offset, int(compressionLength)) {
		return "", errors.New("invalid ClientHello compression methods")
	}
	if offset == len(body) {
		return "", errors.New("ClientHello does not contain SNI")
	}
	extensionsLength, ok := readUint16(body, &offset)
	if !ok || offset+int(extensionsLength) != len(body) {
		return "", errors.New("invalid ClientHello extensions")
	}
	end := offset + int(extensionsLength)
	for offset < end {
		extensionType, ok := readUint16(body, &offset)
		if !ok {
			return "", errors.New("invalid ClientHello extension type")
		}
		extensionLength, ok := readUint16(body, &offset)
		if !ok || offset+int(extensionLength) > end {
			return "", errors.New("invalid ClientHello extension payload")
		}
		extensionData := body[offset : offset+int(extensionLength)]
		offset += int(extensionLength)
		if extensionType != 0 {
			continue
		}
		return extractHostNameFromSNI(extensionData)
	}
	return "", errors.New("ClientHello does not contain SNI")
}

func authenticatedIngressClientHelloBody(records []byte) ([]byte, error) {
	handshake := make([]byte, 0, minInt(len(records), hardMaxClientHelloPayloadBytes+tlsHandshakeHeaderLen))
	for offset, count := 0, 0; offset < len(records) && count < hardMaxClientHelloRecords; count++ {
		if len(records)-offset < tlsRecordHeaderLength {
			return nil, fmt.Errorf("%w: TLS record header", ErrIncompleteClientHello)
		}
		recordLength := int(records[offset+3])<<8 | int(records[offset+4])
		payloadStart := offset + tlsRecordHeaderLength
		payloadEnd := payloadStart + recordLength
		if records[offset] != tlsRecordTypeHandshake || recordLength <= 0 || payloadEnd > len(records) {
			return nil, errors.New("invalid TLS record while extracting SNI")
		}
		handshake = append(handshake, records[payloadStart:payloadEnd]...)
		if len(handshake) >= tlsHandshakeHeaderLen {
			declaredLength := int(handshake[1])<<16 | int(handshake[2])<<8 | int(handshake[3])
			if len(handshake) >= tlsHandshakeHeaderLen+declaredLength {
				return handshake[tlsHandshakeHeaderLen : tlsHandshakeHeaderLen+declaredLength], nil
			}
		}
		offset = payloadEnd
	}
	return nil, fmt.Errorf("%w: ClientHello body", ErrIncompleteClientHello)
}

func extractHostNameFromSNI(data []byte) (string, error) {
	if err := validateSNIStructure(data); err != nil {
		return "", err
	}
	offset := 2
	for offset < len(data) {
		nameType := data[offset]
		offset++
		nameLength := int(data[offset])<<8 | int(data[offset+1])
		offset += 2
		name := data[offset : offset+nameLength]
		offset += nameLength
		if nameType != 0 {
			continue
		}
		for _, value := range name {
			if value == 0 || value > 0x7f {
				return "", errors.New("SNI host_name must be non-empty ASCII without NUL")
			}
		}
		host := strings.ToLower(string(name))
		if host == "" {
			return "", errors.New("SNI host_name is empty")
		}
		return host, nil
	}
	return "", errors.New("SNI does not contain a host_name entry")
}
