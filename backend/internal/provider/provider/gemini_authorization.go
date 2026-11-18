package provider

import (
	"context"
	"fmt"

	"github.com/TokenFlux/TokenRouter/internal/infra/telemetry/logging"
	"github.com/TokenFlux/TokenRouter/internal/protocol/google"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/upstream/gemini/codeassist"
)

func GeminiAuthorizationOptions(config func() google.OAuthConfig, resolveProxy func(context.Context, int64) (string, bool)) provider.GeminiAuthorizationOptions {
	return provider.GeminiAuthorizationOptions{
		Config:                config,
		BuiltinClientID:       codeassist.GeminiCLIOAuthClientID,
		CLIRedirectURI:        codeassist.GeminiCLIRedirectURI,
		AIStudioRedirectURI:   codeassist.AIStudioOAuthRedirectURI,
		GenerateState:         codeassist.GenerateState,
		GenerateCodeVerifier:  codeassist.GenerateCodeVerifier,
		GenerateSessionID:     codeassist.GenerateSessionID,
		GenerateCodeChallenge: codeassist.GenerateCodeChallenge,
		EffectiveOAuthConfig:  codeassist.EffectiveOAuthConfig,
		BuildAuthorizationURL: codeassist.BuildAuthorizationURL,
		FetchProject:          codeassist.FetchProjectIDFromResourceManager,
		ResolveProxy:          resolveProxy,
		Logf:                  func(format string, args ...any) { logging.LegacyPrintf("service.gemini_oauth", format, args...) },
		Printf:                func(format string, args ...any) { _, _ = fmt.Printf(format, args...) },
	}
}
