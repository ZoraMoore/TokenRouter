package provider

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
	"github.com/TokenFlux/TokenRouter/internal/protocol"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
)

func TestBuildProviderForCreateRequiresCustomGeminiThirdPartyBaseURL(t *testing.T) {
	invalidInput := &CreateProviderInput{
		Name:     "third-party Gemini",
		Platform: capability.PlatformGemini,
		Type:     capability.ProviderTypeAPIKey,
		Credentials: map[string]any{
			GeminiProviderTypeCredentialKey: GeminiProviderTypeThirdParty,
			"base_url":                      "https://generativelanguage.googleapis.com",
		},
	}

	_, err := BuildProviderForCreate(t.Context(), invalidInput, nil, CreationOptions{Now: time.Now, LoadLocation: time.LoadLocation, NewSeed: uuid.NewString})
	require.Error(t, err)
	require.Equal(t, "GEMINI_THIRD_PARTY_BASE_URL_REQUIRED", apperror.Reason(err))

	validInput := &CreateProviderInput{
		Name:     "third-party Gemini",
		Platform: capability.PlatformGemini,
		Type:     capability.ProviderTypeAPIKey,
		Credentials: map[string]any{
			GeminiProviderTypeCredentialKey: GeminiProviderTypeThirdParty,
			"base_url":                      "https://provider.example.test",
		},
	}
	provider, err := BuildProviderForCreate(t.Context(), validInput, nil, CreationOptions{Now: time.Now, LoadLocation: time.LoadLocation, NewSeed: uuid.NewString})
	require.NoError(t, err)
	require.NotNil(t, provider)
}

// TestBuildProviderForCreateNormalizesLegacyOpenAIConfigurationForCreateAndImport 检查通用导入调用 CreateProvider，创建时清理废弃 OpenAI 配置。
func TestBuildProviderForCreateNormalizesLegacyOpenAIConfigurationForCreateAndImport(t *testing.T) {
	input := &CreateProviderInput{
		Name:     "legacy-openai",
		Platform: capability.PlatformOpenAI,
		Type:     capability.ProviderTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "secret",
			"openai_capabilities": map[string]any{
				"chat_completions": true,
				"embeddings":       false,
			},
		},
	}
	extra := map[string]any{
		"openai_responses_mode":      "force_responses",
		"openai_responses_supported": false,
		"unrelated":                  map[string]any{"keep": true},
	}

	provider, err := BuildProviderForCreate(t.Context(), input, extra, CreationOptions{Now: time.Now, LoadLocation: time.LoadLocation, NewSeed: uuid.NewString})

	require.NoError(t, err)
	require.Contains(t, provider.UpstreamProtocols(), protocol.ProtocolOpenAIResponses)
	require.NotContains(t, provider.UpstreamProtocols(), protocol.ProtocolOpenAIChatCompletions)
	require.NotContains(t, provider.Extra, ExtraKeyTextRouteMode)
	require.NotContains(t, provider.Extra, "openai_responses_probe_status")
	require.Equal(t, false, provider.Extra[ExtraKeyResponsesContinuationSupported])
	require.Equal(t, map[string]any{"keep": true}, provider.Extra["unrelated"])
	require.NotContains(t, provider.Credentials, LegacyOpenAICapabilitiesCredentialKey)
	require.NotContains(t, provider.Extra, LegacyOpenAIResponsesModeExtraKey)
	require.NotContains(t, provider.Extra, "openai_responses_supported")
}

func TestBuildProviderForCreateAddsCodexModels(t *testing.T) {
	for _, credentials := range []map[string]any{nil, {"model_whitelist": []any{}}, {"model_mapping": map[string]any{"gpt-6-sol": "gpt-6-sol"}}} {
		value, err := BuildProviderForCreate(t.Context(), &CreateProviderInput{
			Platform: PlatformOpenAI, Type: ProviderTypeOAuth, Credentials: credentials,
		}, nil, CreationOptions{Now: time.Now, LoadLocation: time.LoadLocation, NewSeed: uuid.NewString})
		require.NoError(t, err)
		require.Len(t, value.Credentials["model_whitelist"], 7)
		require.Contains(t, value.Credentials["model_whitelist"], "gpt-6.1-sol")
	}
}

// TestBuildProviderForCreateUsesGlobalOAuthModels 覆盖未指定名单的账号和自带限制的导入账号。
func TestBuildProviderForCreateUsesGlobalOAuthModels(t *testing.T) {
	ctx := t.Context()
	models := []string{"gpt-custom", "gpt-5.5"}
	options := CreationOptions{
		Now: time.Now, LoadLocation: time.LoadLocation, NewSeed: uuid.NewString,
		ReadOpenAIOAuthModels: func(context.Context) (OpenAIOAuthModelPolicy, error) {
			return NewOpenAIOAuthModelPolicy(models), nil
		},
	}

	for _, tc := range []struct {
		name        string
		credentials map[string]any
		want        []string
	}{
		{"use site models", nil, models},
		{"mapped alias", map[string]any{"model_mapping": map[string]any{"alias": "gpt-outside"}}, models},
		{"account subset", map[string]any{"model_whitelist": []any{"gpt-custom"}}, []string{"gpt-custom"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value, err := BuildProviderForCreate(t.Context(), &CreateProviderInput{
				Platform: PlatformOpenAI, Type: ProviderTypeOAuth, Credentials: tc.credentials,
			}, nil, options)
			require.NoError(t, err)
			require.Equal(t, tc.want, value.Credentials["model_whitelist"])
		})
	}

	readErr := errors.New("settings offline")
	options.ReadOpenAIOAuthModels = func(context.Context) (OpenAIOAuthModelPolicy, error) {
		return OpenAIOAuthModelPolicy{}, readErr
	}
	_, err := BuildProviderForCreate(ctx, &CreateProviderInput{Platform: PlatformOpenAI, Type: ProviderTypeOAuth}, nil, options)
	require.ErrorIs(t, err, readErr)
	key, err := BuildProviderForCreate(ctx, &CreateProviderInput{Platform: PlatformOpenAI, Type: ProviderTypeAPIKey}, nil, options)
	require.NoError(t, err)
	require.NotContains(t, key.Credentials, "model_whitelist")
}
