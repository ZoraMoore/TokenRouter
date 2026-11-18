package provider

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIOAuthFinalModelsIntersectAccountRules(t *testing.T) {
	ctx := context.Background()
	global := NewOpenAIOAuthModelPolicy([]string{"gpt-here", "gpt-experimental", "other"})
	reads := 0
	record := &Record{Platform: PlatformOpenAI, Type: ProviderTypeOAuth, Credentials: map[string]any{
		"model_whitelist": []string{"gpt-*"},
		"model_mapping":   map[string]any{"public": "gpt-experimental", "stale": "gpt-obsolete"},
	}, OpenAIOAuthModelWhitelist: func(context.Context) (OpenAIOAuthModelPolicy, error) {
		reads++
		return global, nil
	}}
	defaults := ModelMappingDefaults{Models: func(*Record) []string { return []string{"gpt-here", "gpt-obsolete"} }}
	rules := ModelPlatformRules{}
	snapshot := PrepareModelRules(record, defaults, rules)
	require.Equal(t, 1, reads)
	for _, id := range []string{"gpt-here", "gpt-experimental", "public"} {
		require.True(t, snapshot.Supports(id), id)
	}
	for _, id := range []string{"gpt-obsolete", "stale", "other", "GPT-HERE"} {
		require.False(t, snapshot.Supports(id), id)
	}
	require.Equal(t, 1, reads, "目录中的模型共用一次全局白名单读取")
	require.ElementsMatch(t, []string{"gpt-here", "gpt-experimental", "public"}, snapshot.ConfiguredModels())
	require.Equal(t, snapshot.ConfiguredModels(), record.GetConfiguredRequestModels(defaults))
	require.True(t, record.IsModelSupported("public", defaults, rules))
	require.False(t, record.FinalModelWhitelisted("gpt-obsolete", defaults, rules))
	require.False(t, record.OpenAIOAuthFinalModelPolicy(ctx).Allows("gpt-obsolete"))
	global = NewOpenAIOAuthModelPolicy([]string{"gpt-obsolete"})
	require.False(t, record.IsModelSupported("public", defaults, rules))
	require.True(t, record.IsModelSupported("stale", defaults, rules))
	require.True(t, snapshot.Supports("public"), "同一份目录快照的答案一致")
}

func TestOpenAIOAuthModelPolicyScopeAndReadFailure(t *testing.T) {
	ctx := context.Background()
	readError := func(context.Context) (OpenAIOAuthModelPolicy, error) {
		return OpenAIOAuthModelPolicy{}, errors.New("settings unavailable")
	}
	for _, tc := range []struct {
		platform string
		kind     string
		allowed  bool
	}{
		{PlatformOpenAI, ProviderTypeOAuth, false},
		{PlatformOpenAI, ProviderTypeAPIKey, true},
		{PlatformOpenAI, ProviderTypeSetupToken, true},
		{PlatformGrok, ProviderTypeOAuth, true},
	} {
		record := &Record{Platform: tc.platform, Type: tc.kind, OpenAIOAuthModelWhitelist: readError}
		require.Equal(t, tc.allowed, record.OpenAIOAuthFinalModelPolicy(ctx).Allows("gpt-custom"))
		require.Equal(t, tc.allowed, PrepareModelRules(record, ModelMappingDefaults{}, ModelPlatformRules{}).Supports("gpt-custom"))
	}
}

// TestOpenAIOAuthFinalModelsKeepSparkScope 检查 Spark 账号的独立额度模型仍受全局名单约束。
func TestOpenAIOAuthFinalModelsKeepSparkScope(t *testing.T) {
	parentID := int64(5)
	global := NewOpenAIOAuthModelPolicy([]string{"gpt-spark", "gpt-other"})
	record := &Record{
		Platform: PlatformOpenAI, Type: ProviderTypeOAuth, ParentProviderID: &parentID,
		Credentials:               map[string]any{"model_whitelist": []string{"*"}},
		OpenAIOAuthModelWhitelist: func(context.Context) (OpenAIOAuthModelPolicy, error) { return global, nil },
	}
	defaults := ModelMappingDefaults{Models: func(*Record) []string { return []string{"gpt-spark"} }}
	snapshot := PrepareModelRules(record, defaults, ModelPlatformRules{})
	require.Equal(t, []string{"gpt-spark"}, snapshot.ConfiguredModels())
	require.True(t, snapshot.Supports("gpt-spark"))
	require.False(t, snapshot.Supports("gpt-other"))
	global = NewOpenAIOAuthModelPolicy([]string{"gpt-other"})
	require.False(t, record.AllowsOpenAIOAuthTarget(record.OpenAIOAuthFinalModelPolicy(t.Context()), "gpt-spark", defaults))
}

// TestPreparedModelRulesParity 核对准备后的索引与逐次读取配置的匹配结果。
func TestPreparedModelRulesParity(t *testing.T) {
	rules := ModelPlatformRules{
		NormalizeQoder:      func(model string) string { return strings.TrimSuffix(strings.ToLower(model), "-alias") },
		QoderCompatible:     func(model string) bool { return model != "blocked-site" },
		OpenAIOAuthServable: func(model string, _ OpenAIOAuthModelPolicy) bool { return !strings.HasPrefix(model, "foreign-") },
	}
	defaults := ModelMappingDefaults{Models: func(*Record) []string { return []string{"known", "foreign-model"} }, Antigravity: func() map[string]string { return map[string]string{"alias": "known"} }}
	for _, platform := range []string{PlatformOpenAI, PlatformQoder, PlatformAnthropic, PlatformAntigravity} {
		for _, whitelist := range [][]string{nil, {}, {"known", "gpt-*", "ſ", "Σ", "K", "İ"}, {"*"}, {"known-alias", "blocked-site"}} {
			record := &Record{Platform: platform, Type: "oauth", Credentials: map[string]any{
				"model_whitelist": whitelist,
				"model_mapping":   map[string]any{"alias": "known", "gpt-*": "known", "gpt-special*": "foreign-model", "empty-target": ""},
			}}
			snapshot := PrepareModelRules(record, defaults, rules)
			for _, model := range []string{"", "known", "KNOWN", " known ", "alias", " alias ", "gpt-new", "gpt-special-new", "empty-target", "unknown", "foreign-model", "blocked-site", "known-alias", "S", "s", "ſ", "σ", "ς", "Σ", "K", "k", "K", "İ", "i", "I", "ı"} {
				require.Equal(t, record.IsModelSupported(model, defaults, rules), snapshot.Supports(model), "platform=%s whitelist=%v model=%s", platform, whitelist, model)
				require.Equal(t, record.FinalModelWhitelisted(model, defaults, rules), snapshot.FinalModelWhitelisted(model), "platform=%s whitelist=%v model=%s", platform, whitelist, model)
				mapped, matched := ResolveMappedModel(ResolveModelMapping(record, defaults), model)
				got, gotMatch := snapshot.ResolveMappedModel(model)
				require.Equal(t, mapped, got)
				require.Equal(t, matched, gotMatch)
				require.Equal(t, ResolveForwardMappedModel(record, model, defaults), snapshot.ForwardMappedModel(model))
			}
			require.Equal(t, record.GetConfiguredRequestModels(defaults), snapshot.ConfiguredModels())
		}
	}
}

// TestPreparedModelRulesOwnConfiguration 配置替换和调用方修改返回值不会改变已准备的规则。
func TestPreparedModelRulesOwnConfiguration(t *testing.T) {
	mapping := map[string]any{"alias": "known"}
	whitelist := []string{"known"}
	record := &Record{Platform: PlatformOpenAI, Credentials: map[string]any{"model_mapping": mapping, "model_whitelist": whitelist}}
	snapshot := PrepareModelRules(record, ModelMappingDefaults{}, ModelPlatformRules{})
	mapping["alias"] = "changed"
	whitelist[0] = "changed"
	copy := snapshot.Mapping()
	copy["alias"] = "copy-change"
	models := snapshot.ConfiguredModels()
	models[0] = "copy-change"
	require.True(t, snapshot.Supports("alias"))
	require.False(t, snapshot.Supports("changed"))
	require.Equal(t, "known", snapshot.ForwardMappedModel("alias"))
	require.ElementsMatch(t, []string{"known", "alias"}, snapshot.ConfiguredModels())
}

// TestPreparedModelRulesReadDefaultsOnce 多个型号校验共用一次默认配置读取。
func TestPreparedModelRulesReadDefaultsOnce(t *testing.T) {
	modelsReads, mappingReads := 0, 0
	defaults := ModelMappingDefaults{
		Models:      func(*Record) []string { modelsReads++; return []string{"known"} },
		Antigravity: func() map[string]string { mappingReads++; return map[string]string{"alias": "known"} },
	}
	record := &Record{Platform: PlatformAntigravity}
	snapshot := PrepareModelRules(record, defaults, ModelPlatformRules{})
	for range 100 {
		require.True(t, snapshot.Supports("alias"))
		require.True(t, snapshot.FinalModelWhitelisted("known"))
		_ = snapshot.ConfiguredModels()
	}
	require.Equal(t, 1, modelsReads)
	require.Equal(t, 1, mappingReads)
}
