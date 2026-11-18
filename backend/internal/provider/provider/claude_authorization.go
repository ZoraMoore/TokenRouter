package provider

import (
	"context"
	"log"

	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/upstream/anthropic/oauth"
)

func ClaudeAuthorizationOptions(resolveProxy func(context.Context, int64) (string, bool)) provider.ClaudeAuthorizationOptions {
	return provider.ClaudeAuthorizationOptions{
		ScopeOAuth:            oauth.ScopeOAuth,
		ScopeAPI:              oauth.ScopeAPI,
		ScopeInference:        oauth.ScopeInference,
		GenerateState:         oauth.GenerateState,
		GenerateCodeVerifier:  oauth.GenerateCodeVerifier,
		GenerateCodeChallenge: oauth.GenerateCodeChallenge,
		GenerateSessionID:     oauth.GenerateSessionID,
		BuildAuthorizationURL: oauth.BuildAuthorizationURL,
		Logf:                  log.Printf,
		ResolveProxy:          resolveProxy,
	}
}
