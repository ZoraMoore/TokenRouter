package tierpolicy

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/protocol/openai"
)

// TestDecisionPreservesLazyPolicyOrder 检查系统短路、分组关闭和按需查价的调用顺序。
func TestDecisionPreservesLazyPolicyOrder(t *testing.T) {
	for _, tc := range []struct {
		name, group, key, original, action string
		want                               []string
	}{
		{name: "系统拒绝", original: "priority", action: "block", want: []string{"system:priority"}},
		{name: "分组关闭", group: "force_off", key: "force_on", want: nil},
		{name: "Key开启后复核系统", key: "force_on", want: []string{"key", "price", "system:priority"}},
		{name: "Key关闭不查价", key: "force_off", original: "priority", want: []string{"system:priority", "key"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			input := DecisionInput{
				Model: "gpt-5.5", GroupPolicy: tc.group, OpenAI: true,
				Evaluate: func(tier string) (string, string) {
					calls = append(calls, "system:"+tier)
					return tc.action, ""
				},
				KeyPolicy:        func() string { calls = append(calls, "key"); return tc.key },
				ForceOnSupported: func() bool { calls = append(calls, "price"); return true },
			}
			Resolve(input, tc.original, tc.original != "")
			require.Equal(t, tc.want, calls)
		})
	}
}

// TestEmptyBodyDoesNotReadFastPolicy 检查空报文直接返回。
func TestEmptyBodyDoesNotReadFastPolicy(t *testing.T) {
	result, err := ApplyBody(nil, DecisionInput{})
	require.NoError(t, err)
	require.Nil(t, result)
}

// TestDecisionProviderForceOffPrecedesOtherPolicies 检查提供商关闭加速时的系统裁决和按需读取。
func TestDecisionProviderForceOffPrecedesOtherPolicies(t *testing.T) {
	for _, tc := range []struct {
		tier, want string
		remove     bool
	}{
		{tier: "priority", remove: true},
		{tier: "fast", remove: true},
		{tier: " ULTRAFAST ", remove: true},
		{},
		{tier: "flex", want: "flex"},
		{tier: "auto", want: "auto"},
		{tier: "default", want: "default"},
		{tier: "scale", want: "scale"},
	} {
		t.Run(tc.tier, func(t *testing.T) {
			var systemTiers []string
			input := DecisionInput{
				Model: "gpt-5.5", OpenAI: true, GroupPolicy: "force_ultrafast", ProviderForceOff: true,
				Evaluate: func(tier string) (string, string) {
					systemTiers = append(systemTiers, tier)
					return "force_priority", ""
				},
				KeyPolicy: func() string {
					t.Fatal("提供商关闭加速后仍读取 Key 策略")
					return "force_on"
				},
				ForceOnSupported: func() bool {
					t.Fatal("提供商关闭加速后仍查询模型价格")
					return true
				},
			}
			decision := Resolve(input, tc.tier, tc.tier != "")
			require.Nil(t, decision.Blocked)
			require.Equal(t, tc.remove, decision.DeleteField)
			require.Equal(t, tc.want, decision.Tier)
			var wantSystemTiers []string
			if tc.tier != "" {
				wantSystemTiers = []string{openai.ServiceTierValue(tc.tier)}
			}
			require.Equal(t, wantSystemTiers, systemTiers)
		})
	}
}
