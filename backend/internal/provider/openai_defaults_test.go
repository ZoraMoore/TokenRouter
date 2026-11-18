package provider

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplyOpenAICreationModelsMergesAndIsolatesCredentials(t *testing.T) {
	credentials := map[string]any{
		"model_whitelist": []any{"gpt-6-sol", "custom-model"},
		"model_mapping":   map[string]any{"alias": "mapped-model"},
	}
	value := &Record{Platform: PlatformOpenAI, Type: ProviderTypeOAuth, Credentials: credentials}
	ApplyOpenAICreationModels(value)
	require.Equal(t, []string{"codex-auto-review", "gpt-5.5", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-6-astra", "gpt-6-sol", "gpt-6.1-sol", "custom-model", "mapped-model"}, value.Credentials["model_whitelist"])
	require.Equal(t, []any{"gpt-6-sol", "custom-model"}, credentials["model_whitelist"])
	ApplyOpenAICreationModels(value)
	require.Len(t, value.Credentials["model_whitelist"], 9)
}

func TestApplyOpenAICreationModelsScope(t *testing.T) {
	for _, value := range []*Record{
		{Platform: PlatformOpenAI, Type: ProviderTypeAPIKey},
		{Platform: PlatformAnthropic, Type: ProviderTypeOAuth},
		{Platform: PlatformOpenAI, Type: ProviderTypeOAuth, ParentProviderID: new(int64)},
	} {
		ApplyOpenAICreationModels(value)
		require.Nil(t, value.Credentials)
	}
}

func TestDefaultOpenAITestModelHonorsWhitelistAndMapping(t *testing.T) {
	for _, tc := range []struct {
		name        string
		credentials map[string]any
		want        string
	}{
		{"unrestricted", nil, "gpt-6.1-sol"},
		{"preferred", map[string]any{"model_whitelist": []string{"gpt-6-astra", "gpt-6.1-sol"}}, "gpt-6.1-sol"},
		{"fallback", map[string]any{"model_whitelist": []string{"gpt-6-astra"}}, "gpt-6-astra"},
		{"unavailable", map[string]any{"model_whitelist": []string{"custom"}}, ""},
		{"mapped", map[string]any{"model_whitelist": []string{"custom"}, "model_mapping": map[string]any{"gpt-6.1-sol": "custom"}}, "gpt-6.1-sol"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := &Record{Platform: PlatformOpenAI, Type: ProviderTypeOAuth, Credentials: tc.credentials}
			require.Equal(t, tc.want, DefaultOpenAITestModel(value, ModelMappingDefaults{}, ModelPlatformRules{}))
		})
	}
}
