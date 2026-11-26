package openai

import (
	"crypto/sha256"
	"fmt"
	"testing"
)

// TestCodexBaseInstructionsForModel 校验各型号选到的 Codex v0.172.0 正文。
func TestCodexBaseInstructionsForModel(t *testing.T) {
	const (
		gpt56     = "a91357a1cd2727a0be06d461248d6e3a7274746e38108f548a3adf2cc2430415"
		gpt6Astra = "4ddce5803b638f01342f657002289d1307634c677df840eeefdd4bc5b451276d"
		gpt6Sol   = "a8567d97e0fb03f19942650430e0905fd2e092f102e8d98c03fc1b89995619c2"
		gpt6Luna  = "e52174f354e3970ad8de5935fff8d04ddf2890a35659283d163befb56490198c"
		gpt61Sol  = "d12e6dbdfd4a4ddb26a1f6048b69a32b8b32f74e4add9791d1dff7f4cac0b032"
		gpt62     = "aa47d2dee261594c4944241b646088730d74d87afa4da8d2da0554540a5e7623"
		codex     = "42842be69650ae563d212695e8d3f3591534908fd8ca33b63f742daf41f88b65"
	)
	cases := []struct {
		model      string
		wantSHA256 string
	}{
		{"gpt-6.2-sol", gpt62},
		{"gpt-6.2-astra", gpt62},
		{"gpt-6.2-sol-xhigh", gpt62},
		{"gpt-6.2-astra-openai-compact", gpt62},
		{"gpt-6.1-sol", gpt61Sol},
		{"gpt-6.1-sol-minimal", gpt61Sol},
		{"gpt-6.1-terra", gpt6Astra},
		{"gpt-6.1-terra-minimal", gpt6Astra},
		{"gpt-6-sol", gpt6Sol},
		{"openai/GPT-6__SOL openai-compact", gpt6Sol},
		{"gpt-6-luna", gpt6Luna},
		{"gpt-6-luna-high", gpt6Luna},
		{"gpt-6-astra", gpt6Astra},
		{"gpt-6", gpt6Astra},
		{" OPENAI/GPT-6_ASTRA ", gpt6Astra},
		{"gpt-6-astra-2026-09-01", gpt6Astra},
		{"gpt-5.6", gpt56},
		{"gpt-5.6-sol", gpt56},
		{"gpt-5.6-terra", gpt56},
		{"gpt-5.6-luna", gpt56},
		{"codex-auto-review", codex},
		{"gpt-5.6-codex", codex},
		{"gpt-5-codex", codex},
		{"gpt-5.3-codex", codex},
		{"gpt-5.3-codex-spark", codex},
		{"gpt-5.1-codex-max", codex},
		{"gpt-5.2-codex", codex},
		{"gpt-5.1", gpt56},
		{"gpt-5.2", gpt56},
		{" GPT-5.5 ", gpt56},
		{"gpt-5", gpt56},
		{"gpt-5.3", gpt56},
		{"gpt-5.4", gpt56},
		{"gpt-6.2", gpt56},
		{"gpt-6.2-sol-extra", gpt56},
		{"gpt-6-sol-minimal", gpt56},
		{"some-unknown-model", gpt56},
		{"", gpt56},
	}
	for _, tc := range cases {
		sum := sha256.Sum256([]byte(CodexBaseInstructionsForModel(tc.model)))
		if got := fmt.Sprintf("%x", sum); got != tc.wantSHA256 {
			t.Errorf("model %q: got %s, want %s", tc.model, got, tc.wantSHA256)
		}
	}
}

// TestCodexPromptModelID 检查正文相同的型号是否仍各自对应正确的描述文件。
func TestCodexPromptModelID(t *testing.T) {
	cases := map[string]string{
		"gpt-6.2-sol":                       "gpt-6.2-sol",
		"vendor/openai/GPT-6.2_ASTRA xhigh": "gpt-6.2-astra",
		"gpt-6.1-terra-openai-compact":      "gpt-6.1-terra",
		" vendor / openai / GPT5.6_SOL ":    "gpt-5.6",
		"gpt-6-astra":                       "gpt-6-astra",
	}
	for model, want := range cases {
		if got := codexPromptModelID(model); got != want {
			t.Errorf("model %q: got %q, want %q", model, got, want)
		}
	}
}

// TestCodexTemplateFromDescriptor 检查缺失或无效的模型描述能否走回退。
func TestCodexTemplateFromDescriptor(t *testing.T) {
	const model = "gpt-6.2-sol"
	cases := []struct {
		name       string
		descriptor string
		want       string
	}{
		{"valid", `{"slug":"gpt-6.2-sol","model_messages":{"instructions_template":" template\n"}}`, " template\n"},
		{"invalid JSON", `{`, ""},
		{"unexpected slug", `{"slug":"gpt-6.2-astra","model_messages":{"instructions_template":"template"}}`, ""},
		{"missing field", `{"slug":"gpt-6.2-sol","base_instructions":"template"}`, ""},
		{"blank template", `{"slug":"gpt-6.2-sol","model_messages":{"instructions_template":" \n"}}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := codexTemplateFromDescriptor(model, tc.descriptor); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
