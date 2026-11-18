package usageclient

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/infra/httpclient"
)

// TestUpstreamUsageEndpointUsesExistingVersionedBaseURLRules 检查用量查询地址的版本路径处理。
func TestUpstreamUsageEndpointUsesExistingVersionedBaseURLRules(t *testing.T) {
	tests := map[string]string{
		"https://gateway.example/v1":     "https://gateway.example/v1/usage",
		"https://gateway.example/v4":     "https://gateway.example/v4/usage",
		"https://gateway.example/v1beta": "https://gateway.example/v1beta/usage",
		"https://gateway.example/api":    "https://gateway.example/api/v1/usage",
	}
	for base, want := range tests {
		got, err := UpstreamUsageEndpoint(base, "/v1/usage", httpclient.BuildOpenAIEndpointURL)
		require.NoError(t, err, base)
		require.Equal(t, want, got, base)
	}

	statusTests := map[string]string{
		"https://gateway.example/v1":         "https://gateway.example/api/status",
		"https://gateway.example/subpath/v1": "https://gateway.example/subpath/api/status",
		"https://gateway.example/v4":         "https://gateway.example/api/status",
	}
	for base, want := range statusTests {
		got, err := UpstreamUsageStatusEndpoint(base)
		require.NoError(t, err, base)
		require.Equal(t, want, got, base)
	}
	tokenEndpoint, err := UpstreamUsageRootEndpoint("https://gateway.example/v1", "/api/usage/token")
	require.NoError(t, err)
	require.Equal(t, "https://gateway.example/api/usage/token", tokenEndpoint)
	tokenEndpoint, err = UpstreamUsageTokenEndpoint("https://gateway.example/v1")
	require.NoError(t, err)
	require.Equal(t, "https://gateway.example/api/usage/token/", tokenEndpoint)
	walletEndpoint, err := UpstreamUsageWalletEndpoint("https://gateway.example/v1")
	require.NoError(t, err)
	require.Equal(t, "https://gateway.example/user/balance", walletEndpoint)
	selfEndpoint, err := UpstreamUsageUserSelfEndpoint("https://gateway.example/v1")
	require.NoError(t, err)
	require.Equal(t, "https://gateway.example/api/user/self", selfEndpoint)
}
