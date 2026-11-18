package provider

import (
	"context"
	_ "embed"
	"encoding/json"
	"maps"
	"slices"
	"strings"
)

var (
	//go:embed openai_defaults.json
	openAIDefaultsJSON []byte
	openAIDefaults     = loadOpenAIDefaults()
)

type openAIDefaultSettings struct {
	Models     []string `json:"models"`
	TestModels []string `json:"test_models"`
	TestPrompt string   `json:"test_prompt"`
}

func loadOpenAIDefaults() openAIDefaultSettings {
	var defaults openAIDefaultSettings
	if err := json.Unmarshal(openAIDefaultsJSON, &defaults); err != nil {
		panic(err)
	}
	return defaults
}

// DefaultProviderTestPrompt 返回文字连接测试的默认提示词。
func DefaultProviderTestPrompt() string { return openAIDefaults.TestPrompt }

// DefaultOpenAITestModel 按优先级选择通过账号模型规则的测试型号。
func DefaultOpenAITestModel(value *Record, defaults ModelMappingDefaults, rules ModelPlatformRules) string {
	for _, model := range openAIDefaults.TestModels {
		if value.IsModelSupported(model, defaults, rules) {
			return model
		}
	}
	return ""
}

// ApplyOpenAICreationModels 为 OpenAI OAuth 账号预填模型。管理员可在导入时指定更窄的账号名单。
// @project-doc docs/interfaces/openai_upstream.md#oauth_final_model_whitelist
func ApplyOpenAICreationModels(ctx context.Context, value *Record, read OpenAIOAuthModelPolicyReader) error {
	if value == nil || !value.IsOpenAIOAuth() || value.IsShadow() {
		return nil
	}
	var policy OpenAIOAuthModelPolicy
	if read != nil {
		var err error
		if policy, err = read(ctx); err != nil {
			return err
		}
	}
	local := ExtractExplicitFinalModelWhitelist(value.Credentials["model_whitelist"])
	var models []string
	if policy.Configured() {
		if len(local) > 0 {
			models = slices.Sorted(maps.Keys(local))
		} else {
			models = policy.Models()
		}
	} else {
		// 首次设置站点名单前，账号使用内置型号、导入的白名单和具体映射目标。
		models = slices.Clone(openAIDefaults.Models)
		if local == nil {
			local = make(map[string]struct{})
		}
		for _, target := range ResolveModelMapping(value, ModelMappingDefaults{}) {
			target = strings.TrimSpace(target)
			if target != "" && !strings.Contains(target, "*") {
				local[target] = struct{}{}
			}
		}
		for _, model := range slices.Sorted(maps.Keys(local)) {
			if !slices.Contains(models, model) {
				models = append(models, model)
			}
		}
	}
	credentials := maps.Clone(value.Credentials)
	if credentials == nil {
		credentials = make(map[string]any)
	}
	credentials["model_whitelist"] = models
	value.Credentials = credentials
	return nil
}

// OpenAITestFallbackModel 返回首选型号不可用时允许测试的回退型号。
func OpenAITestFallbackModel(value *Record, requested string, defaults ModelMappingDefaults, rules ModelPlatformRules) string {
	if requested != openAIDefaults.TestModels[0] {
		return ""
	}
	fallback := openAIDefaults.TestModels[1]
	if value.IsModelSupported(fallback, defaults, rules) {
		return fallback
	}
	return ""
}
