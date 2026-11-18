package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/egress"
	"github.com/TokenFlux/TokenRouter/internal/infra/httpclient/tlsfingerprint"
	"github.com/TokenFlux/TokenRouter/internal/infra/telemetry/logging"
	"github.com/TokenFlux/TokenRouter/internal/pkg/authenticatedingress"
)

const (
	authenticatedIngressEdgeTokenHeader          = "X-TokenRouter-Ingress-Edge-Token"
	authenticatedIngressConnectionIDHeader       = "X-TokenRouter-Ingress-Connection-Id"
	authenticatedIngressConnectionRequestsHeader = "X-TokenRouter-Ingress-Connection-Requests"
	authenticatedIngressRemoteAddressHeader      = "X-TokenRouter-Ingress-Remote-Addr"
	authenticatedIngressResolvedAddressHeader    = "X-TokenRouter-Ingress-Resolved-Client-Addr"
	authenticatedIngressRemotePortHeader         = "X-TokenRouter-Ingress-Remote-Port"
	authenticatedIngressServerAddressHeader      = "X-TokenRouter-Ingress-Server-Addr"
	authenticatedIngressServerPortHeader         = "X-TokenRouter-Ingress-Server-Port"
	authenticatedIngressSessionReusedHeader      = "X-TokenRouter-Ingress-Session-Reused"
	authenticatedIngressALPNHeader               = "X-TokenRouter-Ingress-Alpn"
	authenticatedIngressHTTPProtocolHeader       = "X-TokenRouter-Ingress-Http-Protocol"

	authenticatedIngressValidationTimeout = 45 * time.Second
	authenticatedIngressMatchWindow       = 5 * time.Second
	authenticatedIngressValidationWorkers = 2
	authenticatedIngressValidationQueue   = 128
	authenticatedIngressMatchWorkers      = 2
	authenticatedIngressMatchQueue        = 256
	authenticatedIngressCleanupInterval   = 5 * time.Second
	authenticatedIngressMaxEventBytes     = 32 << 10
	authenticatedIngressMaxPendingBytes   = 64 << 20
)

var (
	authenticatedIngressCodexUA  = regexp.MustCompile(`(?i)(codex_cli_rs|codex-tui|codex_vscode)/([0-9][0-9a-z.+-]*)`)
	authenticatedIngressClaudeUA = regexp.MustCompile(`(?i)claude-cli/([0-9][0-9a-z.+-]*)`)
	authenticatedIngressNodeUA   = regexp.MustCompile(`(?i)(?:openai/nodejs|node(?:\.js)?)/([0-9][0-9a-z.+-]*)`)
)

// AuthenticatedIngressQuotaStore 为自动验证提供跨实例、跨重启的原子日配额。
type AuthenticatedIngressQuotaStore interface {
	ReserveValidation(ctx context.Context, date, ownerID string, expiresAt time.Time, globalLimit, ownerLimit int) (bool, error)
	ReserveProfile(ctx context.Context, date string, expiresAt time.Time, limit int) (bool, error)
}

// AuthenticatedIngressTLSStatus 是不包含客户、地址或 API Key 标识的运行状态摘要。
type AuthenticatedIngressTLSStatus struct {
	Enabled                   bool       `json:"enabled"`
	PendingClientHellos       int        `json:"pending_client_hellos"`
	PendingAuthenticatedFlows int        `json:"pending_authenticated_flows"`
	CandidateGroups           int        `json:"candidate_groups"`
	ValidationsInFlight       int        `json:"validations_in_flight"`
	MatchedFlows              uint64     `json:"matched_flows"`
	RejectedFlows             uint64     `json:"rejected_flows"`
	VerifiedProfilesCreated   uint64     `json:"verified_profiles_created"`
	ExistingProfilesMatched   uint64     `json:"existing_profiles_matched"`
	LastVerifiedAt            *time.Time `json:"last_verified_at,omitempty"`
	LastError                 string     `json:"last_error,omitempty"`
}

type authenticatedIngressClientIdentity struct {
	Kind               string
	Version            string
	Runtime            string
	RuntimeVersion     string
	OperatingSystem    string
	Architecture       string
	ClassifierEvidence []string
}

type authenticatedIngressAuthEvent struct {
	flowID       string
	connectionID string
	ownerID      string
	observedAt   time.Time
	identity     authenticatedIngressClientIdentity
	alpn         string
	httpProtocol string
}

type authenticatedIngressHelloEvent struct {
	observation    *tlsfingerprint.AuthenticatedIngressObservation
	estimatedBytes int
	sequence       uint64
}

type authenticatedIngressHelloOrderEntry struct {
	flowID   string
	sequence uint64
}

type authenticatedIngressMatchedEvent struct {
	hello *authenticatedIngressHelloEvent
	auth  *authenticatedIngressAuthEvent
}

type authenticatedIngressCandidate struct {
	key               string
	profile           *egress.TLSFingerprintProfile
	replay            tlsfingerprint.ProfileValidationReport
	identity          authenticatedIngressClientIdentity
	observedHost      string
	connections       map[string]struct{}
	firstSeen         time.Time
	lastSeen          time.Time
	inFlight          bool
	completed         bool
	estimatedBytes    int
	validationOwnerID string
	failureCount      int
	retryAfter        time.Time
}

// AuthenticatedIngressTLSService 关联短期 ClientHello 与已通过 API Key 鉴权的同一连接。
// 原始五元组、API Key、TLS random、ticket 和密钥字节都不会进入该服务的持久状态。
type AuthenticatedIngressTLSService struct {
	cfg      *authenticatedingress.Config
	profiles *egress.TLSFingerprintProfileService
	quota    AuthenticatedIngressQuotaStore

	mu              sync.Mutex
	hellos          map[string]*authenticatedIngressHelloEvent
	auth            map[string]*authenticatedIngressAuthEvent
	candidates      map[string]*authenticatedIngressCandidate
	completed       map[string]time.Time
	stopping        bool
	wg              sync.WaitGroup
	workerCtx       context.Context
	workerStop      context.CancelFunc
	matchQueue      chan *authenticatedIngressMatchedEvent
	validationQueue chan *authenticatedIngressCandidate
	helloBytes      int
	helloSequence   uint64
	helloOrder      []authenticatedIngressHelloOrderEntry
	helloOrderHead  int
	candidateBytes  int
	lastCleanup     time.Time

	matchedFlows            uint64
	rejectedFlows           uint64
	verifiedProfilesCreated uint64
	existingProfilesMatched uint64
	lastVerifiedAt          *time.Time
	lastError               string
	probeTLSHandshake       func(context.Context, *tlsfingerprint.Profile, string) (*tlsfingerprint.TLSHandshakeResult, error)
}

type (
	AuthenticatedIngressRequestSnapshot = authenticatedingress.AuthenticatedIngressRequestSnapshot
	AuthenticatedIngressIdentity        = authenticatedingress.AuthenticatedIngressIdentity
)

// NewAuthenticatedIngressTLSService 创建默认关闭、显式配置后才工作的可信关联服务。
func NewAuthenticatedIngressTLSService(cfg *authenticatedingress.Config, profiles *egress.TLSFingerprintProfileService, quota AuthenticatedIngressQuotaStore) *AuthenticatedIngressTLSService {
	workerCtx, workerStop := context.WithCancel(context.Background())
	svc := &AuthenticatedIngressTLSService{
		cfg:               cfg,
		profiles:          profiles,
		quota:             quota,
		hellos:            make(map[string]*authenticatedIngressHelloEvent),
		auth:              make(map[string]*authenticatedIngressAuthEvent),
		candidates:        make(map[string]*authenticatedIngressCandidate),
		completed:         make(map[string]time.Time),
		workerCtx:         workerCtx,
		workerStop:        workerStop,
		matchQueue:        make(chan *authenticatedIngressMatchedEvent, authenticatedIngressMatchQueue),
		validationQueue:   make(chan *authenticatedIngressCandidate, authenticatedIngressValidationQueue),
		probeTLSHandshake: tlsfingerprint.ProbeTLSHandshake,
	}
	if svc.Enabled() {
		for range authenticatedIngressMatchWorkers {
			svc.wg.Add(1)
			go svc.matchWorker()
		}
		for range authenticatedIngressValidationWorkers {
			svc.wg.Add(1)
			go svc.validationWorker()
		}
	}
	return svc
}

// Stop 阻止新验证并等待已经启动的无凭据 TLS-only 验证结束。
func (s *AuthenticatedIngressTLSService) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.stopping = true
	s.hellos = make(map[string]*authenticatedIngressHelloEvent)
	s.auth = make(map[string]*authenticatedIngressAuthEvent)
	s.candidates = make(map[string]*authenticatedIngressCandidate)
	s.completed = make(map[string]time.Time)
	s.helloBytes = 0
	s.helloOrder = nil
	s.helloOrderHead = 0
	s.candidateBytes = 0
	if s.workerStop != nil {
		s.workerStop()
	}
	s.mu.Unlock()
	s.wg.Wait()
}

// Enabled 返回可信关联是否已经显式配置并启用。
func (s *AuthenticatedIngressTLSService) Enabled() bool {
	return s != nil && s.cfg != nil && s.cfg.Enabled
}

// Status 返回匿名化的采集进度，不暴露 owner、flow 或客户端地址。
func (s *AuthenticatedIngressTLSService) Status() AuthenticatedIngressTLSStatus {
	status := AuthenticatedIngressTLSStatus{Enabled: s.Enabled()}
	if s == nil {
		return status
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked(time.Now())
	status.PendingClientHellos = len(s.hellos)
	status.PendingAuthenticatedFlows = len(s.auth)
	status.CandidateGroups = len(s.candidates)
	for _, candidate := range s.candidates {
		if candidate != nil && candidate.inFlight {
			status.ValidationsInFlight++
		}
	}
	status.MatchedFlows = s.matchedFlows
	status.RejectedFlows = s.rejectedFlows
	status.VerifiedProfilesCreated = s.verifiedProfilesCreated
	status.ExistingProfilesMatched = s.existingProfilesMatched
	status.LastVerifiedAt = s.lastVerifiedAt
	status.LastError = s.lastError
	return status
}

// VerifyObserverToken 使用常量时间比较内部观察器 token。
func (s *AuthenticatedIngressTLSService) VerifyObserverToken(value string) bool {
	if !s.Enabled() {
		return false
	}
	want := strings.TrimSpace(s.cfg.ObserverToken)
	got := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(value), "Bearer "))
	return len(want) == len(got) && subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}

// AcceptObservation 接收 observer 已经去除敏感字节的 ClientHello 结构。
func (s *AuthenticatedIngressTLSService) AcceptObservation(observation *tlsfingerprint.AuthenticatedIngressObservation) error {
	if !s.Enabled() {
		return errors.New("authenticated ingress TLS collection is disabled")
	}
	if observation == nil || observation.ClientHello == nil {
		return errors.New("authenticated ingress observation is incomplete")
	}
	if observation.SchemaVersion != tlsfingerprint.AuthenticatedIngressObservationSchemaVersion {
		return errors.New("authenticated ingress observation schema is unsupported")
	}
	if len(observation.FlowID) != sha256.Size*2 {
		return errors.New("authenticated ingress flow id is invalid")
	}
	if _, err := hex.DecodeString(observation.FlowID); err != nil {
		return errors.New("authenticated ingress flow id is invalid")
	}
	now := time.Now()
	ttl := s.pendingTTL()
	if observation.ObservedAt.IsZero() || observation.ObservedAt.Before(now.Add(-ttl)) || observation.ObservedAt.After(now.Add(30*time.Second)) {
		return errors.New("authenticated ingress observation is outside the allowed time window")
	}
	observation.ServerName = strings.ToLower(strings.TrimSpace(observation.ServerName))
	if !s.allowedObservedHost(observation.ServerName) {
		return errors.New("authenticated ingress observed host is not allowed")
	}
	if observation.ClientHello.HasPreSharedKey || observation.ClientHello.SessionTicketLength > 0 {
		return errors.New("TLS resumption observations are not eligible")
	}
	estimatedBytes := estimateAuthenticatedIngressClientHelloBytes(observation.ClientHello)
	if estimatedBytes <= 0 || estimatedBytes > authenticatedIngressMaxEventBytes {
		return errors.New("authenticated ingress observation exceeds the safe structural size")
	}

	s.mu.Lock()
	if s.stopping {
		s.mu.Unlock()
		return errors.New("authenticated ingress TLS service is stopping")
	}
	s.cleanupLocked(now)
	previous := s.hellos[observation.FlowID]
	if previous != nil && previous.observation != nil && !observation.ObservedAt.After(previous.observation.ObservedAt) {
		s.mu.Unlock()
		return nil
	}
	if previous != nil {
		delete(s.hellos, observation.FlowID)
		s.helloBytes -= previous.estimatedBytes
	}
	for len(s.hellos) >= s.maxPendingFlows() || s.helloBytes+estimatedBytes > authenticatedIngressMaxPendingBytes {
		if !s.evictOldestHelloLocked() {
			if previous != nil {
				s.hellos[observation.FlowID] = previous
				s.helloBytes += previous.estimatedBytes
			}
			s.rejectedFlows++
			s.lastError = "authenticated ingress pending ClientHello capacity is full"
			s.mu.Unlock()
			return errors.New("authenticated ingress pending capacity is full")
		}
	}
	s.helloSequence++
	event := &authenticatedIngressHelloEvent{observation: observation, estimatedBytes: estimatedBytes, sequence: s.helloSequence}
	s.hellos[observation.FlowID] = event
	s.helloBytes += estimatedBytes
	s.helloOrder = append(s.helloOrder, authenticatedIngressHelloOrderEntry{flowID: observation.FlowID, sequence: event.sequence})
	s.trimStaleHelloOrderLocked()
	s.rebuildHelloOrderIfNeededLocked()
	matched := s.popMatchLocked(observation.FlowID)
	s.mu.Unlock()
	s.enqueueMatched(matched)
	return nil
}

// ObserveAuthenticatedIngressRequest 在 API Key 完整鉴权成功后接收一次请求。
// APIKeyService 只传入白名单请求快照与两个内部数字 ID，不交出完整 APIKey 对象。
func (s *AuthenticatedIngressTLSService) ObserveAuthenticatedIngressRequest(request AuthenticatedIngressRequestSnapshot, identityProof AuthenticatedIngressIdentity) {
	if !s.Enabled() || identityProof.APIKeyID <= 0 || identityProof.UserID <= 0 {
		return
	}
	configValue := s.cfg
	if len(request.EdgeToken) != len(configValue.EdgeToken) || subtle.ConstantTimeCompare([]byte(request.EdgeToken), []byte(configValue.EdgeToken)) != 1 {
		return
	}
	// 每条 TLS 连接只使用第一个已认证 HTTP 请求，避免 HTTP/2 多路请求重复计数。
	if request.ConnectionRequests != "1" || request.ConnectionID == "" {
		return
	}
	if request.SessionReused != "." {
		return
	}
	// RealIP 改写意味着 TLS 在可信 CDN/代理处已经重新终止；此时 observer 看到的是代理指纹。
	if !sameAuthenticatedIngressAddress(request.RemoteAddress, request.ResolvedAddress) {
		s.mu.Lock()
		s.rejectedFlows++
		s.lastError = "authenticated ingress rejected an edge-reterminated TLS connection"
		s.mu.Unlock()
		return
	}
	sourcePort, err := parseAuthenticatedIngressPort(request.RemotePort)
	if err != nil {
		return
	}
	destinationPort, err := parseAuthenticatedIngressPort(request.ServerPort)
	if err != nil {
		return
	}
	flowID, err := tlsfingerprint.BuildAuthenticatedIngressFlowID(
		configValue.ObserverToken,
		request.RemoteAddress,
		sourcePort,
		request.ServerAddress,
		destinationPort,
	)
	if err != nil {
		return
	}
	identity := classifyAuthenticatedIngressClient(request)
	if identity.Kind == "" || identity.Version == "" {
		s.mu.Lock()
		s.rejectedFlows++
		s.lastError = "authenticated ingress client could not be classified"
		s.mu.Unlock()
		return
	}
	event := &authenticatedIngressAuthEvent{
		flowID:       flowID,
		connectionID: request.ConnectionID,
		ownerID:      s.ownerID(identityProof.UserID),
		observedAt:   time.Now(),
		identity:     identity,
		alpn:         request.ALPN,
		httpProtocol: request.HTTPProtocol,
	}
	s.mu.Lock()
	if s.stopping {
		s.mu.Unlock()
		return
	}
	s.cleanupLocked(event.observedAt)
	if _, exists := s.auth[flowID]; !exists && len(s.auth) >= s.maxPendingFlows() {
		s.rejectedFlows++
		s.lastError = "authenticated ingress pending auth capacity is full"
		s.mu.Unlock()
		return
	}
	s.auth[flowID] = event
	matched := s.popMatchLocked(flowID)
	s.mu.Unlock()
	s.enqueueMatched(matched)
}

func takeAuthenticatedIngressRequestSnapshot(request *http.Request) AuthenticatedIngressRequestSnapshot {
	if request == nil {
		return AuthenticatedIngressRequestSnapshot{}
	}
	header := request.Header
	result := AuthenticatedIngressRequestSnapshot{
		UserAgent:          strings.TrimSpace(header.Get("User-Agent")),
		Originator:         strings.TrimSpace(header.Get("originator")),
		EdgeToken:          strings.TrimSpace(header.Get(authenticatedIngressEdgeTokenHeader)),
		ConnectionID:       strings.TrimSpace(header.Get(authenticatedIngressConnectionIDHeader)),
		ConnectionRequests: strings.TrimSpace(header.Get(authenticatedIngressConnectionRequestsHeader)),
		RemoteAddress:      strings.TrimSpace(header.Get(authenticatedIngressRemoteAddressHeader)),
		ResolvedAddress:    strings.TrimSpace(header.Get(authenticatedIngressResolvedAddressHeader)),
		RemotePort:         strings.TrimSpace(header.Get(authenticatedIngressRemotePortHeader)),
		ServerAddress:      strings.TrimSpace(header.Get(authenticatedIngressServerAddressHeader)),
		ServerPort:         strings.TrimSpace(header.Get(authenticatedIngressServerPortHeader)),
		SessionReused:      strings.TrimSpace(header.Get(authenticatedIngressSessionReusedHeader)),
		ALPN:               strings.TrimSpace(header.Get(authenticatedIngressALPNHeader)),
		HTTPProtocol:       strings.TrimSpace(header.Get(authenticatedIngressHTTPProtocolHeader)),
	}
	for _, name := range []string{
		authenticatedIngressEdgeTokenHeader,
		authenticatedIngressConnectionIDHeader,
		authenticatedIngressConnectionRequestsHeader,
		authenticatedIngressRemoteAddressHeader,
		authenticatedIngressResolvedAddressHeader,
		authenticatedIngressRemotePortHeader,
		authenticatedIngressServerAddressHeader,
		authenticatedIngressServerPortHeader,
		authenticatedIngressSessionReusedHeader,
		authenticatedIngressALPNHeader,
		authenticatedIngressHTTPProtocolHeader,
	} {
		header.Del(name)
	}
	return result
}

func sameAuthenticatedIngressAddress(left, right string) bool {
	parse := func(value string) (netip.Addr, error) {
		value = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(value), "["), "]"))
		address, err := netip.ParseAddr(value)
		if err != nil {
			return netip.Addr{}, err
		}
		return address.Unmap(), nil
	}
	leftAddress, leftErr := parse(left)
	rightAddress, rightErr := parse(right)
	return leftErr == nil && rightErr == nil && leftAddress == rightAddress
}

func parseAuthenticatedIngressPort(value string) (uint16, error) {
	parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 16)
	if err != nil || parsed == 0 {
		return 0, errors.New("invalid authenticated ingress port")
	}
	return uint16(parsed), nil
}

func (s *AuthenticatedIngressTLSService) popMatchLocked(flowID string) *authenticatedIngressMatchedEvent {
	hello := s.hellos[flowID]
	auth := s.auth[flowID]
	if hello == nil || auth == nil {
		return nil
	}
	delete(s.hellos, flowID)
	delete(s.auth, flowID)
	s.helloBytes -= hello.estimatedBytes
	if s.helloBytes < 0 {
		s.helloBytes = 0
	}
	s.trimStaleHelloOrderLocked()
	if delta := auth.observedAt.Sub(hello.observation.ObservedAt); delta < 0 || delta > authenticatedIngressMatchWindow {
		s.rejectedFlows++
		s.lastError = "authenticated ingress flow timestamps did not match"
		return nil
	}
	s.matchedFlows++
	return &authenticatedIngressMatchedEvent{hello: hello, auth: auth}
}

func (s *AuthenticatedIngressTLSService) enqueueMatched(value *authenticatedIngressMatchedEvent) {
	if value == nil {
		return
	}
	select {
	case s.matchQueue <- value:
	default:
		s.mu.Lock()
		s.rejectedFlows++
		s.lastError = "authenticated ingress match queue is full"
		s.mu.Unlock()
	}
}

func (s *AuthenticatedIngressTLSService) matchWorker() {
	defer s.wg.Done()
	for {
		select {
		case <-s.workerCtx.Done():
			return
		case matched := <-s.matchQueue:
			if matched != nil {
				s.processMatched(matched)
			}
		}
	}
}

func (s *AuthenticatedIngressTLSService) processMatched(matched *authenticatedIngressMatchedEvent) {
	if matched == nil || matched.hello == nil || matched.auth == nil || matched.hello.observation == nil || matched.hello.observation.ClientHello == nil {
		return
	}
	profile := authenticatedIngressProfileFromCapture(matched.hello.observation.ClientHello)
	replay := tlsfingerprint.ValidateCapturedProfile(matched.hello.observation.ClientHello, ToTLSProfile(profile))
	if !replay.Reusable {
		s.mu.Lock()
		s.rejectedFlows++
		s.lastError = "authenticated ingress ClientHello is not reproducible"
		s.mu.Unlock()
		return
	}
	now := time.Now()
	auth := matched.auth
	hello := matched.hello
	groupKey := authenticatedIngressCandidateKey(hello.observation.ServerName, profile.FingerprintSHA256(), s.validationTargets())
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopping {
		return
	}
	candidate := s.candidates[groupKey]
	if _, completed := s.completed[groupKey]; completed {
		return
	}
	if candidate == nil {
		if len(s.candidates) >= s.maxCandidateGroups() || s.candidateBytes+hello.estimatedBytes > authenticatedIngressMaxPendingBytes {
			s.rejectedFlows++
			s.lastError = "authenticated ingress candidate capacity is full"
			return
		}
		candidate = &authenticatedIngressCandidate{
			key:            groupKey,
			profile:        profile,
			replay:         replay,
			identity:       auth.identity,
			observedHost:   hello.observation.ServerName,
			connections:    make(map[string]struct{}),
			firstSeen:      now,
			estimatedBytes: hello.estimatedBytes,
		}
		s.candidates[groupKey] = candidate
		s.candidateBytes += candidate.estimatedBytes
	}
	candidate.lastSeen = now
	minimum := s.minimumConnections()
	if candidate.completed || candidate.inFlight {
		return
	}
	if !candidate.retryAfter.IsZero() && now.Before(candidate.retryAfter) {
		return
	}
	if len(candidate.connections) < minimum {
		connectionEvidence := sha256.Sum256([]byte(hello.observation.FlowID + "\x00" + auth.connectionID))
		candidate.connections[hex.EncodeToString(connectionEvidence[:])] = struct{}{}
	}
	if len(candidate.connections) < minimum || !s.cfg.AutoCreateVerified {
		return
	}
	candidate.inFlight = true
	candidate.validationOwnerID = auth.ownerID
	validation := cloneAuthenticatedIngressCandidate(candidate)
	select {
	case s.validationQueue <- validation:
	default:
		candidate.inFlight = false
		candidate.failureCount++
		candidate.retryAfter = now.Add(10 * time.Second)
		s.rejectedFlows++
		s.lastError = "authenticated ingress validation queue is full"
	}
}

func cloneAuthenticatedIngressCandidate(value *authenticatedIngressCandidate) *authenticatedIngressCandidate {
	clone := *value
	clone.profile = cloneTLSFingerprintProfile(value.profile)
	clone.connections = make(map[string]struct{}, len(value.connections))
	for connection := range value.connections {
		clone.connections[connection] = struct{}{}
	}
	clone.identity.ClassifierEvidence = append([]string(nil), value.identity.ClassifierEvidence...)
	return &clone
}

func (s *AuthenticatedIngressTLSService) validationWorker() {
	defer s.wg.Done()
	for {
		select {
		case <-s.workerCtx.Done():
			return
		case candidate := <-s.validationQueue:
			if candidate != nil {
				s.validateAndCreate(s.workerCtx, candidate)
			}
		}
	}
}

func (s *AuthenticatedIngressTLSService) validateAndCreate(parent context.Context, candidate *authenticatedIngressCandidate) {
	ctx, cancel := context.WithTimeout(parent, authenticatedIngressValidationTimeout)
	defer cancel()

	if existing, err := s.findEquivalentProfile(ctx, candidate.profile, s.validationTargets()); err == nil && existing != nil {
		s.mu.Lock()
		s.existingProfilesMatched++
		s.mu.Unlock()
		s.finishCandidate(candidate.key, true, "")
		return
	}
	quotaAllowed, quotaErr := s.reserveValidationQuota(ctx, candidate.validationOwnerID)
	if quotaErr != nil {
		s.finishCandidate(candidate.key, false, "authenticated ingress validation quota is unavailable")
		return
	}
	if !quotaAllowed {
		s.finishCandidate(candidate.key, false, "authenticated ingress daily validation quota reached")
		return
	}

	now := time.Now()
	validations := make([]egress.TLSFingerprintTargetValidation, 0, len(s.validationTargets()))
	for _, target := range s.validationTargets() {
		probe, err := s.probeTLSHandshake(ctx, ToTLSProfile(candidate.profile), target+":443")
		validation := egress.TLSFingerprintTargetValidation{Host: target, Success: err == nil, ValidatedAt: time.Now()}
		if err != nil {
			s.finishCandidate(candidate.key, false, fmt.Sprintf("TLS-only target handshake failed for %s: %v", target, err))
			return
		}
		validation.NegotiatedVersion = probe.Version
		validation.CipherSuite = probe.CipherSuite
		validation.NegotiatedALPN = probe.ALPN
		validations = append(validations, validation)
	}

	profile := candidate.profile
	profile.Name = authenticatedIngressProfileName(profile.FingerprintSHA256())
	description := fmt.Sprintf("由已认证入站 %s 客户端的 %d 个独立完整 TLS 握手自动验证生成", candidate.identity.Kind, len(candidate.connections))
	profile.Description = &description
	profile.Verification = egress.TLSFingerprintProfileVerification{
		SourceKind:             egress.TLSFingerprintProfileSourceAuthenticatedIngress,
		ClientKind:             candidate.identity.Kind,
		ClientVersion:          candidate.identity.Version,
		Runtime:                candidate.identity.Runtime,
		RuntimeVersion:         candidate.identity.RuntimeVersion,
		OperatingSystem:        candidate.identity.OperatingSystem,
		Architecture:           candidate.identity.Architecture,
		CaptureTarget:          s.validationTargets()[0],
		ObservedHost:           candidate.observedHost,
		IndependentConnections: len(candidate.connections),
		ClassifierEvidence:     append([]string(nil), candidate.identity.ClassifierEvidence...),
		ValidationStatus:       egress.TLSFingerprintProfileValidationVerified,
		ValidatorVersion:       egress.TLSFingerprintProfileValidatorVersion,
		ObservedJA3Hash:        candidate.replay.CapturedJA3Hash,
		ReplayedJA3Hash:        candidate.replay.ReplayedJA3Hash,
		FingerprintSHA256:      profile.FingerprintSHA256(),
		Issues:                 tlsFingerprintValidationFindingMessages(candidate.replay.Issues),
		Warnings:               tlsFingerprintValidationFindingMessages(candidate.replay.Warnings),
		ValidatedTargets:       validations,
		ValidatedAt:            &now,
	}
	if err := profile.Validate(); err != nil {
		s.finishCandidate(candidate.key, false, err.Error())
		return
	}
	if existing, err := s.findEquivalentProfile(ctx, profile, s.validationTargets()); err == nil && existing != nil {
		s.mu.Lock()
		s.existingProfilesMatched++
		s.mu.Unlock()
		s.finishCandidate(candidate.key, true, "")
		return
	}
	profileSlot, quotaErr := s.reserveVerifiedProfileSlot(ctx)
	if quotaErr != nil {
		s.finishCandidate(candidate.key, false, "authenticated ingress profile quota is unavailable")
		return
	}
	if !profileSlot {
		s.finishCandidate(candidate.key, false, "authenticated ingress daily verified profile limit reached")
		return
	}
	created, err := s.profiles.Create(ctx, profile)
	if err != nil {
		// 并发实例可能已经创建等价名称；重新查询后确认，不能直接吞掉数据库错误。
		if existing, listErr := s.findEquivalentProfile(ctx, profile, s.validationTargets()); listErr == nil && existing != nil {
			s.mu.Lock()
			s.existingProfilesMatched++
			s.mu.Unlock()
			s.finishCandidate(candidate.key, true, "")
			return
		}
		s.finishCandidate(candidate.key, false, fmt.Sprintf("create verified TLS profile: %v", err))
		return
	}
	if created == nil {
		s.finishCandidate(candidate.key, false, "create verified TLS profile returned no profile")
		return
	}
	s.mu.Lock()
	s.verifiedProfilesCreated++
	s.mu.Unlock()
	s.finishCandidate(candidate.key, true, "")
}

func (s *AuthenticatedIngressTLSService) reserveValidationQuota(ctx context.Context, ownerID string) (bool, error) {
	if s.quota == nil || strings.TrimSpace(ownerID) == "" {
		return false, errors.New("authenticated ingress quota store is unavailable")
	}
	date, expiresAt := authenticatedIngressQuotaWindow(time.Now())
	return s.quota.ReserveValidation(
		ctx,
		date,
		ownerID,
		expiresAt,
		s.maxValidationAttemptsPerDay(),
		s.maxValidationAttemptsPerOwner(),
	)
}

func (s *AuthenticatedIngressTLSService) reserveVerifiedProfileSlot(ctx context.Context) (bool, error) {
	if s.quota == nil {
		return false, errors.New("authenticated ingress quota store is unavailable")
	}
	date, expiresAt := authenticatedIngressQuotaWindow(time.Now())
	return s.quota.ReserveProfile(
		ctx,
		date,
		expiresAt,
		s.maxVerifiedProfilesPerDay(),
	)
}

func authenticatedIngressQuotaWindow(now time.Time) (string, time.Time) {
	utc := now.UTC()
	start := time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
	return start.Format("2006-01-02"), start.Add(25 * time.Hour)
}

func (s *AuthenticatedIngressTLSService) finishCandidate(key string, success bool, errorMessage string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	candidate := s.candidates[key]
	if candidate != nil {
		candidate.inFlight = false
		candidate.completed = success
	}
	if success {
		if candidate != nil {
			s.candidateBytes -= candidate.estimatedBytes
			delete(s.candidates, key)
		}
		if len(s.completed) < s.maxCandidateGroups() {
			s.completed[key] = time.Now()
		}
		now := time.Now()
		s.lastVerifiedAt = &now
		s.lastError = ""
		return
	}
	if candidate != nil {
		candidate.failureCount++
		delay := time.Minute << min(candidate.failureCount-1, 4)
		candidate.retryAfter = time.Now().Add(delay)
	}
	s.lastError = errorMessage
	logging.LegacyPrintf("service.authenticated_ingress_tls", "[AuthenticatedIngressTLS] verification rejected: %s", errorMessage)
}

func (s *AuthenticatedIngressTLSService) findEquivalentProfile(
	ctx context.Context,
	candidate *egress.TLSFingerprintProfile,
	targets []string,
) (*egress.TLSFingerprintProfile, error) {
	if s.profiles == nil {
		return nil, errors.New("TLS fingerprint profile service is unavailable")
	}
	profiles, err := s.profiles.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, profile := range profiles {
		if profile == nil || profile.FingerprintSHA256() != candidate.FingerprintSHA256() {
			continue
		}
		allTargetsReady := true
		for _, target := range targets {
			if !profile.IsVerifiedForTarget(target) {
				allTargetsReady = false
				break
			}
		}
		if allTargetsReady {
			return profile, nil
		}
	}
	return nil, nil
}

func (s *AuthenticatedIngressTLSService) cleanupLocked(now time.Time) {
	if !s.lastCleanup.IsZero() && now.Sub(s.lastCleanup) < authenticatedIngressCleanupInterval {
		return
	}
	pendingCutoff := now.Add(-s.pendingTTL())
	for key, value := range s.hellos {
		if value == nil || value.observation == nil || value.observation.ObservedAt.Before(pendingCutoff) {
			if value != nil {
				s.helloBytes -= value.estimatedBytes
			}
			delete(s.hellos, key)
		}
	}
	for key, value := range s.auth {
		if value == nil || value.observedAt.Before(pendingCutoff) {
			delete(s.auth, key)
		}
	}
	candidateCutoff := now.Add(-s.candidateTTL())
	for key, value := range s.candidates {
		if value == nil || (!value.inFlight && value.lastSeen.Before(candidateCutoff)) {
			if value != nil {
				s.candidateBytes -= value.estimatedBytes
			}
			delete(s.candidates, key)
		}
	}
	for key, completedAt := range s.completed {
		if completedAt.Before(candidateCutoff) {
			delete(s.completed, key)
		}
	}
	if s.helloBytes < 0 {
		s.helloBytes = 0
	}
	if s.candidateBytes < 0 {
		s.candidateBytes = 0
	}
	s.trimStaleHelloOrderLocked()
	s.lastCleanup = now
}

func (s *AuthenticatedIngressTLSService) evictOldestHelloLocked() bool {
	for s.helloOrderHead < len(s.helloOrder) {
		entry := s.helloOrder[s.helloOrderHead]
		s.helloOrderHead++
		current := s.hellos[entry.flowID]
		if current == nil || current.sequence != entry.sequence {
			continue
		}
		delete(s.hellos, entry.flowID)
		s.helloBytes -= current.estimatedBytes
		if s.helloBytes < 0 {
			s.helloBytes = 0
		}
		s.compactHelloOrderLocked()
		return true
	}
	s.compactHelloOrderLocked()
	return false
}

func (s *AuthenticatedIngressTLSService) compactHelloOrderLocked() {
	if s.helloOrderHead < 1024 || s.helloOrderHead*2 < len(s.helloOrder) {
		return
	}
	remaining := append([]authenticatedIngressHelloOrderEntry(nil), s.helloOrder[s.helloOrderHead:]...)
	s.helloOrder = remaining
	s.helloOrderHead = 0
}

func (s *AuthenticatedIngressTLSService) trimStaleHelloOrderLocked() {
	for s.helloOrderHead < len(s.helloOrder) {
		entry := s.helloOrder[s.helloOrderHead]
		current := s.hellos[entry.flowID]
		if current != nil && current.sequence == entry.sequence {
			break
		}
		s.helloOrderHead++
	}
	s.compactHelloOrderLocked()
}

func (s *AuthenticatedIngressTLSService) rebuildHelloOrderIfNeededLocked() {
	activeOrderEntries := len(s.helloOrder) - s.helloOrderHead
	if activeOrderEntries <= 2*len(s.hellos)+32 {
		return
	}
	rebuilt := make([]authenticatedIngressHelloOrderEntry, 0, len(s.hellos))
	for flowID, event := range s.hellos {
		if event != nil {
			rebuilt = append(rebuilt, authenticatedIngressHelloOrderEntry{flowID: flowID, sequence: event.sequence})
		}
	}
	sort.Slice(rebuilt, func(left, right int) bool { return rebuilt[left].sequence < rebuilt[right].sequence })
	s.helloOrder = rebuilt
	s.helloOrderHead = 0
}

func (s *AuthenticatedIngressTLSService) pendingTTL() time.Duration {
	seconds := s.cfg.PendingTTLSeconds
	if seconds <= 0 {
		seconds = 300
	}
	return time.Duration(seconds) * time.Second
}

func (s *AuthenticatedIngressTLSService) candidateTTL() time.Duration {
	seconds := s.cfg.CandidateTTLSeconds
	if seconds <= 0 {
		seconds = 86400
	}
	return time.Duration(seconds) * time.Second
}

func (s *AuthenticatedIngressTLSService) minimumConnections() int {
	value := s.cfg.MinimumIndependentConnections
	if value < 3 {
		return 3
	}
	return value
}

func (s *AuthenticatedIngressTLSService) maxPendingFlows() int {
	value := s.cfg.MaxPendingFlows
	if value <= 0 {
		return 20000
	}
	return value
}

func (s *AuthenticatedIngressTLSService) maxCandidateGroups() int {
	value := s.cfg.MaxCandidateGroups
	if value <= 0 {
		return 5000
	}
	return value
}

func (s *AuthenticatedIngressTLSService) maxValidationAttemptsPerDay() int {
	value := s.cfg.MaxValidationAttemptsPerDay
	if value <= 0 {
		return 100
	}
	return value
}

func (s *AuthenticatedIngressTLSService) maxValidationAttemptsPerOwner() int {
	value := s.cfg.MaxValidationAttemptsPerOwner
	if value <= 0 {
		return 5
	}
	return value
}

func (s *AuthenticatedIngressTLSService) maxVerifiedProfilesPerDay() int {
	value := s.cfg.MaxVerifiedProfilesPerDay
	if value <= 0 {
		return 50
	}
	return value
}

func (s *AuthenticatedIngressTLSService) allowedObservedHost(host string) bool {
	for _, allowed := range s.cfg.ObservedHosts {
		if strings.EqualFold(strings.TrimSpace(allowed), host) {
			return true
		}
	}
	return false
}

func (s *AuthenticatedIngressTLSService) validationTargets() []string {
	values := append([]string(nil), s.cfg.ValidationTargets...)
	for index := range values {
		values[index] = strings.ToLower(strings.TrimSpace(values[index]))
	}
	sort.Strings(values)
	return values
}

func (s *AuthenticatedIngressTLSService) ownerID(userID int64) string {
	key := sha256.Sum256([]byte("owner-id\x00" + s.cfg.ObserverToken))
	mac := hmac.New(sha256.New, key[:])
	_, _ = fmt.Fprintf(mac, "%d", userID)
	return hex.EncodeToString(mac.Sum(nil))
}

func authenticatedIngressCandidateKey(observedHost, fingerprintHash string, targets []string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		strings.ToLower(observedHost),
		fingerprintHash,
		strings.Join(targets, ","),
	}, "\x00")))
	return hex.EncodeToString(sum[:])
}

func authenticatedIngressProfileFromCapture(capture *tlsfingerprint.CapturedClientHello) *egress.TLSFingerprintProfile {
	return &egress.TLSFingerprintProfile{
		Name:                    "Authenticated ingress candidate",
		EnableGREASE:            capture.EnableGREASE,
		CipherSuites:            append([]uint16(nil), capture.CipherSuites...),
		Curves:                  append([]uint16(nil), capture.Curves...),
		PointFormats:            append([]uint16(nil), capture.PointFormats...),
		SignatureAlgorithms:     append([]uint16(nil), capture.SignatureAlgorithms...),
		SignatureAlgorithmsCert: append([]uint16(nil), capture.SignatureAlgorithmsCert...),
		ALPNProtocols:           append([]string(nil), capture.ALPNProtocols...),
		SupportedVersions:       append([]uint16(nil), capture.SupportedVersions...),
		KeyShareGroups:          append([]uint16(nil), capture.KeyShareGroups...),
		PSKModes:                append([]uint16(nil), capture.PSKModes...),
		Extensions:              append([]uint16(nil), capture.Extensions...),
		Verification: egress.TLSFingerprintProfileVerification{
			SourceKind:       egress.TLSFingerprintProfileSourceAuthenticatedIngress,
			ValidationStatus: egress.TLSFingerprintProfileValidationDraft,
		},
	}
}

func estimateAuthenticatedIngressClientHelloBytes(capture *tlsfingerprint.CapturedClientHello) int {
	if capture == nil {
		return 0
	}
	size := 256 + len(capture.JA3Raw) + len(capture.JA3Hash)
	size += 2 * (len(capture.RecordVersions) + len(capture.RecordLengths) + len(capture.CipherSuites) + len(capture.Curves))
	size += 2 * (len(capture.PointFormats) + len(capture.SignatureAlgorithms) + len(capture.SignatureAlgorithmsCert))
	size += 2 * (len(capture.SupportedVersions) + len(capture.KeyShareGroups) + len(capture.PSKModes) + len(capture.Extensions))
	size += len(capture.CompressionMethods)
	size += 4 * len(capture.ExtensionMetadata)
	size += 4 * len(capture.KeyShares)
	for _, protocol := range capture.ALPNProtocols {
		size += len(protocol) + 16
	}
	return size
}

func authenticatedIngressProfileName(fingerprintHash string) string {
	shortHash := fingerprintHash
	if len(shortHash) > 10 {
		shortHash = shortHash[:10]
	}
	return "Authenticated ingress " + shortHash
}

func classifyAuthenticatedIngressClient(request AuthenticatedIngressRequestSnapshot) authenticatedIngressClientIdentity {
	ua := strings.TrimSpace(request.UserAgent)
	originator := strings.TrimSpace(request.Originator)
	identity := authenticatedIngressClientIdentity{}
	if match := authenticatedIngressCodexUA.FindStringSubmatch(ua); len(match) == 3 {
		identity.Kind = "codex"
		identity.Version = normalizeAuthenticatedIngressClientVersion(match[2])
		identity.Runtime = "rust"
		identity.ClassifierEvidence = []string{"user_agent=" + strings.ToLower(match[1]), "api_key=validated"}
	} else if match := authenticatedIngressClaudeUA.FindStringSubmatch(ua); len(match) == 2 {
		identity.Kind = "claude_code"
		identity.Version = normalizeAuthenticatedIngressClientVersion(match[1])
		identity.Runtime = "node"
		identity.ClassifierEvidence = []string{"user_agent=claude-cli", "api_key=validated"}
	} else if match := authenticatedIngressNodeUA.FindStringSubmatch(ua); len(match) == 2 {
		identity.Kind = "node"
		identity.Version = normalizeAuthenticatedIngressClientVersion(match[1])
		identity.Runtime = "node"
		identity.RuntimeVersion = match[1]
		identity.ClassifierEvidence = []string{"user_agent=node", "api_key=validated"}
		return identity
	} else {
		return identity
	}
	if normalizedOriginator := normalizedAuthenticatedIngressOriginator(originator); normalizedOriginator != "" {
		identity.ClassifierEvidence = append(identity.ClassifierEvidence, "originator="+normalizedOriginator)
	}
	lowerUA := strings.ToLower(ua)
	for _, pair := range []struct{ needle, value string }{
		{"linux", "linux"},
		{"darwin", "macos"},
		{"macos", "macos"},
		{"windows", "windows"},
	} {
		if strings.Contains(lowerUA, pair.needle) {
			identity.OperatingSystem = pair.value
			break
		}
	}
	for _, pair := range []struct{ needle, value string }{
		{"x86_64", "x86_64"},
		{"amd64", "x86_64"},
		{"aarch64", "arm64"},
		{"arm64", "arm64"},
	} {
		if strings.Contains(lowerUA, pair.needle) {
			identity.Architecture = pair.value
			break
		}
	}
	return identity
}

func normalizeAuthenticatedIngressClientVersion(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 64 {
		return ""
	}
	return value
}

func normalizedAuthenticatedIngressOriginator(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "codex_cli_rs", "codex-tui", "codex_vscode", "claude-cli", "claude-code":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}
