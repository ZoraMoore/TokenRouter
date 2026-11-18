package provider

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/egress"
	"github.com/TokenFlux/TokenRouter/internal/infra/httpclient/tlsfingerprint"
	"github.com/TokenFlux/TokenRouter/internal/pkg/authenticatedingress"
)

const (
	authenticatedIngressTestEdgeToken     = "edge-token-0123456789abcdef0123456789abcdef"
	authenticatedIngressTestObserverToken = "observer-token-0123456789abcdef0123456789abcdef"
)

type authenticatedIngressTestQuotaStore struct {
	mu          sync.Mutex
	validations map[string]int
	owners      map[string]int
	profiles    map[string]int
}

type authenticatedIngressProfileRepo struct {
	mu       sync.Mutex
	profiles []*egress.TLSFingerprintProfile
}

func TestAuthenticatedIngressTLSCreatesVerifiedProfileWithoutClientConfiguration(t *testing.T) {
	repo := &authenticatedIngressProfileRepo{}
	profiles := egress.NewTLSFingerprintProfileService(repo, nil)
	profiles.Start()
	t.Cleanup(profiles.Stop)
	svc := NewAuthenticatedIngressTLSService(authenticatedIngressTestConfig(), profiles, newAuthenticatedIngressTestQuotaStore())
	var probeCount atomic.Int32
	svc.probeTLSHandshake = func(_ context.Context, _ *tlsfingerprint.Profile, target string) (*tlsfingerprint.TLSHandshakeResult, error) {
		probeCount.Add(1)
		require.Equal(t, "chatgpt.com:443", target)
		return &tlsfingerprint.TLSHandshakeResult{Version: 0x0304, CipherSuite: 0x1301, ALPN: "h2"}, nil
	}
	t.Cleanup(svc.Stop)

	captured, err := tlsfingerprint.ParseCapturedClientHelloRecords(buildControlledTLSFingerprintClientHelloRecord())
	require.NoError(t, err)
	identityProof := AuthenticatedIngressIdentity{APIKeyID: 77, UserID: 88}

	for index := range 3 {
		sourcePort := uint16(41000 + index)
		flowID, flowErr := tlsfingerprint.BuildAuthenticatedIngressFlowID(
			authenticatedIngressTestObserverToken,
			"198.51.100.10",
			sourcePort,
			"15.204.66.161",
			443,
		)
		require.NoError(t, flowErr)
		require.NoError(t, svc.AcceptObservation(&tlsfingerprint.AuthenticatedIngressObservation{
			SchemaVersion: tlsfingerprint.AuthenticatedIngressObservationSchemaVersion,
			FlowID:        flowID,
			ObservedAt:    time.Now(),
			ServerName:    "api.scienceedu.me",
			ClientHello:   captured,
		}))

		request, requestErr := http.NewRequest(http.MethodPost, "http://tokenrouter.local/v1/responses", nil)
		require.NoError(t, requestErr)
		request.Header.Set("User-Agent", "codex_cli_rs/0.200.1 (Linux; x86_64)")
		setAuthenticatedIngressTestHeaders(request, sourcePort, 1)
		svc.ObserveAuthenticatedIngressRequest(takeAuthenticatedIngressRequestSnapshot(request), identityProof)
		require.Empty(t, request.Header.Get(authenticatedIngressEdgeTokenHeader), "内部关联头必须在进入业务转发前删除")
	}

	require.Eventually(t, func() bool {
		return len(repo.list()) == 1
	}, 3*time.Second, 10*time.Millisecond)
	created := repo.list()[0]
	require.Equal(t, egress.TLSFingerprintProfileSourceAuthenticatedIngress, created.Verification.SourceKind)
	require.Equal(t, egress.TLSFingerprintProfileValidationVerified, created.Verification.ValidationStatus)
	require.Equal(t, "api.scienceedu.me", created.Verification.ObservedHost)
	require.Equal(t, 3, created.Verification.IndependentConnections)
	require.Equal(t, "codex", created.Verification.ClientKind)
	require.Equal(t, "0.200.1", created.Verification.ClientVersion)
	require.True(t, created.IsVerifiedForTarget("chatgpt.com"))
	require.False(t, created.IsOutboundReadyForTarget("chatgpt.com"), "自动模板必须由管理员批准后才能绑定")
	require.NotNil(t, created.Description)
	require.NotContains(t, *created.Description, "77")

	// 同一 TLS 结构即使 User-Agent 版本变化也只对应一个全局模板与一次目标验证。
	for index := range 3 {
		sourcePort := uint16(45000 + index)
		flowID, flowErr := tlsfingerprint.BuildAuthenticatedIngressFlowID(
			authenticatedIngressTestObserverToken, "198.51.100.10", sourcePort, "15.204.66.161", 443,
		)
		require.NoError(t, flowErr)
		require.NoError(t, svc.AcceptObservation(&tlsfingerprint.AuthenticatedIngressObservation{
			SchemaVersion: tlsfingerprint.AuthenticatedIngressObservationSchemaVersion,
			FlowID:        flowID,
			ObservedAt:    time.Now(),
			ServerName:    "api.scienceedu.me",
			ClientHello:   captured,
		}))
		request, requestErr := http.NewRequest(http.MethodPost, "http://tokenrouter.local/v1/responses", nil)
		require.NoError(t, requestErr)
		request.Header.Set("User-Agent", "codex_cli_rs/9.9."+strconv.Itoa(index))
		setAuthenticatedIngressTestHeaders(request, sourcePort, 1)
		svc.ObserveAuthenticatedIngressRequest(takeAuthenticatedIngressRequestSnapshot(request), identityProof)
	}
	require.Eventually(t, func() bool { return svc.Status().MatchedFlows == 6 }, time.Second, 10*time.Millisecond)
	require.Equal(t, int32(1), probeCount.Load())
	require.Len(t, repo.list(), 1)
}

func TestAuthenticatedIngressTLSRejectsUntrustedOrRepeatedRequest(t *testing.T) {
	svc := NewAuthenticatedIngressTLSService(authenticatedIngressTestConfig(), egress.NewTLSFingerprintProfileService(&authenticatedIngressProfileRepo{}, nil), nil)
	request, err := http.NewRequest(http.MethodPost, "http://tokenrouter.local/v1/responses", nil)
	require.NoError(t, err)
	request.Header.Set("User-Agent", "codex_cli_rs/0.200.1")
	setAuthenticatedIngressTestHeaders(request, 42000, 2)
	svc.ObserveAuthenticatedIngressRequest(takeAuthenticatedIngressRequestSnapshot(request), AuthenticatedIngressIdentity{APIKeyID: 1, UserID: 2})
	require.Zero(t, svc.Status().PendingAuthenticatedFlows)

	setAuthenticatedIngressTestHeaders(request, 42000, 1)
	request.Header.Set(authenticatedIngressEdgeTokenHeader, "untrusted")
	svc.ObserveAuthenticatedIngressRequest(takeAuthenticatedIngressRequestSnapshot(request), AuthenticatedIngressIdentity{APIKeyID: 1, UserID: 2})
	require.Zero(t, svc.Status().PendingAuthenticatedFlows)

	setAuthenticatedIngressTestHeaders(request, 42001, 1)
	request.Header.Set(authenticatedIngressResolvedAddressHeader, "203.0.113.99")
	svc.ObserveAuthenticatedIngressRequest(takeAuthenticatedIngressRequestSnapshot(request), AuthenticatedIngressIdentity{APIKeyID: 1, UserID: 2})
	require.Zero(t, svc.Status().PendingAuthenticatedFlows)
	require.Contains(t, svc.Status().LastError, "edge-reterminated")
}

func TestAuthenticatedIngressTLSRejectsResumptionObservation(t *testing.T) {
	svc := NewAuthenticatedIngressTLSService(authenticatedIngressTestConfig(), egress.NewTLSFingerprintProfileService(&authenticatedIngressProfileRepo{}, nil), nil)
	flowID, err := tlsfingerprint.BuildAuthenticatedIngressFlowID(
		authenticatedIngressTestObserverToken,
		"198.51.100.11", 43000, "15.204.66.161", 443,
	)
	require.NoError(t, err)
	err = svc.AcceptObservation(&tlsfingerprint.AuthenticatedIngressObservation{
		SchemaVersion: tlsfingerprint.AuthenticatedIngressObservationSchemaVersion,
		FlowID:        flowID,
		ObservedAt:    time.Now(),
		ServerName:    "api.scienceedu.me",
		ClientHello:   &tlsfingerprint.CapturedClientHello{HasPreSharedKey: true},
	})
	require.ErrorContains(t, err, "resumption")
}

func TestAuthenticatedIngressTLSRepeatedFlowKeepsBoundedAccounting(t *testing.T) {
	cfg := authenticatedIngressTestConfig()
	(*cfg).MaxPendingFlows = 2
	svc := NewAuthenticatedIngressTLSService(cfg, egress.NewTLSFingerprintProfileService(&authenticatedIngressProfileRepo{}, nil), nil)
	t.Cleanup(svc.Stop)
	captured, err := tlsfingerprint.ParseCapturedClientHelloRecords(buildControlledTLSFingerprintClientHelloRecord())
	require.NoError(t, err)

	makeObservation := func(port uint16) *tlsfingerprint.AuthenticatedIngressObservation {
		flowID, flowErr := tlsfingerprint.BuildAuthenticatedIngressFlowID(
			authenticatedIngressTestObserverToken, "198.51.100.20", port, "15.204.66.161", 443,
		)
		require.NoError(t, flowErr)
		return &tlsfingerprint.AuthenticatedIngressObservation{
			SchemaVersion: tlsfingerprint.AuthenticatedIngressObservationSchemaVersion,
			FlowID:        flowID,
			ObservedAt:    time.Now(),
			ServerName:    "api.scienceedu.me",
			ClientHello:   captured,
		}
	}
	require.NoError(t, svc.AcceptObservation(makeObservation(43999)))
	first := makeObservation(44000)
	for range 50 {
		first.ObservedAt = time.Now()
		require.NoError(t, svc.AcceptObservation(first))
	}
	require.NoError(t, svc.AcceptObservation(makeObservation(44001)), "满容量时应以 FIFO 淘汰旧匿名 pending，而不是突破上限")

	svc.mu.Lock()
	defer svc.mu.Unlock()
	require.Len(t, svc.hellos, 2)
	require.LessOrEqual(t, svc.helloBytes, 2*estimateAuthenticatedIngressClientHelloBytes(captured))
	require.LessOrEqual(t, len(svc.helloOrder)-svc.helloOrderHead, 2*len(svc.hellos)+32)
}

func TestAuthenticatedIngressTLSQuotaIsSharedAcrossInstances(t *testing.T) {
	quota := newAuthenticatedIngressTestQuotaStore()
	cfg := authenticatedIngressTestConfig()
	(*cfg).MaxValidationAttemptsPerDay = 2
	(*cfg).MaxValidationAttemptsPerOwner = 1
	(*cfg).MaxVerifiedProfilesPerDay = 1
	first := NewAuthenticatedIngressTLSService(cfg, egress.NewTLSFingerprintProfileService(&authenticatedIngressProfileRepo{}, nil), quota)
	second := NewAuthenticatedIngressTLSService(cfg, egress.NewTLSFingerprintProfileService(&authenticatedIngressProfileRepo{}, nil), quota)
	t.Cleanup(first.Stop)
	t.Cleanup(second.Stop)

	allowed, err := first.reserveValidationQuota(context.Background(), "owner-a")
	require.NoError(t, err)
	require.True(t, allowed)
	allowed, err = second.reserveValidationQuota(context.Background(), "owner-a")
	require.NoError(t, err)
	require.False(t, allowed, "单 owner 配额必须跨实例共享")
	allowed, err = second.reserveValidationQuota(context.Background(), "owner-b")
	require.NoError(t, err)
	require.True(t, allowed)
	allowed, err = first.reserveValidationQuota(context.Background(), "owner-c")
	require.NoError(t, err)
	require.False(t, allowed, "全局配额必须跨实例共享")

	allowed, err = first.reserveVerifiedProfileSlot(context.Background())
	require.NoError(t, err)
	require.True(t, allowed)
	allowed, err = second.reserveVerifiedProfileSlot(context.Background())
	require.NoError(t, err)
	require.False(t, allowed, "模板创建配额必须跨实例共享")
}

func authenticatedIngressTestConfig() *authenticatedingress.Config {
	return &authenticatedingress.Config{
		Enabled:                       true,
		EdgeToken:                     authenticatedIngressTestEdgeToken,
		ObserverToken:                 authenticatedIngressTestObserverToken,
		ObservedHosts:                 []string{"api.scienceedu.me"},
		ValidationTargets:             []string{"chatgpt.com"},
		PendingTTLSeconds:             300,
		CandidateTTLSeconds:           3600,
		MinimumIndependentConnections: 3,
		MaxPendingFlows:               100,
		MaxCandidateGroups:            10,
		AutoCreateVerified:            true,
	}
}

func setAuthenticatedIngressTestHeaders(request *http.Request, sourcePort uint16, requestNumber int) {
	request.Header.Set(authenticatedIngressEdgeTokenHeader, authenticatedIngressTestEdgeToken)
	request.Header.Set(authenticatedIngressConnectionIDHeader, "connection-"+strconv.Itoa(int(sourcePort)))
	request.Header.Set(authenticatedIngressConnectionRequestsHeader, strconv.Itoa(requestNumber))
	request.Header.Set(authenticatedIngressRemoteAddressHeader, "198.51.100.10")
	request.Header.Set(authenticatedIngressResolvedAddressHeader, "198.51.100.10")
	request.Header.Set(authenticatedIngressRemotePortHeader, strconv.Itoa(int(sourcePort)))
	request.Header.Set(authenticatedIngressServerAddressHeader, "15.204.66.161")
	request.Header.Set(authenticatedIngressServerPortHeader, "443")
	request.Header.Set(authenticatedIngressSessionReusedHeader, ".")
	request.Header.Set(authenticatedIngressALPNHeader, "h2")
	request.Header.Set(authenticatedIngressHTTPProtocolHeader, "HTTP/2.0")
}

func newAuthenticatedIngressTestQuotaStore() *authenticatedIngressTestQuotaStore {
	return &authenticatedIngressTestQuotaStore{
		validations: make(map[string]int),
		owners:      make(map[string]int),
		profiles:    make(map[string]int),
	}
}

func (s *authenticatedIngressTestQuotaStore) ReserveValidation(_ context.Context, date, ownerID string, _ time.Time, globalLimit, ownerLimit int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.validations[date] >= globalLimit || s.owners[date+":"+ownerID] >= ownerLimit {
		return false, nil
	}
	s.validations[date]++
	s.owners[date+":"+ownerID]++
	return true, nil
}

func (s *authenticatedIngressTestQuotaStore) ReserveProfile(_ context.Context, date string, _ time.Time, limit int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.profiles[date] >= limit {
		return false, nil
	}
	s.profiles[date]++
	return true, nil
}

func (r *authenticatedIngressProfileRepo) List(context.Context) ([]*egress.TLSFingerprintProfile, error) {
	return r.list(), nil
}

func (r *authenticatedIngressProfileRepo) GetByID(context.Context, int64) (*egress.TLSFingerprintProfile, error) {
	return nil, nil
}

func (r *authenticatedIngressProfileRepo) Create(_ context.Context, profile *egress.TLSFingerprintProfile) (*egress.TLSFingerprintProfile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	clone := *profile
	clone.ID = int64(len(r.profiles) + 1)
	r.profiles = append(r.profiles, &clone)
	return &clone, nil
}

func (r *authenticatedIngressProfileRepo) Update(_ context.Context, profile *egress.TLSFingerprintProfile) (*egress.TLSFingerprintProfile, error) {
	return profile, nil
}

func (r *authenticatedIngressProfileRepo) Delete(context.Context, int64) error {
	return nil
}

func (r *authenticatedIngressProfileRepo) list() []*egress.TLSFingerprintProfile {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*egress.TLSFingerprintProfile(nil), r.profiles...)
}
