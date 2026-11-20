package httpapi

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
	"text/template/parse"

	"github.com/TokenFlux/TokenRouter/internal/upstream/openai"
)

type forcedCodexInstructionsTemplateData struct {
	ExistingInstructions string
	OriginalModel        string
	NormalizedModel      string
	BillingModel         string
	UpstreamModel        string
}

func applyForcedCodexInstructionsTemplate(
	reqBody map[string]any,
	templateText string,
	data forcedCodexInstructionsTemplateData,
) (bool, error) {
	tmpl, err := template.New("forced_codex_instructions").Option("missingkey=zero").Parse(templateText)
	if err != nil {
		return false, fmt.Errorf("parse forced codex instructions template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return false, fmt.Errorf("render forced codex instructions template: %w", err)
	}
	rendered := strings.TrimSpace(buf.String())
	if rendered == "" {
		return false, nil
	}

	existing, _ := reqBody["instructions"].(string)
	previousID, _ := reqBody["previous_response_id"].(string)
	if openai.UsesGPT62CodexCachePrefix(data.UpstreamModel) && strings.TrimSpace(previousID) == "" &&
		strings.TrimSpace(existing) == "" && strings.TrimSpace(data.ExistingInstructions) == "" &&
		isStaticCodexInstructionsTemplate(tmpl) {
		if openai.PrependCodexDeveloperPrefix(reqBody, rendered) {
			return true, nil
		}
		if openai.HasCodexDeveloperPrefix(reqBody, rendered) {
			return false, nil
		}
	}
	if strings.TrimSpace(existing) == rendered {
		return false, nil
	}
	reqBody["instructions"] = rendered
	return true, nil
}

// isStaticCodexInstructionsTemplate 识别纯文本和直接引用模型字段的模板。
// 条件分支和子模板可能间接读取本轮指令，交给顶层 instructions 处理。
func isStaticCodexInstructionsTemplate(tmpl *template.Template) bool {
	for _, node := range tmpl.Root.Nodes {
		switch n := node.(type) {
		case *parse.TextNode:
			continue
		case *parse.ActionNode:
			switch n.Pipe.String() {
			case ".OriginalModel", ".NormalizedModel", ".BillingModel", ".UpstreamModel":
				continue
			}
		}
		return false
	}
	return true
}
