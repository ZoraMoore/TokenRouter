package provider

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/egress"
	egressprovider "github.com/TokenFlux/TokenRouter/internal/egress/provider"
	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
	providercore "github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
	"github.com/TokenFlux/TokenRouter/internal/server/httpx"
	upstreamcore "github.com/TokenFlux/TokenRouter/internal/upstream"
	"github.com/TokenFlux/TokenRouter/internal/upstream/openai"
)

func TestCodexInviteResetServiceGetStatusAggregatesDesktopEndpoints(t *testing.T) {
	provider := &providercore.Record{
		ID:          42,
		Platform:    capability.PlatformOpenAI,
		Type:        capability.ProviderTypeOAuth,
		Concurrency: 3,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}
	upstream := &codexInviteResetHTTPUpstreamStub{responses: []*http.Response{
		codexInviteResetJSONResponse(`{"requires_explicit_confirmation":true,"should_show":true,"remaining_send_capacity":5,"remaining_reward_capacity":3,"offer_id":"reward","unrelated":"private_marker","grants":[{"recipient":"referrer","grant_type":"rate_limit_reset_credit","amount":3}],"rules":[{"text":"friend must send first Codex message"}]}`),
		codexInviteResetJSONResponse(`{"rate_limit_reset_credits":{"available_count":2}}`),
		codexInviteResetJSONResponse(`{"available_count":2,"credits":[{"id":"credit-1","status":"available","title":"Reset","profile_user_id":"private_marker","reset_type":"primary","granted_at":"2026-07-01T04:05:06Z"},{"id":"credit-2","status":"available"}]}`),
	}}
	svc := newCodexInviteForTest(codexInviteResetAdminServiceStub{provider: provider}, upstream, nil, nil, nil)

	status, err := svc.GetStatus(context.Background(), provider.ID)
	require.NoError(t, err)
	require.Equal(t, "codex_referral_consumer", status.ReferralKey)
	require.Equal(t, 2, status.AvailableCount)
	require.True(t, status.RequiresConsent)
	require.NotNil(t, status.ShouldShow)
	require.True(t, *status.ShouldShow)
	require.Equal(t, 3, *status.AvailableInvites)
	require.Equal(t, "rate_limit_reset_credit", status.GrantAction)
	require.NotNil(t, status.GrantAmount)
	require.Equal(t, 3, *status.GrantAmount)
	require.NotNil(t, status.HasRewards)
	require.True(t, *status.HasRewards)
	require.Equal(t, "rate_limit_reset", status.GrantType)
	require.Len(t, status.Credits, 2)
	public, err := json.Marshal(status)
	require.NoError(t, err)
	require.NotContains(t, string(public), "private_marker")
	require.Equal(t, "primary", status.Credits[0].ResetType)
	require.Equal(t, "2026-07-01T04:05:06Z", status.Credits[0].GrantedAt)
	require.Equal(t, "friend must send first Codex message", status.EligibilityRules[0])

	require.Len(t, upstream.requests, 3)
	require.Equal(t, "/backend-api/referrals/invite/eligibility", upstream.requests[0].URL.Path)
	require.Equal(t, "codex_referral_consumer", upstream.requests[0].URL.Query().Get("program_id"))
	require.Equal(t, "persistent", upstream.requests[0].URL.Query().Get("entrypoint"))
	require.Equal(t, "/backend-api/wham/usage", upstream.requests[1].URL.Path)
	require.Equal(t, "true", upstream.requests[1].URL.Query().Get("supports_rewardless_invites"))
	require.Equal(t, "/backend-api/wham/rate-limit-reset-credits", upstream.requests[2].URL.Path)
	require.Equal(t, "Bearer oauth-token", upstream.requests[0].Header.Get("Authorization"))
	require.Equal(t, "Codex Desktop", upstream.requests[0].Header.Get("originator"))
	require.Equal(t, openai.CodexInviteDefaultUserAgent, upstream.requests[0].Header.Get("User-Agent"))
	require.Equal(t, "1", upstream.requests[0].Header.Get("X-OpenAI-Attach-Auth"))
	require.Equal(t, "1", upstream.requests[0].Header.Get("X-OpenAI-Attach-Integrity-State"))
	require.Equal(t, "none", upstream.requests[0].Header.Get("sec-fetch-site"))
	require.Equal(t, "no-cors", upstream.requests[0].Header.Get("sec-fetch-mode"))
	require.Equal(t, "empty", upstream.requests[0].Header.Get("sec-fetch-dest"))
	require.Equal(t, "u=4, i", upstream.requests[0].Header.Get("priority"))
	require.Equal(t, "chatgpt-acc", upstream.requests[0].Header.Get("chatgpt-account-id"))
	require.Equal(t, upstreamcore.HTTPUpstreamProfileOpenAI, upstreamcore.HTTPUpstreamProfileFromContext(upstream.requests[0].Context()))
}

func TestCodexInviteResetServiceGetStatusKeepsUsageCreditsWhenEligibilityReturns422AndDetailsFail(t *testing.T) {
	provider := &providercore.Record{
		ID:       50,
		Platform: capability.PlatformOpenAI,
		Type:     capability.ProviderTypeOAuth,
		Credentials: map[string]any{
			"access_token": "oauth-token",
		},
	}
	upstream := &codexInviteResetHTTPUpstreamStub{responses: []*http.Response{
		codexInviteResetJSONStatusResponse(http.StatusUnprocessableEntity, `{"detail":[{"type":"missing","loc":["query","legacy_field"],"msg":"Field required"}]}`),
		codexInviteResetJSONResponse(`{"rate_limit_reset_credits":{"available_count":1}}`),
		codexInviteResetJSONStatusResponse(http.StatusUnauthorized, `{"detail":"expired"}`),
	}}
	svc := newCodexInviteForTest(codexInviteResetAdminServiceStub{provider: provider}, upstream, nil, nil, nil)

	status, err := svc.GetStatus(context.Background(), provider.ID)
	require.NoError(t, err)
	require.False(t, status.InviteAvailable)
	require.Equal(t, "CODEX_INVITE_ELIGIBILITY_UNKNOWN", status.InviteUnavailableReason)
	require.Equal(t, "暂时无法查询邀请资格，已有重置次数仍可使用", status.InviteUnavailableMessage)
	require.Equal(t, 1, status.AvailableCount)
	require.Empty(t, status.Credits)
	require.NotContains(t, status.InviteUnavailableMessage, "Field required")
	require.Len(t, upstream.requests, 3)
	require.Equal(t, "/backend-api/wham/usage", upstream.requests[1].URL.Path)
	require.Equal(t, "/backend-api/wham/rate-limit-reset-credits", upstream.requests[2].URL.Path)
}

func TestCodexInviteResetServiceSendInviteNormalizesEmails(t *testing.T) {
	provider := &providercore.Record{
		ID:          7,
		Platform:    capability.PlatformOpenAI,
		Type:        capability.ProviderTypeOAuth,
		Credentials: map[string]any{"access_token": "oauth-token"},
	}
	upstream := &codexInviteResetHTTPUpstreamStub{responses: []*http.Response{
		codexInviteResetJSONResponse(`{"should_show":true,"remaining_send_capacity":2}`),
		codexInviteResetJSONResponse(`{"invites":[{"email":"a@example.com"},{"email":"b@example.com"}],"message":"ok"}`),
	}}
	svc := newCodexInviteForTest(codexInviteResetAdminServiceStub{provider: provider}, upstream, nil, nil, nil)

	result, err := svc.SendInvite(context.Background(), provider.ID, providercore.CodexInviteRequest{Emails: []string{"a@example.com, b@example.com", "A@example.com"}, ProgramID: "codex_referral_consumer", Confirmed: true})
	require.NoError(t, err)
	require.Equal(t, 2, result.SentCount)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "/backend-api/referrals/invite", upstream.requests[1].URL.Path)

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(upstream.bodies[1]), &payload))
	require.Equal(t, "codex_referral_consumer", payload["program_id"])
	require.Equal(t, "persistent", payload["entrypoint"])
	require.Equal(t, []any{"a@example.com", "b@example.com"}, payload["emails"])
}

func TestCodexInviteResetServiceSendInviteMapsUnavailableInvite(t *testing.T) {
	provider := &providercore.Record{
		ID:          8,
		Platform:    capability.PlatformOpenAI,
		Type:        capability.ProviderTypeOAuth,
		Credentials: map[string]any{"access_token": "oauth-token"},
	}
	upstream := &codexInviteResetHTTPUpstreamStub{responses: []*http.Response{
		codexInviteResetJSONResponse(`{"should_show":true,"remaining_send_capacity":2}`),
		codexInviteResetJSONStatusResponse(http.StatusForbidden, `{"detail":"该推荐码对应的推荐邀请不可用"}`),
	}}
	svc := newCodexInviteForTest(codexInviteResetAdminServiceStub{provider: provider}, upstream, nil, nil, nil)

	result, err := svc.SendInvite(context.Background(), provider.ID, providercore.CodexInviteRequest{Emails: []string{"a@example.com"}, ProgramID: "codex_referral_consumer", Confirmed: true})
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, http.StatusForbidden, httpx.ErrorCode(err))
	require.Equal(t, "CODEX_INVITE_RESET_REFERRAL_UNAVAILABLE", apperror.Reason(err))
	require.Equal(t, "当前 Codex 推荐邀请入口暂不可用，但已有重置次数仍可使用", apperror.Message(err))
	require.Empty(t, apperror.FromError(err).Metadata)
}

func TestCodexInviteResetServiceConsumeSendsRedeemRequestID(t *testing.T) {
	provider := &providercore.Record{
		ID:          9,
		Platform:    capability.PlatformOpenAI,
		Type:        capability.ProviderTypeOAuth,
		Credentials: map[string]any{"access_token": "oauth-token"},
	}
	upstream := &codexInviteResetHTTPUpstreamStub{responses: []*http.Response{
		codexInviteResetJSONResponse(`{"code":"reset","available_count":0,"windows_reset":2}`),
	}}
	svc := newCodexInviteForTest(codexInviteResetAdminServiceStub{provider: provider}, upstream, nil, nil, nil)

	result, err := svc.Consume(context.Background(), provider.ID, "credit-1")
	require.NoError(t, err)
	require.Equal(t, "reset", result.Code)
	require.Equal(t, "credit-1", result.CreditID)
	require.NotEmpty(t, result.RedeemRequestID)
	require.Equal(t, 2, result.WindowsReset)
	require.NotNil(t, result.AvailableCount)
	require.Equal(t, 0, *result.AvailableCount)

	var payload map[string]string
	require.NoError(t, json.Unmarshal([]byte(upstream.bodies[0]), &payload))
	require.Equal(t, "credit-1", payload["credit_id"])
	require.Equal(t, result.RedeemRequestID, payload["redeem_request_id"])
}

func TestCodexInviteResetServiceConsumeAllowsAutomaticCreditSelection(t *testing.T) {
	provider := &providercore.Record{
		ID:          10,
		Platform:    capability.PlatformOpenAI,
		Type:        capability.ProviderTypeOAuth,
		Credentials: map[string]any{"access_token": "oauth-token"},
	}
	upstream := &codexInviteResetHTTPUpstreamStub{responses: []*http.Response{
		codexInviteResetJSONResponse(`{"code":"reset","windows_reset":1}`),
	}}
	svc := newCodexInviteForTest(codexInviteResetAdminServiceStub{provider: provider}, upstream, nil, nil, nil)

	result, err := svc.Consume(context.Background(), provider.ID, "")
	require.NoError(t, err)
	require.Empty(t, result.CreditID)
	require.Equal(t, 1, result.WindowsReset)

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(upstream.bodies[0]), &payload))
	require.NotEmpty(t, payload["redeem_request_id"])
	require.NotContains(t, payload, "credit_id")
}

func TestCodexInviteResetServiceUsesTLSRouterInviteResetUserAgent(t *testing.T) {
	provider := &providercore.Record{
		ID:          43,
		Platform:    capability.PlatformOpenAI,
		Type:        capability.ProviderTypeOAuth,
		Credentials: map[string]any{"access_token": "oauth-token"},
		Extra: map[string]any{
			"tls_fingerprint_router_id": int64(9),
		},
	}
	upstream := &codexInviteResetHTTPUpstreamStub{responses: []*http.Response{
		codexInviteResetJSONResponse(`{"requires_explicit_confirmation":true}`),
		codexInviteResetJSONResponse(`{"available_count":0,"credits":[]}`),
	}}
	routerReader := &openAIOAuthTokenRouterReaderStub{routers: map[int64]*egress.TLSFingerprintRouter{
		9: {
			ID:                        9,
			Enabled:                   true,
			CodexInviteResetUserAgent: " Codex Desktop/0.135.0-alpha.1 (Windows 10.0.26200; x86_64) ",
		},
	}}
	svc := newCodexInviteForTest(codexInviteResetAdminServiceStub{provider: provider}, upstream, nil, nil, routerReader)

	_, err := svc.GetStatus(context.Background(), provider.ID)
	require.NoError(t, err)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "Codex Desktop/0.135.0-alpha.1 (Windows 10.0.26200; x86_64)", upstream.requests[0].Header.Get("User-Agent"))
}

func TestCodexInviteResetServiceDoesNotReuseTokenUserAgent(t *testing.T) {
	provider := &providercore.Record{
		ID:          45,
		Platform:    capability.PlatformOpenAI,
		Type:        capability.ProviderTypeOAuth,
		Credentials: map[string]any{"access_token": "oauth-token"},
		Extra: map[string]any{
			"tls_fingerprint_router_id": int64(9),
		},
	}
	upstream := &codexInviteResetHTTPUpstreamStub{responses: []*http.Response{
		codexInviteResetJSONResponse(`{"requires_explicit_confirmation":true}`),
		codexInviteResetJSONResponse(`{"available_count":0,"credits":[]}`),
	}}
	routerReader := &openAIOAuthTokenRouterReaderStub{routers: map[int64]*egress.TLSFingerprintRouter{
		9: {
			ID:                         9,
			Enabled:                    true,
			ChatGPTOAuthTokenUserAgent: "codex-tui/0.135.0 (Windows 10.0.26200; x86_64)",
		},
	}}
	svc := newCodexInviteForTest(codexInviteResetAdminServiceStub{provider: provider}, upstream, nil, nil, routerReader)

	_, err := svc.GetStatus(context.Background(), provider.ID)
	require.NoError(t, err)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, openai.CodexInviteDefaultUserAgent, upstream.requests[0].Header.Get("User-Agent"))
}

func TestCodexInviteResetServiceUsesTLSRouterInviteResetTLSProfile(t *testing.T) {
	inviteResetProfileID := int64(20)
	provider := &providercore.Record{
		ID:          44,
		Platform:    capability.PlatformOpenAI,
		Type:        capability.ProviderTypeOAuth,
		Credentials: map[string]any{"access_token": "oauth-token"},
		Extra: map[string]any{
			"enable_tls_fingerprint":     true,
			"tls_fingerprint_profile_id": int64(10),
			"tls_fingerprint_router_id":  int64(9),
		},
	}
	upstream := &codexInviteResetHTTPUpstreamStub{responses: []*http.Response{
		codexInviteResetJSONResponse(`{"requires_explicit_confirmation":true}`),
		codexInviteResetJSONResponse(`{"available_count":0,"credits":[]}`),
	}}
	routerReader := &openAIOAuthTokenRouterReaderStub{routers: map[int64]*egress.TLSFingerprintRouter{
		9: {
			ID:                                      9,
			Enabled:                                 true,
			CodexInviteResetTLSFingerprintProfileID: &inviteResetProfileID,
		},
	}}
	profileService := newTLSProfileServiceWithCacheForTest(map[int64]*egress.TLSFingerprintProfile{
		10: {ID: 10, Name: "provider-fixed"},
		20: {ID: 20, Name: "router-token"},
	})
	svc := newCodexInviteForTest(codexInviteResetAdminServiceStub{provider: provider}, upstream, nil, profileService, routerReader)

	_, err := svc.GetStatus(context.Background(), provider.ID)
	require.NoError(t, err)
	require.Len(t, upstream.profiles, 2)
	require.NotNil(t, upstream.profiles[0])
	require.Equal(t, "router-token", upstream.profiles[0].Name)
}

// newCodexInviteForTest 将传输替身接入邀请重置服务与平台客户端。
func newCodexInviteForTest(reader codexInviteResetAdminServiceStub, transport QoderTransport, token *providercore.OpenAITokenSource, profiles *egressprovider.TLSProfiles, routers OpenAITokenRouterReader) *providercore.CodexInviteResetService {
	factory := CodexInviteFactory{Token: token, Proxy: reader.GetProxy, Transport: transport, Profiles: profiles, Routers: routers}
	return &providercore.CodexInviteResetService{Options: providercore.CodexInviteResetOptions{
		Read: reader.GetProvider, Client: factory.Client, NewID: uuid.NewString, Warn: slog.Warn,
	}}
}

// TestCodexInviteResetServiceEnforcesCurrentEligibility 检查名额与明确同意，未通过时只查询资格。
func TestCodexInviteResetServiceEnforcesCurrentEligibility(t *testing.T) {
	for _, tc := range []struct {
		name, eligibility, program, plan, reason string
		confirmed                                bool
		reads                                    int
	}{
		{"needsConsent", `{"should_show":true,"remaining_send_capacity":3}`, "codex_referral_consumer", "", "CODEX_INVITE_CONSENT_REQUIRED", false, 1},
		{"noCapacity", `{"should_show":true,"remaining_send_capacity":0}`, "codex_referral_consumer", "", "CODEX_INVITE_RESET_REFERRAL_UNAVAILABLE", true, 1},
		{"noRewardCapacity", `{"should_show":true,"remaining_send_capacity":3,"grants":[{"grant_type":"workspace_credits"}],"remaining_reward_capacity":0}`, "codex_referral_workspace", "business", "CODEX_INVITE_RESET_REFERRAL_UNAVAILABLE", true, 1},
		{"unknownCapacity", `{"should_show":true,"remaining_send_capacity":3,"offer_id":"credits"}`, "codex_referral_consumer", "", "CODEX_INVITE_RESET_REFERRAL_UNAVAILABLE", true, 1},
		{"unstated", `{"should_show":true,"remaining_send_capacity":1.5}`, "codex_referral_consumer", "", "CODEX_INVITE_RESET_REFERRAL_UNAVAILABLE", true, 1},
		{"programChanged", "", "codex_referral_consumer", "edu", "CODEX_INVITE_PROGRAM_CHANGED", true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &providercore.Record{ID: 7, Platform: capability.PlatformOpenAI, Type: capability.ProviderTypeOAuth, Credentials: map[string]any{"access_token": "token", "plan_type": tc.plan}}
			up := &codexInviteResetHTTPUpstreamStub{responses: []*http.Response{codexInviteResetJSONResponse(tc.eligibility)}}
			s := newCodexInviteForTest(codexInviteResetAdminServiceStub{provider: p}, up, nil, nil, nil)
			result, err := s.SendInvite(t.Context(), p.ID, providercore.CodexInviteRequest{Emails: []string{"friend@example.com"}, ProgramID: tc.program, Confirmed: tc.confirmed})
			require.Nil(t, result)
			require.Equal(t, tc.reason, apperror.Reason(err))
			require.Len(t, up.requests, tc.reads)
			if tc.reads == 1 {
				require.Equal(t, http.MethodGet, up.requests[0].Method)
				require.Equal(t, tc.program, up.requests[0].URL.Query().Get("program_id"))
			}
		})
	}
}

func TestCodexInviteResetServiceUncertainSendDoesNotRetryOrExposeResponse(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"missing", `{"message":"private_recipient"}`, 200},
		{"nullInvite", `{"invites":[null]}`, 200},
		{"foreignFailure", `{"failed_emails":["private_recipient@example.com"]}`, 200},
		{"unavailable", `{"detail":"private_recipient"}`, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &providercore.Record{ID: 7, Platform: capability.PlatformOpenAI, Type: capability.ProviderTypeOAuth, Credentials: map[string]any{"access_token": "token"}}
			up := &codexInviteResetHTTPUpstreamStub{responses: []*http.Response{codexInviteResetJSONResponse(`{"should_show":true,"remaining_send_capacity":1,"requires_explicit_confirmation":false}`), codexInviteResetJSONStatusResponse(tc.status, tc.body)}}
			s := newCodexInviteForTest(codexInviteResetAdminServiceStub{provider: p}, up, nil, nil, nil)
			result, err := s.SendInvite(t.Context(), p.ID, providercore.CodexInviteRequest{Emails: []string{"friend@example.com"}, ProgramID: "codex_referral_consumer"})
			require.Nil(t, result)
			require.Equal(t, "CODEX_INVITE_SEND_UNKNOWN", apperror.Reason(err))
			require.NotContains(t, err.Error(), "private_recipient")
			require.Empty(t, apperror.FromError(err).Metadata)
			require.Len(t, up.requests, 2)
			require.Equal(t, http.MethodPost, up.requests[1].Method)
		})
	}
}

func TestCodexInviteResetServiceRejectsUnsupportedAndShadowAccounts(t *testing.T) {
	parentID := int64(1)
	for _, p := range []*providercore.Record{
		{ID: 2, Platform: capability.PlatformOpenAI, Type: capability.ProviderTypeOAuth, ParentProviderID: &parentID},
		{ID: 3, Platform: capability.PlatformOpenAI, Type: capability.ProviderTypeAPIKey},
	} {
		up := &codexInviteResetHTTPUpstreamStub{}
		s := newCodexInviteForTest(codexInviteResetAdminServiceStub{provider: p}, up, nil, nil, nil)
		_, err := s.GetStatus(t.Context(), p.ID)
		require.Error(t, err)
		_, err = s.SendInvite(t.Context(), p.ID, providercore.CodexInviteRequest{Emails: []string{"friend@example.com"}})
		require.Error(t, err)
		_, err = s.Consume(t.Context(), p.ID, "")
		require.Error(t, err)
		require.Empty(t, up.requests)
	}
}
