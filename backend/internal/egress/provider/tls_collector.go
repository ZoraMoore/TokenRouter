package provider

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/egress"
	"github.com/TokenFlux/TokenRouter/internal/infra/httpclient/tlsfingerprint"
	"github.com/TokenFlux/TokenRouter/internal/infra/telemetry/logging"
)

const (
	tlsFingerprintCollectorDefaultTTL        = 30 * time.Minute
	tlsFingerprintCollectorDefaultMaxRecords = 20
	// 受控采集允许 ClientHello 跨多个 record，但总量始终受硬上限约束。
	tlsFingerprintCollectorClientHelloBufferSize = 256 << 10
	tlsFingerprintCollectorMinimumColdSamples    = 3
	tlsFingerprintCollectorReadTimeout           = 10 * time.Second

	tlsFingerprintCaptureModeDirect            = "direct_base_url"
	tlsFingerprintCaptureModeControlledConnect = "controlled_connect"
)

var tlsFingerprintControlledTargets = map[string]struct{}{
	"chatgpt.com":    {},
	"api.openai.com": {},
}

// TLSFingerprintCollectorSessionRequest 描述一次管理员受控采集的可信标签。
// 普通公网旁路数据不经过该结构，也不能被提升为出站模板。
// TLSFingerprintCollectorService 管理运行时 TLS 指纹收集器。
type TLSFingerprintCollectorService struct {
	closed  bool
	options CollectorOptions

	mu                sync.Mutex
	server            *http.Server
	listener          net.Listener
	running           bool
	startedAt         *time.Time
	lastError         string
	caPEM             string
	cert              tls.Certificate
	generatedCert     bool
	sessions          map[string]*tlsFingerprintCollectorSessionState
	probeTLSHandshake func(context.Context, *tlsfingerprint.Profile, string) (*tlsfingerprint.TLSHandshakeResult, error)
}

type tlsFingerprintCollectorSessionState struct {
	token     string
	expiresAt time.Time
	records   []*TLSFingerprintCaptureRecord
	request   TLSFingerprintCollectorSessionRequest
}

type tlsFingerprintCaptureContextKey struct{}

type tlsFingerprintCaptureContext struct {
	clientHello    *tlsfingerprint.CapturedClientHello
	negotiatedALPN string
	targetHost     string
	httpProto      string
	connectionID   string
}

type tlsFingerprintCaptureListener struct {
	net.Listener
	cert *tls.Certificate
}

type tlsFingerprintCaptureConn struct {
	net.Conn
	cert           *tls.Certificate
	reader         *bufio.Reader
	captured       *tlsfingerprint.CapturedClientHello
	negotiatedALPN string
	handshakeOnce  sync.Once
	handshakeErr   error
	tlsConn        *tls.Conn
	connectionID   string
}

type readerConn struct {
	net.Conn
	reader *bufio.Reader
}

type CollectorOptions struct {
	Host                 string
	Port                 int
	PublicBaseURL        string
	CertFile             string
	KeyFile              string
	SessionTTLSeconds    int
	MaxRecordsPerSession int
	Now                  func() time.Time
	Diagnostics          egress.Diagnostics
}

type TLSFingerprintCollectorSessionRequest = egress.TLSFingerprintCollectorSessionRequest

type TLSFingerprintCollectorStatus = egress.TLSFingerprintCollectorStatus

type TLSFingerprintCollectorSession = egress.TLSFingerprintCollectorSession

type TLSFingerprintCaptureRecord = egress.TLSFingerprintCaptureRecord

// NewTLSFingerprintCollectorService 创建运行时 TLS 指纹收集器服务。
func NewTLSFingerprintCollectorService(options CollectorOptions) *TLSFingerprintCollectorService {
	return &TLSFingerprintCollectorService{options: options, sessions: make(map[string]*tlsFingerprintCollectorSessionState)}
}

func (s *TLSFingerprintCollectorService) Status() TLSFingerprintCollectorStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked()
}

// Start 启动独立 HTTPS 收集器监听。
func (s *TLSFingerprintCollectorService) Start(ctx context.Context) (TLSFingerprintCollectorStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return TLSFingerprintCollectorStatus{}, fmt.Errorf("collector is shut down")
	}
	if s.running {
		return s.statusLocked(), nil
	}

	cert, caPEM, generated, err := s.loadOrGenerateCertificateLocked()
	if err != nil {
		s.lastError = err.Error()
		return s.statusLocked(), err
	}

	listenAddr := s.listenAddressLocked()
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		s.lastError = err.Error()
		return s.statusLocked(), err
	}

	now := time.Now()
	s.cert = cert
	s.caPEM = caPEM
	s.generatedCert = generated
	s.startedAt = &now
	s.lastError = ""
	s.sessions = make(map[string]*tlsFingerprintCollectorSessionState)
	s.listener = newTLSFingerprintCaptureListener(ln, &s.cert)

	s.server = &http.Server{
		// CONNECT 的 request-target 是 authority-form，URL.Path 可能为空；
		// 直接使用 HandlerFunc，避免 ServeMux 在进入目标白名单校验前返回 404。
		Handler:           http.HandlerFunc(s.handleCaptureRequest),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       60 * time.Second,
		ConnContext: func(ctx context.Context, conn net.Conn) context.Context {
			if captureConn, ok := conn.(*tlsFingerprintCaptureConn); ok {
				return context.WithValue(ctx, tlsFingerprintCaptureContextKey{}, captureConn)
			}
			return ctx
		},
		TLSConfig: &tls.Config{
			// 收集器只需要采集客户端声明的 ALPN，实际响应固定走 HTTP/1.1，避免 h2 请求解析复杂度。
			MinVersion: tls.VersionTLS12,
			NextProtos: []string{"http/1.1"},
		},
	}
	s.running = true

	server := s.server
	listener := s.listener
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			logging.LegacyPrintf("service.tls_fp_collector", "[TLSFPCollector] serve failed: %v", serveErr)
			s.mu.Lock()
			s.running = false
			s.lastError = serveErr.Error()
			s.server = nil
			s.listener = nil
			s.mu.Unlock()
		}
	}()
	return s.statusLocked(), nil
}

// Stop 停止收集器并清空短期会话。
func (s *TLSFingerprintCollectorService) Stop(ctx context.Context) error {
	s.mu.Lock()
	server := s.server
	s.running = false
	s.server = nil
	s.listener = nil
	s.startedAt = nil
	s.caPEM = ""
	s.cert = tls.Certificate{}
	s.generatedCert = false
	s.sessions = make(map[string]*tlsFingerprintCollectorSessionState)
	s.mu.Unlock()

	if server == nil {
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return err
	}
	return nil
}

// CreateSession 创建短期采集会话。只有标签完整的 controlled_connect 会话可提升模板。
func (s *TLSFingerprintCollectorService) CreateControlledSession(request TLSFingerprintCollectorSessionRequest) (*TLSFingerprintCollectorSession, error) {
	normalized, err := normalizeTLSFingerprintCollectorSessionRequest(request)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return nil, errors.New("TLS fingerprint collector is not running")
	}
	s.pruneExpiredSessionsLocked(time.Now())
	token, err := randomHexToken(24)
	if err != nil {
		return nil, err
	}
	expiresAt := time.Now().Add(s.sessionTTLLocked())
	state := &tlsFingerprintCollectorSessionState{
		token:     token,
		expiresAt: expiresAt,
		request:   normalized,
	}
	s.sessions[token] = state
	return s.sessionDTOFromStateLocked(state), nil
}

// ListCaptures 返回指定会话的采集结果。
func (s *TLSFingerprintCollectorService) ListCaptures(token string) ([]*TLSFingerprintCaptureRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.sessionStateLocked(token)
	if err != nil {
		return nil, err
	}
	consistentConnections := make(map[string]map[string]struct{})
	for _, record := range state.records {
		if record == nil || record.Profile == nil || record.ReplayValidation == nil || !record.ReplayValidation.Reusable {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(record.TargetHost)) + ":" + record.Profile.FingerprintSHA256()
		if consistentConnections[key] == nil {
			consistentConnections[key] = make(map[string]struct{})
		}
		connectionID := strings.TrimSpace(record.ConnectionID)
		if connectionID == "" {
			connectionID = "record:" + record.ID
		}
		consistentConnections[key][connectionID] = struct{}{}
	}
	out := make([]*TLSFingerprintCaptureRecord, 0, len(state.records))
	for _, record := range state.records {
		if record == nil {
			continue
		}
		clone := *record
		clone.BlockedReasons = append([]string(nil), record.BlockedReasons...)
		key := strings.ToLower(strings.TrimSpace(record.TargetHost))
		if record.Profile != nil {
			key += ":" + record.Profile.FingerprintSHA256()
		}
		clone.MatchingSampleCount = len(consistentConnections[key])
		if clone.MatchingSampleCount < tlsFingerprintCollectorMinimumColdSamples {
			clone.PromotionEligible = false
			clone.BlockedReasons = append(clone.BlockedReasons, fmt.Sprintf(
				"at least %d matching cold-start connections are required (got %d)",
				tlsFingerprintCollectorMinimumColdSamples,
				clone.MatchingSampleCount,
			))
		}
		out = append(out, &clone)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CapturedAt.After(out[j].CapturedAt)
	})
	return out, nil
}

// BuildVerifiedProfile 从受控 CONNECT 会话构建已验证模板。
// 验证包括至少三次一致冷启动样本、本地 uTLS 回放和固定目标的无凭据 TLS-only 握手。
func (s *TLSFingerprintCollectorService) BuildVerifiedProfile(ctx context.Context, token, captureID string) (*egress.TLSFingerprintProfile, error) {
	s.mu.Lock()
	state, err := s.sessionStateLocked(token)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	request := state.request
	records := append([]*TLSFingerprintCaptureRecord(nil), state.records...)
	s.mu.Unlock()

	if request.Mode != tlsFingerprintCaptureModeControlledConnect {
		return nil, errors.New("only controlled CONNECT captures can be promoted")
	}
	var selected *TLSFingerprintCaptureRecord
	for _, record := range records {
		if record != nil && record.ID == captureID {
			selected = record
			break
		}
	}
	if selected == nil || selected.Profile == nil || selected.RawTLSFingerprint == nil {
		return nil, errors.New("capture not found")
	}
	if selected.SourceKind != egress.TLSFingerprintProfileSourceControlledConnect ||
		!strings.EqualFold(strings.TrimSpace(selected.TargetHost), strings.TrimSpace(request.TargetHost)) {
		return nil, errors.New("capture provenance does not match the controlled session")
	}
	replay := tlsfingerprint.ValidateCapturedProfile(selected.RawTLSFingerprint, ToTLSProfile(selected.Profile))
	if !replay.Reusable {
		return nil, fmt.Errorf("capture is not reproducible: %s", joinTLSFingerprintValidationFindings(replay.Issues))
	}

	fingerprintHash := selected.Profile.FingerprintSHA256()
	consistentConnections := make(map[string]struct{})
	for _, record := range records {
		if record == nil || record.Profile == nil || !strings.EqualFold(record.TargetHost, request.TargetHost) {
			continue
		}
		if record.Profile.FingerprintSHA256() == fingerprintHash && record.ReplayValidation != nil && record.ReplayValidation.Reusable {
			connectionID := strings.TrimSpace(record.ConnectionID)
			if connectionID == "" {
				connectionID = "record:" + record.ID
			}
			consistentConnections[connectionID] = struct{}{}
		}
	}
	consistentSamples := len(consistentConnections)
	if consistentSamples < tlsFingerprintCollectorMinimumColdSamples {
		return nil, fmt.Errorf("at least %d matching cold-start captures are required (got %d)", tlsFingerprintCollectorMinimumColdSamples, consistentSamples)
	}

	targetHost, err := normalizeTLSFingerprintTargetHost(request.TargetHost)
	if err != nil {
		return nil, err
	}
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	probe, probeErr := s.runTLSFingerprintHandshakeProbe(probeCtx, ToTLSProfile(selected.Profile), net.JoinHostPort(targetHost, "443"))
	now := time.Now()
	targetValidation := egress.TLSFingerprintTargetValidation{
		Host:        targetHost,
		Success:     probeErr == nil,
		ValidatedAt: now,
	}
	if probeErr != nil {
		targetValidation.Error = truncateTLSFingerprintCollectorError(probeErr.Error())
		return nil, fmt.Errorf("TLS-only target handshake failed: %w", probeErr)
	}
	targetValidation.NegotiatedVersion = probe.Version
	targetValidation.CipherSuite = probe.CipherSuite
	targetValidation.NegotiatedALPN = probe.ALPN

	profile := cloneTLSFingerprintProfile(selected.Profile)
	profile.Verification = egress.TLSFingerprintProfileVerification{
		SourceKind:         egress.TLSFingerprintProfileSourceControlledConnect,
		ClientKind:         request.ClientKind,
		ClientVersion:      request.ClientVersion,
		Runtime:            request.Runtime,
		RuntimeVersion:     request.RuntimeVersion,
		OperatingSystem:    request.OperatingSystem,
		Architecture:       request.Architecture,
		CaptureTarget:      targetHost,
		ApprovedForUse:     true,
		RandomPoolEligible: true,
		ValidationStatus:   egress.TLSFingerprintProfileValidationVerified,
		ValidatorVersion:   egress.TLSFingerprintProfileValidatorVersion,
		ObservedJA3Hash:    replay.CapturedJA3Hash,
		ReplayedJA3Hash:    replay.ReplayedJA3Hash,
		FingerprintSHA256:  fingerprintHash,
		Issues:             tlsFingerprintValidationFindingMessages(replay.Issues),
		Warnings:           tlsFingerprintValidationFindingMessages(replay.Warnings),
		ValidatedTargets:   []egress.TLSFingerprintTargetValidation{targetValidation},
		ValidatedAt:        &now,
	}
	if err := profile.Validate(); err != nil {
		return nil, err
	}
	return profile, nil
}

// DeleteSession 删除指定采集会话。
func (s *TLSFingerprintCollectorService) DeleteSession(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
}

func (s *TLSFingerprintCollectorService) handleCaptureRequest(w http.ResponseWriter, r *http.Request) {
	token := captureTokenFromRequest(r)
	if token == "" {
		writeCollectorJSON(w, http.StatusUnauthorized, map[string]any{"error": "missing token"})
		return
	}
	if r.Method == http.MethodConnect {
		s.handleControlledConnectCapture(w, r, token)
		return
	}
	captureConn, _ := r.Context().Value(tlsFingerprintCaptureContextKey{}).(*tlsFingerprintCaptureConn)
	if captureConn == nil {
		writeCollectorJSON(w, http.StatusBadRequest, map[string]any{"error": "missing tls connection"})
		return
	}
	captureCtx := captureConn.captureContext()
	if captureCtx == nil || captureCtx.clientHello == nil {
		writeCollectorJSON(w, http.StatusBadRequest, map[string]any{"error": "missing tls fingerprint"})
		return
	}

	request, err := s.sessionRequest(token)
	if err != nil {
		writeCollectorJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}
	if request.Mode == tlsFingerprintCaptureModeControlledConnect {
		writeCollectorJSON(w, http.StatusBadRequest, map[string]any{"error": "controlled capture requires an HTTPS CONNECT proxy request"})
		return
	}
	record := s.buildCaptureRecord(r, captureCtx, request)
	if err := s.appendCapture(token, record); err != nil {
		writeCollectorJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}
	writeCollectorJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"captured_id": record.ID,
		"message":     "TLS fingerprint captured",
	})
}

// handleControlledConnectCapture 只读取 CONNECT 隧道内面向固定目标的 ClientHello。
// 它不会连接目标、完成内层 TLS 或转发任何 HTTP/认证数据，因此不是通用代理。
func (s *TLSFingerprintCollectorService) handleControlledConnectCapture(w http.ResponseWriter, r *http.Request, token string) {
	request, err := s.sessionRequest(token)
	if err != nil {
		writeCollectorJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}
	if request.Mode != tlsFingerprintCaptureModeControlledConnect {
		writeCollectorJSON(w, http.StatusForbidden, map[string]any{"error": "capture session does not allow CONNECT"})
		return
	}
	targetHost, err := normalizeTLSFingerprintConnectTarget(r.Host)
	if err != nil || !strings.EqualFold(targetHost, request.TargetHost) {
		writeCollectorJSON(w, http.StatusForbidden, map[string]any{"error": "CONNECT target is not allowed for this session"})
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		writeCollectorJSON(w, http.StatusInternalServerError, map[string]any{"error": "CONNECT capture is unavailable"})
		return
	}
	conn, readWriter, err := hijacker.Hijack()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(tlsFingerprintCollectorReadTimeout))
	if _, err := readWriter.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	if err := readWriter.Flush(); err != nil {
		return
	}

	records, err := readTLSFingerprintClientHelloRecords(readWriter.Reader, tlsFingerprintCollectorClientHelloBufferSize)
	if err != nil {
		logging.LegacyPrintf("service.tls_fp_collector", "[TLSFPCollector] controlled CONNECT capture failed target=%s: %v", targetHost, err)
		return
	}
	clientHello, err := tlsfingerprint.ParseCapturedClientHelloRecords(records)
	if err != nil {
		logging.LegacyPrintf("service.tls_fp_collector", "[TLSFPCollector] controlled CONNECT parse failed target=%s: %v", targetHost, err)
		return
	}
	connectionID := ""
	if captureConn, ok := r.Context().Value(tlsFingerprintCaptureContextKey{}).(*tlsFingerprintCaptureConn); ok && captureConn != nil {
		connectionID = captureConn.connectionID
	}
	record := s.buildCaptureRecord(r, &tlsFingerprintCaptureContext{
		clientHello:  clientHello,
		targetHost:   targetHost,
		httpProto:    r.Proto,
		connectionID: connectionID,
	}, request)
	if err := s.appendCapture(token, record); err != nil {
		logging.LegacyPrintf("service.tls_fp_collector", "[TLSFPCollector] controlled CONNECT session append failed: %v", err)
	}
}

func readTLSFingerprintClientHelloRecords(reader *bufio.Reader, maxBytes int) ([]byte, error) {
	if reader == nil {
		return nil, errors.New("missing CONNECT tunnel reader")
	}
	if maxBytes <= 0 || maxBytes > tlsFingerprintCollectorClientHelloBufferSize {
		maxBytes = tlsFingerprintCollectorClientHelloBufferSize
	}
	var records bytes.Buffer
	for range 32 {
		var header [5]byte
		if _, err := io.ReadFull(reader, header[:]); err != nil {
			return nil, fmt.Errorf("read TLS record header: %w", err)
		}
		recordLength := int(header[3])<<8 | int(header[4])
		if recordLength <= 0 || records.Len()+len(header)+recordLength > maxBytes {
			return nil, errors.New("controlled ClientHello exceeds capture limit")
		}
		_, _ = records.Write(header[:])
		payload := make([]byte, recordLength)
		if _, err := io.ReadFull(reader, payload); err != nil {
			return nil, fmt.Errorf("read TLS record payload: %w", err)
		}
		_, _ = records.Write(payload)
		if _, err := tlsfingerprint.ParseCapturedClientHelloRecords(records.Bytes()); err == nil {
			return append([]byte(nil), records.Bytes()...), nil
		} else if !errors.Is(err, tlsfingerprint.ErrIncompleteClientHello) {
			return nil, err
		}
	}
	return nil, errors.New("controlled ClientHello spans too many TLS records")
}

func (s *TLSFingerprintCollectorService) appendCapture(token string, record *TLSFingerprintCaptureRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.sessionStateLocked(token)
	if err != nil {
		return err
	}
	maxRecords := s.maxRecordsLocked()
	state.records = append([]*TLSFingerprintCaptureRecord{record}, state.records...)
	if len(state.records) > maxRecords {
		state.records = state.records[:maxRecords]
	}
	return nil
}

func (s *TLSFingerprintCollectorService) sessionRequest(token string) (TLSFingerprintCollectorSessionRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.sessionStateLocked(token)
	if err != nil {
		return TLSFingerprintCollectorSessionRequest{}, err
	}
	return state.request, nil
}

func (s *TLSFingerprintCollectorService) buildCaptureRecord(r *http.Request, captureCtx *tlsFingerprintCaptureContext, sessionRequest TLSFingerprintCollectorSessionRequest) *TLSFingerprintCaptureRecord {
	now := time.Now()
	ch := captureCtx.clientHello
	detectedClientKind := detectTLSFingerprintClientKind(r)
	clientKind := strings.TrimSpace(sessionRequest.ClientKind)
	if clientKind == "" {
		clientKind = detectedClientKind
	}
	sourceKind := egress.TLSFingerprintProfileSourceLegacy
	if sessionRequest.Mode == tlsFingerprintCaptureModeControlledConnect {
		sourceKind = egress.TLSFingerprintProfileSourceControlledConnect
	}
	profile := &egress.TLSFingerprintProfile{
		Name:                    defaultTLSFingerprintProfileName(clientKind, now),
		Description:             tlsFingerprintCollectorStringPtr("由内置 TLS 指纹收集器采集"),
		EnableGREASE:            ch.EnableGREASE,
		CipherSuites:            ch.CipherSuites,
		Curves:                  ch.Curves,
		PointFormats:            ch.PointFormats,
		SignatureAlgorithms:     ch.SignatureAlgorithms,
		SignatureAlgorithmsCert: ch.SignatureAlgorithmsCert,
		ALPNProtocols:           ch.ALPNProtocols,
		SupportedVersions:       ch.SupportedVersions,
		KeyShareGroups:          ch.KeyShareGroups,
		PSKModes:                ch.PSKModes,
		Extensions:              ch.Extensions,
		Verification: egress.TLSFingerprintProfileVerification{
			SourceKind:       sourceKind,
			ClientKind:       clientKind,
			ClientVersion:    sessionRequest.ClientVersion,
			Runtime:          sessionRequest.Runtime,
			RuntimeVersion:   sessionRequest.RuntimeVersion,
			OperatingSystem:  sessionRequest.OperatingSystem,
			Architecture:     sessionRequest.Architecture,
			CaptureTarget:    sessionRequest.TargetHost,
			ValidationStatus: egress.TLSFingerprintProfileValidationDraft,
		},
	}
	replayValidation := tlsfingerprint.ValidateCapturedProfile(ch, ToTLSProfile(profile))
	blockedReasons := tlsFingerprintValidationFindingMessages(replayValidation.Issues)
	if sessionRequest.Mode != tlsFingerprintCaptureModeControlledConnect {
		blockedReasons = append(blockedReasons, "capture must use controlled_connect mode")
	}
	if !sessionRequest.ColdStartConfirmed {
		blockedReasons = append(blockedReasons, "capture must be confirmed as a cold start")
	}
	if strings.TrimSpace(sessionRequest.ClientKind) == "" || strings.TrimSpace(sessionRequest.ClientVersion) == "" || strings.TrimSpace(sessionRequest.OperatingSystem) == "" || strings.TrimSpace(sessionRequest.Architecture) == "" {
		blockedReasons = append(blockedReasons, "controlled client labels are incomplete")
	}
	targetHost := strings.TrimSpace(captureCtx.targetHost)
	if targetHost == "" {
		targetHost = strings.TrimSpace(sessionRequest.TargetHost)
	}
	record := &TLSFingerprintCaptureRecord{
		ID:                 fmt.Sprintf("%d", now.UnixNano()),
		CapturedAt:         now,
		ClientKind:         clientKind,
		DetectedClientKind: detectedClientKind,
		SourceKind:         sourceKind,
		CaptureMode:        sessionRequest.Mode,
		TargetHost:         targetHost,
		PromotionEligible:  len(blockedReasons) == 0 && replayValidation.Reusable,
		BlockedReasons:     blockedReasons,
		ConnectionID:       captureCtx.connectionID,
		RequestPath:        r.URL.Path,
		Method:             r.Method,
		UserAgent:          r.UserAgent(),
		JA3Raw:             ch.JA3Raw,
		JA3Hash:            ch.JA3Hash,
		NegotiatedALPN:     captureCtx.negotiatedALPN,
		HTTPProto:          firstNonEmptyCollectorString(captureCtx.httpProto, r.Proto),
		Profile:            profile,
		YAML:               tlsFingerprintProfileToYAML(profile),
		HeadersSummary:     summarizeTLSFingerprintHeaders(r.Header),
		StainlessSummary:   summarizeStainlessHeaders(r.Header),
		RawTLSFingerprint:  ch,
		ReplayValidation:   &replayValidation,
	}
	return record
}

func (s *TLSFingerprintCollectorService) sessionStateLocked(token string) (*tlsFingerprintCollectorSessionState, error) {
	s.pruneExpiredSessionsLocked(time.Now())
	state := s.sessions[token]
	if state == nil {
		return nil, errors.New("capture session not found or expired")
	}
	return state, nil
}

func (s *TLSFingerprintCollectorService) pruneExpiredSessionsLocked(now time.Time) {
	for token, state := range s.sessions {
		if now.After(state.expiresAt) {
			delete(s.sessions, token)
		}
	}
}

func (s *TLSFingerprintCollectorService) sessionDTOFromStateLocked(state *tlsFingerprintCollectorSessionState) *TLSFingerprintCollectorSession {
	return &TLSFingerprintCollectorSession{
		Token:              state.token,
		ExpiresAt:          state.expiresAt,
		CaptureURL:         s.captureURLLocked(state.token),
		ProxyURL:           s.proxyURLLocked(state.token),
		Mode:               state.request.Mode,
		ClientKind:         state.request.ClientKind,
		ClientVersion:      state.request.ClientVersion,
		Runtime:            state.request.Runtime,
		RuntimeVersion:     state.request.RuntimeVersion,
		OperatingSystem:    state.request.OperatingSystem,
		Architecture:       state.request.Architecture,
		TargetHost:         state.request.TargetHost,
		ColdStartConfirmed: state.request.ColdStartConfirmed,
		MinimumColdSamples: tlsFingerprintCollectorMinimumColdSamples,
		CAPEM:              s.caPEM,
	}
}

func (s *TLSFingerprintCollectorService) statusLocked() TLSFingerprintCollectorStatus {
	return TLSFingerprintCollectorStatus{
		Running:              s.running,
		ListenAddress:        s.listenAddressLocked(),
		PublicBaseURL:        s.publicBaseURLLocked(),
		UsingGeneratedCert:   s.generatedCert,
		CAPEM:                s.caPEM,
		SessionTTLSeconds:    int(s.sessionTTLLocked().Seconds()),
		MaxRecordsPerSession: s.maxRecordsLocked(),
		StartedAt:            s.startedAt,
		LastError:            s.lastError,
	}
}

func (s *TLSFingerprintCollectorService) loadOrGenerateCertificateLocked() (tls.Certificate, string, bool, error) {
	collectorCfg := s.collectorConfigLocked()
	certFile := strings.TrimSpace(collectorCfg.CertFile)
	keyFile := strings.TrimSpace(collectorCfg.KeyFile)
	if certFile != "" || keyFile != "" {
		if certFile == "" || keyFile == "" {
			return tls.Certificate{}, "", false, errors.New("cert_file and key_file must be configured together")
		}
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return tls.Certificate{}, "", false, err
		}
		return cert, "", false, nil
	}
	cert, caPEM, err := generateTLSFingerprintCollectorCertificate(s.publicHostLocked())
	return cert, caPEM, true, err
}

func (s *TLSFingerprintCollectorService) collectorConfigLocked() CollectorOptions { return s.options }

func (s *TLSFingerprintCollectorService) listenAddressLocked() string {
	collectorCfg := s.collectorConfigLocked()
	host := strings.TrimSpace(collectorCfg.Host)
	if host == "" {
		host = "0.0.0.0"
	}
	port := collectorCfg.Port
	if port <= 0 {
		port = 8443
	}
	return net.JoinHostPort(host, strconvItoa(port))
}

func (s *TLSFingerprintCollectorService) publicBaseURLLocked() string {
	collectorCfg := s.collectorConfigLocked()
	if raw := strings.TrimRight(strings.TrimSpace(collectorCfg.PublicBaseURL), "/"); raw != "" {
		return raw
	}
	host := s.publicHostLocked()
	port := collectorCfg.Port
	if port <= 0 {
		port = 8443
	}
	return fmt.Sprintf("https://%s", net.JoinHostPort(host, strconvItoa(port)))
}

func (s *TLSFingerprintCollectorService) publicHostLocked() string {
	collectorCfg := s.collectorConfigLocked()
	if raw := strings.TrimSpace(collectorCfg.PublicBaseURL); raw != "" {
		if host := hostFromURL(raw); host != "" {
			return host
		}
	}
	host := strings.TrimSpace(collectorCfg.Host)
	if host == "" || host == "0.0.0.0" || host == "::" {
		return "localhost"
	}
	return host
}

func (s *TLSFingerprintCollectorService) captureURLLocked(token string) string {
	return s.publicBaseURLLocked() + "/capture/" + token
}

func (s *TLSFingerprintCollectorService) proxyURLLocked(token string) string {
	parsed, err := url.Parse(s.publicBaseURLLocked())
	if err != nil || parsed == nil {
		return ""
	}
	parsed.Path = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.User = url.UserPassword("capture", token)
	return parsed.String()
}

func (s *TLSFingerprintCollectorService) sessionTTLLocked() time.Duration {
	seconds := s.collectorConfigLocked().SessionTTLSeconds
	if seconds <= 0 {
		return tlsFingerprintCollectorDefaultTTL
	}
	return time.Duration(seconds) * time.Second
}

func (s *TLSFingerprintCollectorService) maxRecordsLocked() int {
	maxRecords := s.collectorConfigLocked().MaxRecordsPerSession
	if maxRecords <= 0 {
		return tlsFingerprintCollectorDefaultMaxRecords
	}
	return maxRecords
}

func newTLSFingerprintCaptureListener(inner net.Listener, cert *tls.Certificate) net.Listener {
	return &tlsFingerprintCaptureListener{Listener: inner, cert: cert}
}

func (l *tlsFingerprintCaptureListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return newTLSFingerprintCaptureConn(conn, l.cert), nil
}

func newTLSFingerprintCaptureConn(conn net.Conn, cert *tls.Certificate) *tlsFingerprintCaptureConn {
	connectionID, err := randomHexToken(12)
	if err != nil {
		connectionID = fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return &tlsFingerprintCaptureConn{
		Conn:         conn,
		cert:         cert,
		reader:       bufio.NewReaderSize(conn, tlsFingerprintCollectorClientHelloBufferSize),
		connectionID: connectionID,
	}
}

func (c *tlsFingerprintCaptureConn) Read(p []byte) (int, error) {
	if err := c.ensureHandshake(); err != nil {
		return 0, err
	}
	return c.tlsConn.Read(p)
}

func (c *tlsFingerprintCaptureConn) Write(p []byte) (int, error) {
	if err := c.ensureHandshake(); err != nil {
		return 0, err
	}
	return c.tlsConn.Write(p)
}

func (c *tlsFingerprintCaptureConn) Close() error {
	if c.tlsConn != nil {
		return c.tlsConn.Close()
	}
	return c.Conn.Close()
}

func (c *tlsFingerprintCaptureConn) ensureHandshake() error {
	c.handshakeOnce.Do(func() {
		if err := c.captureClientHello(); err != nil {
			c.handshakeErr = err
			return
		}
		tlsConn := tls.Server(&readerConn{Conn: c.Conn, reader: c.reader}, &tls.Config{
			Certificates: []tls.Certificate{*c.cert},
			MinVersion:   tls.VersionTLS12,
			NextProtos:   []string{"http/1.1"},
		})
		if err := tlsConn.Handshake(); err != nil {
			c.handshakeErr = err
			return
		}
		c.tlsConn = tlsConn
		c.negotiatedALPN = tlsConn.ConnectionState().NegotiatedProtocol
	})
	return c.handshakeErr
}

func (c *tlsFingerprintCaptureConn) captureClientHello() error {
	offset := 0
	for range 32 {
		headerView, err := c.reader.Peek(offset + 5)
		if err != nil {
			return err
		}
		header := headerView[offset : offset+5]
		recordLen := int(header[3])<<8 | int(header[4])
		nextOffset := offset + 5 + recordLen
		if recordLen <= 0 || nextOffset > tlsFingerprintCollectorClientHelloBufferSize {
			return errors.New("TLS ClientHello exceeds capture limit")
		}
		records, err := c.reader.Peek(nextOffset)
		if err != nil {
			return err
		}
		captured, parseErr := tlsfingerprint.ParseCapturedClientHelloRecords(records[:nextOffset])
		if parseErr == nil {
			c.captured = captured
			return nil
		}
		if !errors.Is(parseErr, tlsfingerprint.ErrIncompleteClientHello) {
			return parseErr
		}
		offset = nextOffset
	}
	return errors.New("TLS ClientHello spans too many records")
}

func (c *tlsFingerprintCaptureConn) captureContext() *tlsFingerprintCaptureContext {
	if c.captured == nil {
		return nil
	}
	return &tlsFingerprintCaptureContext{
		clientHello:    c.captured,
		negotiatedALPN: c.negotiatedALPN,
		connectionID:   c.connectionID,
	}
}

func (c *readerConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}

func writeCollectorJSON(w http.ResponseWriter, status int, payload map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func randomHexToken(bytesLen int) (string, error) {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func generateTLSFingerprintCollectorCertificate(host string) (tls.Certificate, string, error) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	now := time.Now()
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(now.UnixNano()),
		Subject:               pkix.Name{CommonName: "TokenRouter TLS Fingerprint Collector CA"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return tls.Certificate{}, "", err
	}

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(now.UnixNano() + 1),
		Subject:      pkix.Name{CommonName: host},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	addCertificateHost(serverTemplate, host)
	addCertificateHost(serverTemplate, "localhost")
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverKey.PublicKey, caKey)
	if err != nil {
		return tls.Certificate{}, "", err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER})
	keyDER, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	caPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}))
	return cert, caPEM, nil
}

func addCertificateHost(cert *x509.Certificate, host string) {
	host = strings.TrimSpace(host)
	if host == "" {
		return
	}
	if ip := net.ParseIP(host); ip != nil {
		cert.IPAddresses = append(cert.IPAddresses, ip)
		return
	}
	if strings.Contains(host, ":") {
		if splitHost, _, err := net.SplitHostPort(host); err == nil {
			addCertificateHost(cert, splitHost)
			return
		}
	}
	cert.DNSNames = append(cert.DNSNames, host)
}

func detectTLSFingerprintClientKind(r *http.Request) string {
	ua := strings.ToLower(r.UserAgent())
	path := strings.ToLower(r.URL.Path)
	switch {
	case strings.Contains(ua, "claude") || strings.Contains(path, "messages"):
		return "claude_code"
	case strings.Contains(ua, "codex") || strings.Contains(path, "codex") || strings.Contains(path, "responses"):
		return "codex"
	default:
		return "unknown"
	}
}

func captureTokenFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if token := strings.TrimSpace(r.URL.Query().Get("token")); token != "" {
		return token
	}
	if token := strings.TrimSpace(r.Header.Get("X-TLS-Fingerprint-Token")); token != "" {
		return token
	}
	if token := proxyAuthorizationToken(r.Header.Get("Proxy-Authorization")); token != "" {
		return token
	}
	// 客户端把采集地址配置为 base URL 后会继续追加自身 API 路径，因此从 /capture/{token}/... 中提取 token。
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) >= 2 && parts[0] == "capture" {
		return strings.TrimSpace(parts[1])
	}
	// Claude Code 通过 ANTHROPIC_AUTH_TOKEN 发送鉴权信息，采集器复用该值作为会话 token。
	if token := bearerTokenFromAuthorization(r.Header.Get("Authorization")); token != "" {
		return token
	}
	if token := strings.TrimSpace(r.Header.Get("X-Api-Key")); token != "" {
		return token
	}
	return ""
}

func proxyAuthorizationToken(value string) string {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) != 2 {
		return ""
	}
	if strings.EqualFold(fields[0], "Bearer") {
		return strings.TrimSpace(fields[1])
	}
	if !strings.EqualFold(fields[0], "Basic") {
		return ""
	}
	decoded, err := base64.StdEncoding.DecodeString(fields[1])
	if err != nil {
		return ""
	}
	username, password, ok := strings.Cut(string(decoded), ":")
	if !ok || username != "capture" {
		return ""
	}
	return strings.TrimSpace(password)
}

func normalizeTLSFingerprintCollectorSessionRequest(request TLSFingerprintCollectorSessionRequest) (TLSFingerprintCollectorSessionRequest, error) {
	request.Mode = strings.ToLower(strings.TrimSpace(request.Mode))
	if request.Mode == "" {
		request.Mode = tlsFingerprintCaptureModeDirect
	}
	if request.Mode == tlsFingerprintCaptureModeDirect {
		return request, nil
	}
	if request.Mode != tlsFingerprintCaptureModeControlledConnect {
		return request, errors.New("capture mode must be controlled_connect")
	}
	request.ClientKind = strings.ToLower(strings.TrimSpace(request.ClientKind))
	switch request.ClientKind {
	case "codex", "node", "claude_code":
	default:
		return request, errors.New("client_kind must be codex, node, or claude_code")
	}
	request.ClientVersion = strings.TrimSpace(request.ClientVersion)
	request.Runtime = strings.TrimSpace(request.Runtime)
	request.RuntimeVersion = strings.TrimSpace(request.RuntimeVersion)
	request.OperatingSystem = strings.TrimSpace(request.OperatingSystem)
	request.Architecture = strings.TrimSpace(request.Architecture)
	if request.ClientVersion == "" || request.OperatingSystem == "" || request.Architecture == "" {
		return request, errors.New("client_version, operating_system, and architecture are required")
	}
	for _, value := range []string{request.ClientVersion, request.Runtime, request.RuntimeVersion, request.OperatingSystem, request.Architecture} {
		if len(value) > 128 {
			return request, errors.New("capture labels must be at most 128 characters")
		}
	}
	targetHost, err := normalizeTLSFingerprintTargetHost(request.TargetHost)
	if err != nil {
		return request, err
	}
	request.TargetHost = targetHost
	if !request.ColdStartConfirmed {
		return request, errors.New("cold_start_confirmed must be true for controlled capture")
	}
	return request, nil
}

func normalizeTLSFingerprintTargetHost(raw string) (string, error) {
	host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw), "."))
	if _, ok := tlsFingerprintControlledTargets[host]; !ok {
		return "", errors.New("target_host must be chatgpt.com or api.openai.com")
	}
	return host, nil
}

func normalizeTLSFingerprintConnectTarget(raw string) (string, error) {
	host, port, err := net.SplitHostPort(strings.TrimSpace(raw))
	if err != nil || port != "443" {
		return "", errors.New("CONNECT target must use port 443")
	}
	return normalizeTLSFingerprintTargetHost(host)
}

func firstNonEmptyCollectorString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func tlsFingerprintValidationFindingMessages(values []tlsfingerprint.ProfileValidationFinding) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		message := strings.TrimSpace(value.Message)
		if code := strings.TrimSpace(value.Code); code != "" {
			message = code + ": " + message
		}
		if message != "" {
			result = append(result, message)
		}
	}
	return result
}

func joinTLSFingerprintValidationFindings(values []tlsfingerprint.ProfileValidationFinding) string {
	messages := tlsFingerprintValidationFindingMessages(values)
	if len(messages) == 0 {
		return "unknown validation error"
	}
	return strings.Join(messages, "; ")
}

func truncateTLSFingerprintCollectorError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 300 {
		return value
	}
	return value[:300]
}

func (s *TLSFingerprintCollectorService) runTLSFingerprintHandshakeProbe(ctx context.Context, profile *tlsfingerprint.Profile, target string) (*tlsfingerprint.TLSHandshakeResult, error) {
	if s != nil && s.probeTLSHandshake != nil {
		return s.probeTLSHandshake(ctx, profile, target)
	}
	return tlsfingerprint.ProbeTLSHandshake(ctx, profile, target)
}

func cloneTLSFingerprintProfile(source *egress.TLSFingerprintProfile) *egress.TLSFingerprintProfile {
	if source == nil {
		return nil
	}
	clone := *source
	clone.CipherSuites = append([]uint16(nil), source.CipherSuites...)
	clone.Curves = append([]uint16(nil), source.Curves...)
	clone.PointFormats = append([]uint16(nil), source.PointFormats...)
	clone.SignatureAlgorithms = append([]uint16(nil), source.SignatureAlgorithms...)
	clone.SignatureAlgorithmsCert = append([]uint16(nil), source.SignatureAlgorithmsCert...)
	clone.ALPNProtocols = append([]string(nil), source.ALPNProtocols...)
	clone.SupportedVersions = append([]uint16(nil), source.SupportedVersions...)
	clone.KeyShareGroups = append([]uint16(nil), source.KeyShareGroups...)
	clone.PSKModes = append([]uint16(nil), source.PSKModes...)
	clone.Extensions = append([]uint16(nil), source.Extensions...)
	return &clone
}

func bearerTokenFromAuthorization(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	fields := strings.Fields(value)
	if len(fields) == 2 && strings.EqualFold(fields[0], "Bearer") {
		return strings.TrimSpace(fields[1])
	}
	if len(fields) == 1 {
		return strings.TrimSpace(fields[0])
	}
	return ""
}

func defaultTLSFingerprintProfileName(clientKind string, ts time.Time) string {
	switch clientKind {
	case "claude_code":
		return "Claude Code " + ts.Format("2006-01-02 15:04:05")
	case "codex":
		return "Codex CLI " + ts.Format("2006-01-02 15:04:05")
	default:
		return "Captured TLS " + ts.Format("2006-01-02 15:04:05")
	}
}

func tlsFingerprintProfileToYAML(profile *egress.TLSFingerprintProfile) string {
	if profile == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprint(&b, "captured_profile:\n")
	writeYAMLString(&b, "name", profile.Name)
	if profile.Description != nil {
		writeYAMLString(&b, "description", *profile.Description)
	}
	fmt.Fprintf(&b, "  enable_grease: %t\n", profile.EnableGREASE)
	writeYAMLNumberArray(&b, "cipher_suites", profile.CipherSuites)
	writeYAMLNumberArray(&b, "curves", profile.Curves)
	writeYAMLNumberArray(&b, "point_formats", profile.PointFormats)
	writeYAMLNumberArray(&b, "signature_algorithms", profile.SignatureAlgorithms)
	writeYAMLNumberArray(&b, "signature_algorithms_cert", profile.SignatureAlgorithmsCert)
	writeYAMLStringArray(&b, "alpn_protocols", profile.ALPNProtocols)
	writeYAMLNumberArray(&b, "supported_versions", profile.SupportedVersions)
	writeYAMLNumberArray(&b, "key_share_groups", profile.KeyShareGroups)
	writeYAMLNumberArray(&b, "psk_modes", profile.PSKModes)
	writeYAMLNumberArray(&b, "extensions", profile.Extensions)
	return b.String()
}

func writeYAMLString(b *strings.Builder, key, value string) {
	escaped := strings.ReplaceAll(value, `"`, `\"`)
	fmt.Fprintf(b, "  %s: \"%s\"\n", key, escaped)
}

func writeYAMLNumberArray[T ~uint16](b *strings.Builder, key string, values []T) {
	fmt.Fprintf(b, "  %s: [", key)
	for i, value := range values {
		if i > 0 {
			fmt.Fprint(b, ", ")
		}
		fmt.Fprintf(b, "%d", value)
	}
	fmt.Fprint(b, "]\n")
}

func writeYAMLStringArray(b *strings.Builder, key string, values []string) {
	fmt.Fprintf(b, "  %s: [", key)
	for i, value := range values {
		if i > 0 {
			fmt.Fprint(b, ", ")
		}
		escaped := strings.ReplaceAll(value, `"`, `\"`)
		fmt.Fprintf(b, "\"%s\"", escaped)
	}
	fmt.Fprint(b, "]\n")
}

func summarizeTLSFingerprintHeaders(headers http.Header) map[string]string {
	allowed := []string{
		"user-agent",
		"x-stainless-os",
		"x-stainless-arch",
		"x-stainless-runtime",
		"x-stainless-runtime-version",
		"x-stainless-lang",
		"x-stainless-package-version",
		"anthropic-version",
		"openai-organization",
	}
	out := make(map[string]string)
	for _, key := range allowed {
		if value := headers.Get(key); value != "" {
			out[key] = value
		}
	}
	return out
}

func summarizeStainlessHeaders(headers http.Header) map[string]string {
	out := make(map[string]string)
	for key, values := range headers {
		lower := strings.ToLower(key)
		if !strings.HasPrefix(lower, "x-stainless-") || len(values) == 0 {
			continue
		}
		out[lower] = values[0]
	}
	return out
}

func hostFromURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	req, err := http.NewRequest(http.MethodGet, raw, nil)
	if err != nil || req.URL == nil {
		return ""
	}
	host := req.URL.Hostname()
	if host == "" {
		return req.URL.Host
	}
	return host
}

func tlsFingerprintCollectorStringPtr(v string) *string {
	return &v
}

func strconvItoa(v int) string {
	return fmt.Sprintf("%d", v)
}

// CreateSession 要求调用方提供受控采集声明。
func (s *TLSFingerprintCollectorService) CreateSession() (*TLSFingerprintCollectorSession, error) {
	return nil, fmt.Errorf("controlled capture requires client identity and target")
}

// Shutdown 关闭监听并清理内存采集记录。
func (s *TLSFingerprintCollectorService) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	return s.Stop(ctx)
}
