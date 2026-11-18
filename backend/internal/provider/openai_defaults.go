package provider

import (
	_ "embed"
	"encoding/json"
	"maps"
	"slices"
	"sort"
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

// ApplyOpenAICreationModels 为新建 Codex 账号合并站点默认型号和导入的模型配置。
func ApplyOpenAICreationModels(value *Record) {
	if value == nil || !value.IsOpenAIOAuth() || value.IsShadow() {
		return
	}
	credentials := maps.Clone(value.Credentials)
	if credentials == nil {
		credentials = make(map[string]any)
	}
	models := slices.Clone(openAIDefaults.Models)
	add := func(model string) {
		model = strings.TrimSpace(model)
		if model != "" && !slices.Contains(models, model) {
			models = append(models, model)
		}
	}
	// 导入的额外型号和映射目标也需要出现在最终模型白名单中。
	var imported []string
	for model := range ExtractExplicitFinalModelWhitelist(credentials["model_whitelist"]) {
		imported = append(imported, model)
	}
	for _, target := range ResolveModelMapping(value, ModelMappingDefaults{}) {
		if !strings.Contains(target, "*") {
			imported = append(imported, target)
		}
	}
	sort.Strings(imported)
	for _, model := range imported {
		add(model)
	}
	credentials["model_whitelist"] = models
	value.Credentials = credentials
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
