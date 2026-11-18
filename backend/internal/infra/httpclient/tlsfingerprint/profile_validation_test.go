package tlsfingerprint

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseCapturedClientHelloRecordsReassemblesBoundedFragments(t *testing.T) {
	original := buildSensitiveClientHelloRecord(t)
	handshake := original[tlsRecordHeaderLength:]
	fragmented := frameHandshakeWithChunks(handshake, 37)

	captured, err := ParseCapturedClientHelloRecords(fragmented)
	require.NoError(t, err)
	require.Len(t, captured.RecordVersions, 2)
	require.Equal(t, []uint16{37, uint16(len(handshake) - 37)}, captured.RecordLengths)
	require.Equal(t, uint16(0x0301), captured.RecordVersion)
	require.Equal(t, uint16(0x0304), captured.HandshakeVersion)
	require.Equal(t, uint16(0x0303), captured.LegacyVersion)
	require.Equal(t, uint8(len("SECRET_SESSION")), captured.SessionIDLength)
	require.Equal(t, []uint8{0}, captured.CompressionMethods)
	require.Equal(t, uint16(len("SECRET_TICKET")), captured.SessionTicketLength)
	require.Equal(t, []CapturedKeyShare{{Group: 29, KeyExchangeLength: uint16(len("SECRET_KEY"))}}, captured.KeyShares)
	require.True(t, captured.HasPreSharedKey)
	require.NotZero(t, captured.PreSharedKeyExtensionLength)
}

func TestParseCapturedClientHelloRecordsIncompleteIsDetectable(t *testing.T) {
	record := buildSensitiveClientHelloRecord(t)
	_, err := ParseCapturedClientHelloRecords(record[:len(record)-1])
	require.ErrorIs(t, err, ErrIncompleteClientHello)

	malformed := append([]byte(nil), record...)
	malformed[tlsRecordHeaderLength] = 2
	_, err = ParseCapturedClientHelloRecords(malformed)
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrIncompleteClientHello))
}

func TestParseCapturedClientHelloRecordsHonorsReducedRecordLimit(t *testing.T) {
	original := buildSensitiveClientHelloRecord(t)
	handshake := original[tlsRecordHeaderLength:]
	fragmented := frameHandshakeWithChunks(handshake, 37)
	limits := DefaultClientHelloParseLimits()
	limits.MaxRecords = 1

	_, err := ParseCapturedClientHelloRecordsWithLimits(fragmented, limits)
	require.ErrorContains(t, err, "more than 1 TLS records")
}

func TestCapturedClientHelloJSONDoesNotRetainSensitiveBytes(t *testing.T) {
	captured, err := ParseCapturedClientHello(buildSensitiveClientHelloRecord(t))
	require.NoError(t, err)

	payload, err := json.Marshal(captured)
	require.NoError(t, err)
	serialized := string(payload)
	for _, secret := range []string{"SECRET_RANDOM", "SECRET_SESSION", "SECRET_TICKET", "SECRET_KEY", "SECRET_PSK_ID"} {
		require.NotContains(t, serialized, secret)
	}
	require.Contains(t, serialized, `"session_ticket_length":13`)
	require.Contains(t, serialized, `"has_pre_shared_key":true`)
}

func TestValidateCapturedProfileRoundTrip(t *testing.T) {
	profile := replayableTestProfile()
	records, err := marshalProfileClientHello(profile, "capture.example")
	require.NoError(t, err)
	captured, err := ParseCapturedClientHelloRecords(records)
	require.NoError(t, err)

	report := ValidateCapturedProfile(captured, profile)
	require.Truef(t, report.Reusable, "report: %+v", report)
	require.True(t, report.ExactJA3)
	require.True(t, report.ExactStructure)
	require.Equal(t, captured.JA3Hash, report.ReplayedJA3Hash)
	require.Empty(t, report.Issues)
	require.Equal(t, []uint16{0x0804, 0x0403}, captured.SignatureAlgorithmsCert)

	_, err = json.Marshal(report)
	require.NoError(t, err)
}

func TestDefaultProfileBuildsAndReplaysLocally(t *testing.T) {
	records, err := marshalProfileClientHello(nil, "capture.example")
	require.NoError(t, err)
	captured, err := ParseCapturedClientHelloRecords(records)
	require.NoError(t, err)
	require.Contains(t, captured.Extensions, uint16(65037))

	report := ValidateCapturedProfile(captured, nil)
	require.Truef(t, report.ExactJA3, "report: %+v", report)
	require.Truef(t, report.Reusable, "report: %+v", report)
	require.NotEmpty(t, report.ReplayedJA3Hash)
}

func TestValidateCapturedProfileFailsClosedForStatefulOrUnsupportedData(t *testing.T) {
	profile := replayableTestProfile()
	records, err := marshalProfileClientHello(profile, "capture.example")
	require.NoError(t, err)
	captured, err := ParseCapturedClientHelloRecords(records)
	require.NoError(t, err)

	captured.SessionTicketLength = 8
	captured.HasPreSharedKey = true
	report := ValidateCapturedProfile(captured, profile)
	require.False(t, report.Reusable)
	require.True(t, findingHasCode(report.Issues, "captured_non_empty_session_ticket"))
	require.True(t, findingHasCode(report.Issues, "captured_psk_resumption"))
}

func TestValidateProfileRejectsUnrepresentableExtensions(t *testing.T) {
	for _, extensionID := range []uint16{21, 27, 41, 17513, 17613, 1234} {
		t.Run(strconv.Itoa(int(extensionID)), func(t *testing.T) {
			err := ValidateProfile(&Profile{Extensions: []uint16{0, extensionID}})
			require.Error(t, err)
		})
	}
	require.NoError(t, ValidateProfile(&Profile{Extensions: []uint16{0, 22, 23, 35}}))
}

func TestDialerRejectsInvalidProfileBeforeTCPDial(t *testing.T) {
	called := false
	dialer := NewDialer(&Profile{Extensions: []uint16{0, 1234}}, func(context.Context, string, string) (net.Conn, error) {
		called = true
		return nil, errors.New("unexpected dial")
	})

	conn, err := dialer.DialTLSContext(context.Background(), "tcp", "example.com:443")
	require.Nil(t, conn)
	require.ErrorContains(t, err, "unknown_extension")
	require.False(t, called)
}

func TestProbeTLSHandshakeRejectsInvalidTargetWithoutDialing(t *testing.T) {
	result, err := ProbeTLSHandshake(context.Background(), nil, "example.com")
	require.Nil(t, result)
	require.ErrorContains(t, err, "expected host:port")
}

func replayableTestProfile() *Profile {
	return &Profile{
		Name:                    "local replay",
		CipherSuites:            []uint16{0x1301, 0x1302},
		Curves:                  []uint16{29, 23},
		PointFormats:            []uint16{0},
		SignatureAlgorithms:     []uint16{0x0403, 0x0804},
		SignatureAlgorithmsCert: []uint16{0x0804, 0x0403},
		ALPNProtocols:           []string{"h2", "http/1.1"},
		SupportedVersions:       []uint16{0x0304, 0x0303},
		KeyShareGroups:          []uint16{29},
		PSKModes:                []uint16{1},
		Extensions:              []uint16{0, 10, 11, 13, 50, 16, 35, 43, 45, 51},
	}
}

func buildSensitiveClientHelloRecord(t *testing.T) []byte {
	t.Helper()
	random := bytes.Repeat([]byte("SECRET_RANDOM"), 3)[:32]
	sessionID := []byte("SECRET_SESSION")
	ticket := []byte("SECRET_TICKET")
	keyExchange := []byte("SECRET_KEY")
	pskIdentity := []byte("SECRET_PSK_ID")

	body := []byte{0x03, 0x03}
	body = append(body, random...)
	body = append(body, byte(len(sessionID)))
	body = append(body, sessionID...)
	body = appendUint16ForTest(body, 2)
	body = appendUint16ForTest(body, 0x1301)
	body = append(body, 1, 0)

	extensions := make([]byte, 0)
	extensions = appendExtensionForTest(extensions, 35, ticket)
	keyShareData := appendUint16ForTest(nil, 4+len(keyExchange))
	keyShareData = appendUint16ForTest(keyShareData, 29)
	keyShareData = appendUint16ForTest(keyShareData, len(keyExchange))
	keyShareData = append(keyShareData, keyExchange...)
	extensions = appendExtensionForTest(extensions, 51, keyShareData)
	extensions = appendExtensionForTest(extensions, 43, []byte{2, 0x03, 0x04})

	identityEntry := appendUint16ForTest(nil, len(pskIdentity))
	identityEntry = append(identityEntry, pskIdentity...)
	identityEntry = append(identityEntry, 0, 0, 0, 0)
	pskData := appendUint16ForTest(nil, len(identityEntry))
	pskData = append(pskData, identityEntry...)
	binderEntry := append([]byte{32}, bytes.Repeat([]byte{'B'}, 32)...)
	pskData = appendUint16ForTest(pskData, len(binderEntry))
	pskData = append(pskData, binderEntry...)
	extensions = appendExtensionForTest(extensions, 41, pskData)
	body = appendUint16ForTest(body, len(extensions))
	body = append(body, extensions...)

	handshake := []byte{tlsHandshakeClientHello, byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body))}
	handshake = append(handshake, body...)
	return frameHandshakeWithChunks(handshake, len(handshake))
}

func frameHandshakeWithChunks(handshake []byte, firstChunk int) []byte {
	chunks := []int{firstChunk}
	if firstChunk < len(handshake) {
		chunks = append(chunks, len(handshake)-firstChunk)
	}
	result := make([]byte, 0, len(handshake)+len(chunks)*tlsRecordHeaderLength)
	offset := 0
	for _, chunkLength := range chunks {
		result = append(result, tlsRecordTypeHandshake, 0x03, 0x01, byte(chunkLength>>8), byte(chunkLength))
		result = append(result, handshake[offset:offset+chunkLength]...)
		offset += chunkLength
	}
	return result
}

func appendExtensionForTest(target []byte, extensionID uint16, payload []byte) []byte {
	target = appendUint16ForTest(target, int(extensionID))
	target = appendUint16ForTest(target, len(payload))
	return append(target, payload...)
}

func appendUint16ForTest(target []byte, value int) []byte {
	return append(target, byte(value>>8), byte(value))
}

func findingHasCode(findings []ProfileValidationFinding, code string) bool {
	for _, finding := range findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}
