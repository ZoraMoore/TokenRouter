package provider

import (
	"context"
	"errors"
	"net/http"
	"strings"

	forwardcore "github.com/TokenFlux/TokenRouter/internal/gateway/forward"
	"github.com/TokenFlux/TokenRouter/internal/gateway/requeststate"
	providercore "github.com/TokenFlux/TokenRouter/internal/provider"
)

// RequestCredentials 读取执行凭据并处理 Grok 请求级凭据恢复，app 注入依赖。
type RequestCredentials struct {
	Source             *providercore.OpenAIExecutionCredentials
	HasGrokTokenSource bool
	Recovery           *providercore.GrokCredentialRecovery
	Runtime            *providercore.RuntimeBlockState
}

// CredentialObserver 只接收本次脱敏分类，HTTP Adapter 把它关联到现有 Ops 请求。
type CredentialObserver interface {
	ObserveCredentialFailure(int64, forwardcore.GrokCredentialFailure)
}

// @project-doc docs/interfaces/grok_upstream.md#grok_account_contract
func (s *RequestCredentials) Resolve(ctx context.Context, state *requeststate.CredentialBudget, output CredentialObserver, provider *ExecutionProvider) (string, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if provider == nil {
		return "", "", errors.New("provider is nil")
	}
	if !provider.View().IsGrokOAuth() {
		return s.Source.Resolve(ctx, ExecutionRecord(provider))
	}
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	if s == nil || !s.HasGrokTokenSource {
		return "", "", credentialFailover(output, provider, forwardcore.GrokCredentialFailure{
			Scope:   forwardcore.GatewayFailureScopeShared,
			Reason:  forwardcore.GrokCredentialReasonProviderConfig,
			Action:  forwardcore.NextProviderStop,
			Message: "Grok OAuth credential provider is unavailable",
		})
	}
	if s.blocked(provider) {
		return "", "", credentialFailover(output, provider, forwardcore.GrokCredentialFailure{
			Scope:   forwardcore.GatewayFailureScopeProvider,
			Reason:  forwardcore.GrokCredentialReasonProviderChanged,
			Action:  forwardcore.NextProviderRetry,
			Message: "Grok OAuth provider is not currently schedulable",
		})
	}

	credentialCtx, cancel, budgetExpired := state.Acquire(ctx)
	if cancel != nil {
		defer cancel()
	}
	if budgetExpired {
		return "", "", credentialFailover(output, provider, forwardcore.GrokCredentialFailure{
			Scope:   forwardcore.GatewayFailureScopeRequest,
			Reason:  forwardcore.GrokCredentialReasonFailoverTimeout,
			Action:  forwardcore.NextProviderStop,
			Message: "Grok OAuth credential failover budget exhausted",
		})
	}

	token, kind, err := s.Source.Resolve(credentialCtx, ExecutionRecord(provider))
	if err == nil {
		if s.blocked(provider) {
			return "", "", credentialFailover(output, provider, forwardcore.GrokCredentialFailure{
				Scope:   forwardcore.GatewayFailureScopeProvider,
				Reason:  forwardcore.GrokCredentialReasonProviderChanged,
				Action:  forwardcore.NextProviderRetry,
				Message: "Grok OAuth provider is not currently schedulable",
			})
		}
		return token, kind, nil
	}
	if parentErr := ctx.Err(); parentErr != nil {
		return "", "", parentErr
	}
	if credentialCtx.Err() != nil {
		return "", "", credentialFailover(output, provider, forwardcore.GrokCredentialFailure{
			Scope:   forwardcore.GatewayFailureScopeRequest,
			Reason:  forwardcore.GrokCredentialReasonFailoverTimeout,
			Action:  forwardcore.NextProviderStop,
			Message: "Grok OAuth credential failover budget exhausted",
		})
	}

	class := forwardcore.ClassifyGrokCredentialFailure(provider != nil && provider.Record.ProxyID != nil, err)
	if snapshot, ok := providercore.GrokCredentialFailureSnapshot(err); ok {
		class.SetSnapshot(&snapshot)
	}
	if ctx.Err() != nil {
		return "", "", ctx.Err()
	}
	if class.Permanent || class.Transient {
		freshToken, mutationErr := s.Recovery.Apply(credentialCtx, ExecutionRecord(provider), credentialMutation(class))
		if freshToken != "" {
			return freshToken, "oauth", nil
		}
		if mutationErr != nil {
			if ctx.Err() != nil {
				return "", "", ctx.Err()
			}
			if credentialCtx.Err() != nil {
				return "", "", credentialFailover(output, provider, forwardcore.GrokCredentialFailure{
					Scope:   forwardcore.GatewayFailureScopeRequest,
					Reason:  forwardcore.GrokCredentialReasonFailoverTimeout,
					Action:  forwardcore.NextProviderStop,
					Message: "Grok OAuth credential failover budget exhausted",
				})
			}
			if errors.Is(mutationErr, providercore.ErrRefreshProviderStateChanged) {
				class = forwardcore.GrokCredentialFailure{
					Scope:   forwardcore.GatewayFailureScopeProvider,
					Reason:  forwardcore.GrokCredentialReasonProviderChanged,
					Action:  forwardcore.NextProviderRetry,
					Message: "Grok OAuth provider eligibility changed",
				}
			} else if errors.Is(mutationErr, providercore.ErrRefreshProviderRereadFailed) {
				class = forwardcore.GrokCredentialFailure{
					Scope:   forwardcore.GatewayFailureScopeShared,
					Reason:  forwardcore.GrokCredentialReasonProviderDown,
					Action:  forwardcore.NextProviderStop,
					Message: "Grok OAuth provider state is temporarily unavailable",
				}
			} else {
				class = forwardcore.GrokCredentialFailure{
					Scope:   forwardcore.GatewayFailureScopeShared,
					Reason:  forwardcore.GrokCredentialReasonStateUpdate,
					Action:  forwardcore.NextProviderStop,
					Message: "Grok OAuth provider state could not be updated safely",
				}
			}
		}
	}
	return "", "", credentialFailover(output, provider, class)
}

func credentialFailover(output CredentialObserver, provider *ExecutionProvider, class forwardcore.GrokCredentialFailure) error {
	if strings.TrimSpace(class.Message) == "" {
		class.Message = "Grok OAuth credentials are unavailable"
	}
	if output != nil {
		output.ObserveCredentialFailure(provider.Record.ID, class)
	}
	return &forwardcore.UpstreamFailoverError{
		Stage:  forwardcore.GatewayFailureStageProviderAuth,
		Scope:  class.Scope,
		Reason: class.Reason, NextProviderAction: class.Action, ClientStatusCode: http.StatusServiceUnavailable,
		ClientMessage: forwardcore.GrokCredentialUnavailableClientMessage,
	}
}

// credentialMutation 将凭据失败分类转换为提供商存储需要的更新条件。
func credentialMutation(class forwardcore.GrokCredentialFailure) providercore.GrokCredentialMutation {
	return providercore.GrokCredentialMutation{
		Permanent:     class.Permanent,
		Transient:     class.Transient,
		VerifyMissing: class.Reason == forwardcore.GrokCredentialReasonMissing,
		VerifyProxy:   class.Reason == forwardcore.GrokCredentialReasonProxyInvalid,
		Reason:        string(class.Reason),
		Snapshot:      class.Snapshot(),
	}
}

func (s *RequestCredentials) blocked(value *ExecutionProvider) bool {
	if s == nil || value == nil {
		return false
	}
	return s.Runtime.Blocked(value.Record.ID, func() string { return providercore.RefreshCredentialIdentity(ExecutionRecord(value)) })
}
