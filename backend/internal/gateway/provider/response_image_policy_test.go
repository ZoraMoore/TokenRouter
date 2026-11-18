package provider

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/apikey"
	"github.com/TokenFlux/TokenRouter/internal/gateway/media"
	providercore "github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing"
)

// TestResponseImagePolicyUsesAvailableDefault 确认自动挂载工具遵循账号和全局的图片型号限制。
func TestResponseImagePolicyUsesAvailableDefault(t *testing.T) {
	global := providercore.NewOpenAIOAuthModelPolicy([]string{"gpt-text"})
	value := &ExecutionProvider{Record: providercore.Record{
		Platform: providercore.PlatformOpenAI, Type: providercore.ProviderTypeOAuth,
		Credentials:               map[string]any{"model_whitelist": []string{"gpt-text"}},
		OpenAIOAuthModelWhitelist: func(context.Context) (providercore.OpenAIOAuthModelPolicy, error) { return global, nil },
	}}
	key := &apikey.APIKey{Group: &routing.Group{ResponsesImagePolicy: "enabled"}}
	policy := ResponseImagePolicy{}
	require.False(t, policy.Enabled(t.Context(), value, key))
	global = providercore.NewOpenAIOAuthModelPolicy([]string{"gpt-text", media.DefaultOpenAIImageModel})
	require.False(t, policy.Enabled(t.Context(), value, key))
	value.Record.Credentials["model_whitelist"] = []string{media.DefaultOpenAIImageModel}
	require.True(t, policy.Enabled(t.Context(), value, key))
}
