package provider

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/egress"
	"github.com/TokenFlux/TokenRouter/internal/infra/httpclient/tlsfingerprint"
)

func TestTLSFingerprintCollectorServiceLifecycleAndCapture(t *testing.T) {
	port := reserveTLSFingerprintCollectorPort(t)
	cfg := &CollectorOptions{
		Host:                 "127.0.0.1",
		Port:                 port,
		PublicBaseURL:        fmt.Sprintf("https://127.0.0.1:%d", port),
		SessionTTLSeconds:    60,
		MaxRecordsPerSession: 5,
	}
	svc := collectorFromLegacyConfig(cfg)
	ctx := context.Background()

	status, err := svc.Start(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, svc.Stop(ctx)) })
	require.True(t, status.Running)
	require.True(t, status.UsingGeneratedCert)
	require.NotEmpty(t, status.CAPEM)

	session, err := svc.CreateControlledSession(TLSFingerprintCollectorSessionRequest{})
	require.NoError(t, err)
	require.Contains(t, session.CaptureURL, "/capture/"+session.Token)
	require.Equal(t, status.CAPEM, session.CAPEM)

	client := newTLSFingerprintCollectorTestClient(t, session.CAPEM)
	for _, tc := range []struct {
		userAgent string
		wantKind  string
	}{
		{userAgent: "Claude Code/2.0.0 Node.js/24.3.0", wantKind: "claude_code"},
		{userAgent: "codex-cli/0.1.0", wantKind: "codex"},
	} {
		req, err := http.NewRequest(http.MethodPost, session.CaptureURL, nil)
		require.NoError(t, err)
		req.Header.Set("User-Agent", tc.userAgent)
		resp, err := client.Do(req)
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		require.NoError(t, resp.Body.Close())
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

		records, err := svc.ListCaptures(session.Token)
		require.NoError(t, err)
		require.NotEmpty(t, records)
		require.Equal(t, tc.wantKind, records[0].ClientKind)
		require.NotNil(t, records[0].Profile)
		require.NotEmpty(t, records[0].Profile.CipherSuites)
		require.NotEmpty(t, records[0].Profile.Extensions)
		require.Contains(t, records[0].YAML, "captured_profile:")
		require.NotEmpty(t, records[0].JA3Hash)
	}

	svc.DeleteSession(session.Token)
	_, err = svc.ListCaptures(session.Token)
	require.Error(t, err)

	require.NoError(t, svc.Stop(ctx))
	stopped := svc.Status()
	require.False(t, stopped.Running)
	require.Empty(t, stopped.CAPEM)
	require.False(t, stopped.UsingGeneratedCert)
}

func TestNormalizeTLSFingerprintCollectorSessionRequestRequiresControlledLabels(t *testing.T) {
	_, err := normalizeTLSFingerprintCollectorSessionRequest(TLSFingerprintCollectorSessionRequest{
		Mode:               tlsFingerprintCaptureModeControlledConnect,
		ClientKind:         "codex",
		ClientVersion:      "0.144.1",
		OperatingSystem:    "Ubuntu 22.04",
		Architecture:       "x86_64",
		TargetHost:         "chatgpt.com",
		ColdStartConfirmed: true,
	})
	require.NoError(t, err)

	_, err = normalizeTLSFingerprintCollectorSessionRequest(TLSFingerprintCollectorSessionRequest{
		Mode:               tlsFingerprintCaptureModeControlledConnect,
		ClientKind:         "browser",
		ClientVersion:      "1",
		OperatingSystem:    "test",
		Architecture:       "test",
		TargetHost:         "example.com",
		ColdStartConfirmed: true,
	})
	require.Error(t, err)
}

func TestTLSFingerprintProxyAuthorizationToken(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("capture:session-token"))
	require.Equal(t, "session-token", proxyAuthorizationToken("Basic "+encoded))
	require.Empty(t, proxyAuthorizationToken("Basic "+base64.StdEncoding.EncodeToString([]byte("other:session-token"))))
	require.Equal(t, "bearer-token", proxyAuthorizationToken("Bearer bearer-token"))
}

func TestTLSFingerprintControlledConnectRejectsUnapprovedTarget(t *testing.T) {
	svc := collectorFromLegacyConfig(nil)
	token := "controlled-token"
	svc.sessions[token] = &tlsFingerprintCollectorSessionState{
		token:     token,
		expiresAt: time.Now().Add(time.Minute),
		request: TLSFingerprintCollectorSessionRequest{
			Mode:       tlsFingerprintCaptureModeControlledConnect,
			TargetHost: "chatgpt.com",
		},
	}
	req := httptest.NewRequest(http.MethodConnect, "https://collector.invalid", nil)
	req.Host = "example.com:443"
	req.Header.Set("Proxy-Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	svc.handleCaptureRequest(recorder, req)
	require.Equal(t, http.StatusForbidden, recorder.Code)
}

func TestTLSFingerprintCollectorBuildVerifiedProfile(t *testing.T) {
	recordBytes := buildControlledTLSFingerprintClientHelloRecord()
	captured, err := tlsfingerprint.ParseCapturedClientHelloRecords(recordBytes)
	require.NoError(t, err)
	profile := &egress.TLSFingerprintProfile{
		Name:                "controlled",
		CipherSuites:        captured.CipherSuites,
		Curves:              captured.Curves,
		PointFormats:        captured.PointFormats,
		SignatureAlgorithms: captured.SignatureAlgorithms,
		ALPNProtocols:       captured.ALPNProtocols,
		SupportedVersions:   captured.SupportedVersions,
		KeyShareGroups:      captured.KeyShareGroups,
		PSKModes:            captured.PSKModes,
		Extensions:          captured.Extensions,
	}
	replay := tlsfingerprint.ValidateCapturedProfile(captured, ToTLSProfile(profile))
	require.Truef(t, replay.Reusable, "issues=%+v warnings=%+v", replay.Issues, replay.Warnings)

	request := TLSFingerprintCollectorSessionRequest{
		Mode:               tlsFingerprintCaptureModeControlledConnect,
		ClientKind:         "codex",
		ClientVersion:      "0.144.1",
		OperatingSystem:    "Ubuntu 22.04",
		Architecture:       "x86_64",
		TargetHost:         "chatgpt.com",
		ColdStartConfirmed: true,
	}
	svc := collectorFromLegacyConfig(nil)
	svc.probeTLSHandshake = func(_ context.Context, _ *tlsfingerprint.Profile, target string) (*tlsfingerprint.TLSHandshakeResult, error) {
		require.Equal(t, "chatgpt.com:443", target)
		return &tlsfingerprint.TLSHandshakeResult{Version: tls.VersionTLS13, CipherSuite: tls.TLS_AES_128_GCM_SHA256, ALPN: "h2"}, nil
	}
	token := "promote-token"
	state := &tlsFingerprintCollectorSessionState{token: token, expiresAt: time.Now().Add(time.Minute), request: request}
	svc.sessions[token] = state
	requestForRecord := httptest.NewRequest(http.MethodConnect, "https://collector.invalid", nil)
	requestForRecord.Host = "chatgpt.com:443"
	for index := range tlsFingerprintCollectorMinimumColdSamples {
		record := svc.buildCaptureRecord(requestForRecord, &tlsFingerprintCaptureContext{
			clientHello:  captured,
			targetHost:   "chatgpt.com",
			connectionID: fmt.Sprintf("connection-%d", index),
		}, request)
		record.ID = fmt.Sprintf("capture-%d", index)
		require.NoError(t, svc.appendCapture(token, record))
	}

	verified, err := svc.BuildVerifiedProfile(context.Background(), token, "capture-0")
	require.NoError(t, err)
	require.True(t, verified.IsOutboundReady())
	require.Equal(t, egress.TLSFingerprintProfileSourceControlledConnect, verified.Verification.SourceKind)
	require.Equal(t, "chatgpt.com", verified.Verification.CaptureTarget)
}

func TestTLSFingerprintCollectorControlledConnectCapturesWithoutForwardingHTTP(t *testing.T) {
	port := reserveTLSFingerprintCollectorPort(t)
	cfg := &CollectorOptions{
		Host:          "127.0.0.1",
		Port:          port,
		PublicBaseURL: fmt.Sprintf("https://127.0.0.1:%d", port),
	}
	svc := collectorFromLegacyConfig(cfg)
	status, err := svc.Start(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, svc.Stop(context.Background())) })
	svc.probeTLSHandshake = func(_ context.Context, _ *tlsfingerprint.Profile, target string) (*tlsfingerprint.TLSHandshakeResult, error) {
		require.Equal(t, "chatgpt.com:443", target)
		return &tlsfingerprint.TLSHandshakeResult{Version: tls.VersionTLS13, CipherSuite: tls.TLS_AES_128_GCM_SHA256, ALPN: "h2"}, nil
	}
	session, err := svc.CreateControlledSession(TLSFingerprintCollectorSessionRequest{
		Mode:               tlsFingerprintCaptureModeControlledConnect,
		ClientKind:         "codex",
		ClientVersion:      "0.144.1",
		OperatingSystem:    "Ubuntu 22.04",
		Architecture:       "x86_64",
		TargetHost:         "chatgpt.com",
		ColdStartConfirmed: true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, session.ProxyURL)

	innerProfile := &tlsfingerprint.Profile{
		Name:                "controlled-test",
		CipherSuites:        []uint16{tls.TLS_AES_128_GCM_SHA256, tls.TLS_AES_256_GCM_SHA384, tls.TLS_CHACHA20_POLY1305_SHA256},
		Curves:              []uint16{29, 23},
		SignatureAlgorithms: []uint16{0x0403, 0x0804},
		ALPNProtocols:       []string{"h2", "http/1.1"},
		SupportedVersions:   []uint16{tls.VersionTLS13, tls.VersionTLS12},
		KeyShareGroups:      []uint16{29},
		PSKModes:            []uint16{1},
		Extensions:          []uint16{0, 10, 13, 16, 43, 45, 51},
	}
	for range tlsFingerprintCollectorMinimumColdSamples {
		captureTLSFingerprintThroughHTTPSProxy(t, status.ListenAddress, session.CAPEM, session.Token, innerProfile)
	}
	records, err := svc.ListCaptures(session.Token)
	require.NoError(t, err)
	require.Len(t, records, tlsFingerprintCollectorMinimumColdSamples)
	require.True(t, records[0].PromotionEligible)
	require.Equal(t, tlsFingerprintCollectorMinimumColdSamples, records[0].MatchingSampleCount)
	require.Equal(t, "chatgpt.com", records[0].TargetHost)

	verified, err := svc.BuildVerifiedProfile(context.Background(), session.Token, records[0].ID)
	require.NoError(t, err)
	require.True(t, verified.IsOutboundReady())
}

func TestCaptureTokenFromRequest(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "base path", path: "/capture/token-1", want: "token-1"},
		{name: "client appended path", path: "/capture/token-2/v1/responses", want: "token-2"},
		{name: "unsupported path", path: "/v1/responses", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, "https://collector.local"+tc.path, nil)
			require.NoError(t, err)
			require.Equal(t, tc.want, captureTokenFromRequest(req))
		})
	}

	req, err := http.NewRequest(http.MethodPost, "https://collector.local/v1/responses?token=query-token", nil)
	require.NoError(t, err)
	req.Header.Set("X-TLS-Fingerprint-Token", "header-token")
	require.Equal(t, "query-token", captureTokenFromRequest(req))

	req, err = http.NewRequest(http.MethodPost, "https://collector.local/v1/messages", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer claude-token")
	require.Equal(t, "claude-token", captureTokenFromRequest(req))

	req, err = http.NewRequest(http.MethodPost, "https://collector.local/v1/messages", nil)
	require.NoError(t, err)
	req.Header.Set("X-Api-Key", "api-key-token")
	require.Equal(t, "api-key-token", captureTokenFromRequest(req))
}

func TestTLSFingerprintCollectorServiceSessionLimits(t *testing.T) {
	cfg := &CollectorOptions{
		SessionTTLSeconds:    1,
		MaxRecordsPerSession: 1,
	}
	svc := collectorFromLegacyConfig(cfg)
	token := "token"
	now := time.Now()
	svc.sessions[token] = &tlsFingerprintCollectorSessionState{
		token:     token,
		expiresAt: now.Add(time.Minute),
	}

	require.NoError(t, svc.appendCapture(token, &TLSFingerprintCaptureRecord{ID: "1", CapturedAt: now}))
	require.NoError(t, svc.appendCapture(token, &TLSFingerprintCaptureRecord{ID: "2", CapturedAt: now.Add(time.Second)}))
	records, err := svc.ListCaptures(token)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, "2", records[0].ID)

	svc.sessions[token].expiresAt = time.Now().Add(-time.Second)
	_, err = svc.ListCaptures(token)
	require.Error(t, err)
}

func TestTLSFingerprintCaptureConnAcceptsLargeClientHello(t *testing.T) {
	record := buildLargeTLSFingerprintClientHelloRecord(t, 8*1024)
	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() {
		_ = serverConn.Close()
		_ = clientConn.Close()
	})

	go func() {
		_, _ = clientConn.Write(record)
	}()

	captureConn := newTLSFingerprintCaptureConn(serverConn, &tls.Certificate{})
	err := captureConn.captureClientHello()
	require.NoError(t, err)
	require.NotNil(t, captureConn.captured)
	require.Contains(t, captureConn.captured.Extensions, uint16(21))
}

func reserveTLSFingerprintCollectorPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()
	addr, ok := ln.Addr().(*net.TCPAddr)
	require.True(t, ok)
	return addr.Port
}

func newTLSFingerprintCollectorTestClient(t *testing.T, caPEM string) *http.Client {
	t.Helper()
	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM([]byte(caPEM)))
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig:       &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
			ForceAttemptHTTP2:     false,
			ResponseHeaderTimeout: time.Second,
		},
		Timeout: 3 * time.Second,
	}
}

func captureTLSFingerprintThroughHTTPSProxy(t *testing.T, address, caPEM, token string, profile *tlsfingerprint.Profile) {
	t.Helper()
	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM([]byte(caPEM)))
	outer, err := tls.Dial("tcp", address, &tls.Config{
		RootCAs:    pool,
		ServerName: "127.0.0.1",
		MinVersion: tls.VersionTLS12,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = outer.Close() })
	require.NoError(t, outer.SetDeadline(time.Now().Add(3*time.Second)))
	auth := base64.StdEncoding.EncodeToString([]byte("capture:" + token))
	connectRequest := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Opaque: "chatgpt.com:443"},
		Host:   "chatgpt.com:443",
		Header: http.Header{"Proxy-Authorization": []string{"Basic " + auth}},
	}
	require.NoError(t, connectRequest.Write(outer))
	reader := bufio.NewReader(outer)
	response, err := http.ReadResponse(reader, connectRequest)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)

	tunnelReturned := false
	dialer := tlsfingerprint.NewDialer(profile, func(context.Context, string, string) (net.Conn, error) {
		if tunnelReturned {
			return nil, errors.New("tunnel already returned")
		}
		tunnelReturned = true
		return &readerConn{Conn: outer, reader: reader}, nil
	})
	_, err = dialer.DialTLSContext(context.Background(), "tcp", "chatgpt.com:443")
	require.Error(t, err, "capture-only proxy must close before any HTTP request can be sent")
}

func buildLargeTLSFingerprintClientHelloRecord(t *testing.T, paddingLen int) []byte {
	t.Helper()
	extensions := make([]byte, 0, paddingLen+64)
	extensions = appendTLSExtension(extensions, 0, buildTLSFingerprintTestSNI("collector.local"))
	extensions = appendTLSExtension(extensions, 10, []byte{0, 2, 0, 29})
	extensions = appendTLSExtension(extensions, 21, make([]byte, paddingLen))

	hello := make([]byte, 0, len(extensions)+128)
	hello = append(hello, 0x03, 0x03)
	hello = append(hello, make([]byte, 32)...)
	hello = append(hello, 0)
	hello = append(hello, 0, 2, 0x13, 0x01)
	hello = append(hello, 1, 0)
	hello = appendUint16(hello, uint16(len(extensions)))
	hello = append(hello, extensions...)

	handshake := []byte{1, byte(len(hello) >> 16), byte(len(hello) >> 8), byte(len(hello))}
	handshake = append(handshake, hello...)
	record := []byte{22, 0x03, 0x01, byte(len(handshake) >> 8), byte(len(handshake))}
	record = append(record, handshake...)
	require.Greater(t, len(record), 4096)
	return record
}

func appendTLSExtension(dst []byte, id uint16, payload []byte) []byte {
	dst = appendUint16(dst, id)
	dst = appendUint16(dst, uint16(len(payload)))
	return append(dst, payload...)
}

func buildTLSFingerprintTestSNI(host string) []byte {
	name := []byte(host)
	result := make([]byte, 0, len(name)+5)
	result = appendUint16(result, uint16(len(name)+3))
	result = append(result, 0)
	result = appendUint16(result, uint16(len(name)))
	return append(result, name...)
}

func buildControlledTLSFingerprintClientHelloRecord() []byte {
	extensions := make([]byte, 0, 128)
	extensions = appendTLSExtension(extensions, 0, buildTLSFingerprintTestSNI("chatgpt.com"))
	extensions = appendTLSExtension(extensions, 10, []byte{0, 4, 0, 29, 0, 23})
	extensions = appendTLSExtension(extensions, 13, []byte{0, 4, 0x04, 0x03, 0x08, 0x04})
	extensions = appendTLSExtension(extensions, 16, []byte{0, 12, 2, 'h', '2', 8, 'h', 't', 't', 'p', '/', '1', '.', '1'})
	extensions = appendTLSExtension(extensions, 43, []byte{4, 0x03, 0x04, 0x03, 0x03})
	extensions = appendTLSExtension(extensions, 45, []byte{1, 1})
	keyShare := []byte{0, 36, 0, 29, 0, 32}
	keyShare = append(keyShare, make([]byte, 32)...)
	extensions = appendTLSExtension(extensions, 51, keyShare)

	hello := make([]byte, 0, len(extensions)+128)
	hello = append(hello, 0x03, 0x03)
	hello = append(hello, make([]byte, 32)...)
	hello = append(hello, 32)
	hello = append(hello, make([]byte, 32)...)
	hello = appendUint16(hello, 6)
	hello = append(hello, 0x13, 0x01, 0x13, 0x02, 0x13, 0x03)
	hello = append(hello, 1, 0)
	hello = appendUint16(hello, uint16(len(extensions)))
	hello = append(hello, extensions...)

	handshake := []byte{1, byte(len(hello) >> 16), byte(len(hello) >> 8), byte(len(hello))}
	handshake = append(handshake, hello...)
	record := []byte{22, 0x03, 0x01, byte(len(handshake) >> 8), byte(len(handshake))}
	return append(record, handshake...)
}

func appendUint16(dst []byte, v uint16) []byte {
	var buf [2]byte
	binary.BigEndian.PutUint16(buf[:], v)
	return append(dst, buf[:]...)
}

func collectorFromLegacyConfig(cfg *CollectorOptions) *TLSFingerprintCollectorService {
	if cfg == nil {
		cfg = &CollectorOptions{}
	}
	return NewTLSFingerprintCollectorService(*cfg)
}
