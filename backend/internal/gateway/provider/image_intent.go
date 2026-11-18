package provider

import (
	"strings"

	gatewaymedia "github.com/TokenFlux/TokenRouter/internal/gateway/media"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
	"github.com/TokenFlux/TokenRouter/internal/upstream/openai"
)

// ImageIntent 只绑定平台纯工具解析，图片权限、规则和账单字段唯一归 media。
func ImageIntent() gatewaymedia.ImageIntentPolicy {
	return gatewaymedia.NewImageIntentPolicy(gatewaymedia.ImageToolRules{
		IsImageType:     openai.IsOpenAIImageGenerationType,
		IsNamespaceName: openai.IsOpenAIImageGenNamespaceName,
		HasTool:         openai.HasOpenAIImageGenerationTool,
		ToolChoice:      openai.OpenAIAnyToolChoiceSelectsImageGeneration,
		FirstString:     openai.FirstNonEmptyString,
	})
}

// ImageIntentForPlatform 根据平台解析图片意图，Grok 忽略被动工具声明。
func ImageIntentForPlatform(endpoint, model string, body []byte, platform string) bool {
	return ImageIntent().IsImageGenerationIntentForPlatform(endpoint, model, body, strings.EqualFold(strings.TrimSpace(platform), capability.PlatformGrok))
}
