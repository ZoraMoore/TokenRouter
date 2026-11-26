package openai

import (
	_ "embed"
	"encoding/json"
	"strings"
)

const (
	// DefaultTestModel 是测试 OpenAI 提供商时使用的默认模型。
	DefaultTestModel = "gpt-5.4"

	// CodexUsageProbeModel 是 OAuth Codex 额度探针使用的兼容模型。
	CodexUsageProbeModel = "codex-auto-review"
)

var (
	// DefaultInstructions 是 Codex 系列共用的基础提示词。
	//
	//go:embed instructions.txt
	DefaultInstructions string

	// 各型号的提示词取自 OpenAI Codex rust-v0.172.0。
	//
	//go:embed instructions_gpt5_6.txt
	instructionsGPT56 string

	//go:embed instructions_gpt6_astra.txt
	instructionsGPT6Astra string

	//go:embed instructions_gpt6_sol.txt
	instructionsGPT6Sol string

	//go:embed instructions_gpt6_luna.txt
	instructionsGPT6Luna string

	//go:embed codex_gpt61_sol.json
	codexGPT61Sol string

	//go:embed codex_gpt61_terra.json
	codexGPT61Terra string

	//go:embed codex_gpt62_sol.json
	codexGPT62Sol string

	//go:embed codex_gpt62_astra.json
	codexGPT62Astra string

	// codexBasePrompts 在包初始化时从嵌入资源读取提示词。
	codexBasePrompts = map[string]string{
		"codex":         DefaultInstructions,
		"gpt-5.6":       instructionsGPT56,
		"gpt-6-astra":   instructionsGPT6Astra,
		"gpt-6-sol":     instructionsGPT6Sol,
		"gpt-6-luna":    instructionsGPT6Luna,
		"gpt-6.1-sol":   codexTemplateFromDescriptor("gpt-6.1-sol", codexGPT61Sol),
		"gpt-6.1-terra": codexTemplateFromDescriptor("gpt-6.1-terra", codexGPT61Terra),
		"gpt-6.2-sol":   codexTemplateFromDescriptor("gpt-6.2-sol", codexGPT62Sol),
		"gpt-6.2-astra": codexTemplateFromDescriptor("gpt-6.2-astra", codexGPT62Astra),
	}
)

// CodexBaseInstructionsForModel 返回对应型号的基础提示词。未匹配的型号使用 GPT-5.6 模板。
func CodexBaseInstructionsForModel(model string) string {
	if prompt := codexBasePrompts[codexPromptModelID(model)]; strings.TrimSpace(prompt) != "" {
		return prompt
	}
	if strings.TrimSpace(instructionsGPT56) != "" {
		return instructionsGPT56
	}
	return DefaultInstructions
}

// codexTemplateFromDescriptor 读取指定型号的提示词。资源缺失或无效时返回空串。
func codexTemplateFromDescriptor(model string, descriptor string) string {
	var metadata struct {
		Slug          string `json:"slug"`
		ModelMessages struct {
			InstructionsTemplate string `json:"instructions_template"`
		} `json:"model_messages"`
	}
	if err := json.Unmarshal([]byte(descriptor), &metadata); err != nil || metadata.Slug != model {
		return ""
	}
	prompt := metadata.ModelMessages.InstructionsTemplate
	if strings.TrimSpace(prompt) == "" {
		return ""
	}
	return prompt
}

// codexPromptModelID 识别同一基础模板使用的型号拼写。档位是否能用由请求校验决定。
func codexPromptModelID(model string) string {
	model = canonicalCodexPromptModel(model)
	for _, base := range [...]string{
		"gpt-6.2-sol", "gpt-6.2-astra", "gpt-6.1-sol", "gpt-6.1-terra", "gpt-6-sol", "gpt-6-luna",
	} {
		if model == base {
			return base
		}
		suffix, ok := strings.CutPrefix(model, base+"-")
		if !ok {
			continue
		}
		switch suffix {
		case "none", "low", "medium", "high", "xhigh", "max", "openai-compact":
			return base
		case "minimal":
			if strings.HasPrefix(base, "gpt-6.") {
				return base
			}
		}
		break
	}
	switch {
	case model == "gpt-6", model == "gpt-6-astra", strings.HasPrefix(model, "gpt-6-astra-"):
		return "gpt-6-astra"
	case strings.Contains(model, "codex"):
		return "codex"
	case strings.HasPrefix(model, "gpt-5.6"):
		return "gpt-5.6"
	default:
		return ""
	}
}

// canonicalCodexPromptModel 规范提示词查询使用的型号尾段。
func canonicalCodexPromptModel(model string) string {
	if slash := strings.LastIndexByte(model, '/'); slash >= 0 {
		model = model[slash+1:]
	}
	model = strings.ToLower(strings.TrimSpace(model))
	model = strings.ReplaceAll(model, "_", "-")
	model = strings.Join(strings.Fields(model), "-")
	for strings.Contains(model, "--") {
		model = strings.ReplaceAll(model, "--", "-")
	}
	if suffix, ok := strings.CutPrefix(model, "gpt5"); ok {
		return "gpt-5" + suffix
	}
	return model
}
