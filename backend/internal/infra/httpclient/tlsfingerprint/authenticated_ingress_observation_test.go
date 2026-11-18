package tlsfingerprint

import (
	"crypto/sha256"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const sha256HexLength = sha256.Size * 2

func TestBuildAuthenticatedIngressFlowIDCanonicalizesAddresses(t *testing.T) {
	t.Parallel()
	secret := strings.Repeat("s", 32)
	plain, err := BuildAuthenticatedIngressFlowID(secret, "192.0.2.10", 52000, "203.0.113.20", 443)
	require.NoError(t, err)
	mapped, err := BuildAuthenticatedIngressFlowID(secret, "[::ffff:192.0.2.10]", 52000, "::ffff:203.0.113.20", 443)
	require.NoError(t, err)
	require.Equal(t, plain, mapped)
	require.Len(t, plain, sha256HexLength)
	require.NotContains(t, plain, "192.0.2.10")
}

func TestBuildAuthenticatedIngressFlowIDRejectsUnsafeInput(t *testing.T) {
	t.Parallel()
	_, err := BuildAuthenticatedIngressFlowID("short", "192.0.2.10", 52000, "203.0.113.20", 443)
	require.ErrorContains(t, err, "at least 32 bytes")
	_, err = BuildAuthenticatedIngressFlowID(strings.Repeat("s", 32), "not-an-ip", 52000, "203.0.113.20", 443)
	require.ErrorContains(t, err, "invalid source address")
	_, err = BuildAuthenticatedIngressFlowID(strings.Repeat("s", 32), "192.0.2.10", 0, "203.0.113.20", 443)
	require.ErrorContains(t, err, "ports must be non-zero")
}

func TestAuthenticatedIngressObservationJSONOmitsTransportTuple(t *testing.T) {
	t.Parallel()
	payload := AuthenticatedIngressObservation{
		SchemaVersion: AuthenticatedIngressObservationSchemaVersion,
		FlowID:        strings.Repeat("a", sha256HexLength),
		ObservedAt:    time.Unix(1, 0).UTC(),
		ServerName:    "api.scienceedu.me",
		ClientHello:   &CapturedClientHello{JA3Hash: "candidate"},
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "source")
	require.NotContains(t, string(raw), "destination")
	require.NotContains(t, string(raw), "raw_packet")
	require.NotContains(t, string(raw), "source_port")
}
