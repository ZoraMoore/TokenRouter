package tlsfingerprint

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractCapturedClientHelloServerName(t *testing.T) {
	t.Parallel()
	record := buildServerNameTestClientHello("API.ScienceEdu.Me")
	got, err := ExtractCapturedClientHelloServerName(record)
	require.NoError(t, err)
	require.Equal(t, "api.scienceedu.me", got)
}

func TestExtractCapturedClientHelloServerNameAcrossRecords(t *testing.T) {
	t.Parallel()
	record := buildServerNameTestClientHello("api.scienceedu.me")
	payload := record[tlsRecordHeaderLength:]
	split := len(payload) / 2
	fragmented := appendTLSRecord(nil, record[1:3], payload[:split])
	fragmented = appendTLSRecord(fragmented, record[1:3], payload[split:])
	got, err := ExtractCapturedClientHelloServerName(fragmented)
	require.NoError(t, err)
	require.Equal(t, "api.scienceedu.me", got)
}

func buildServerNameTestClientHello(serverName string) []byte {
	name := []byte(serverName)
	sni := make([]byte, 5+len(name))
	binary.BigEndian.PutUint16(sni[0:2], uint16(3+len(name)))
	sni[2] = 0
	binary.BigEndian.PutUint16(sni[3:5], uint16(len(name)))
	copy(sni[5:], name)

	extensions := make([]byte, 4+len(sni))
	binary.BigEndian.PutUint16(extensions[0:2], 0)
	binary.BigEndian.PutUint16(extensions[2:4], uint16(len(sni)))
	copy(extensions[4:], sni)

	body := make([]byte, 0, 64+len(extensions))
	body = append(body, 0x03, 0x03)
	body = append(body, make([]byte, 32)...)
	body = append(body, 0) // session id length
	body = append(body, 0, 2, 0x13, 0x01)
	body = append(body, 1, 0)
	body = append(body, byte(len(extensions)>>8), byte(len(extensions)))
	body = append(body, extensions...)
	handshake := []byte{tlsHandshakeClientHello, byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body))}
	handshake = append(handshake, body...)
	return appendTLSRecord(nil, []byte{0x03, 0x01}, handshake)
}

func appendTLSRecord(destination, version, payload []byte) []byte {
	destination = append(destination, tlsRecordTypeHandshake, version[0], version[1], byte(len(payload)>>8), byte(len(payload)))
	return append(destination, payload...)
}
