package openai

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeriveStableUUIDv4_Deterministic(t *testing.T) {
	a := DeriveStableUUIDv4("test-seed-1")
	b := DeriveStableUUIDv4("test-seed-1")
	assert.Equal(t, a, b, "同一种子应返回相同结果")
}

func TestDeriveStableUUIDv4_DifferentSeeds(t *testing.T) {
	a := DeriveStableUUIDv4("seed-a")
	b := DeriveStableUUIDv4("seed-b")
	assert.NotEqual(t, a, b, "不同种子应返回不同结果")
}

func TestDeriveStableUUIDv4_ValidFormat(t *testing.T) {
	result := DeriveStableUUIDv4("test-seed")
	parsed, err := uuid.Parse(result)
	require.NoError(t, err, "应返回合法 UUID 格式")
	assert.Equal(t, uuid.Version(4), parsed.Version(), "应为 UUIDv4")
	assert.Equal(t, uuid.RFC4122, parsed.Variant(), "应为 RFC4122 变体")
}

func TestScopeCodexProviderIdentityValueInstallationSurvivesKeyRotation(t *testing.T) {
	namespace := "chatgpt:shared-account:user:user-1"
	first := ScopeCodexProviderIdentityValue(namespace, 77, "installation", "client-installation")
	second := ScopeCodexProviderIdentityValue(namespace, 88, "installation", "client-installation")

	// v0.1.277 的设备 ID 固定值用于检查升级后的历史会话兼容性。
	require.Equal(t, "01f2029a-d125-438b-96a9-cd64323c6f90", first)
	require.Equal(t, first, second, "轮换下游 Key 后仍使用同一个设备身份")
	require.NotEqual(t, first, ScopeCodexProviderIdentityValue(namespace, 77, "installation", "another-installation"))
	require.NotEqual(t, first, ScopeCodexProviderIdentityValue("chatgpt:another-account", 77, "installation", "client-installation"))
}

func TestScopeCodexProviderIdentityValueKeepsSessionTenantIsolation(t *testing.T) {
	for _, kind := range []string{"session", "thread", "turn", "window", "request"} {
		t.Run(kind, func(t *testing.T) {
			first := ScopeCodexProviderIdentityValue("chatgpt:shared-account", 77, kind, "client-value")
			second := ScopeCodexProviderIdentityValue("chatgpt:shared-account", 88, kind, "client-value")
			require.NotEqual(t, first, second, "会话身份按下游 Key 隔离")
		})
	}
}
