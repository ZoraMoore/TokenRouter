package routing

import (
	"sort"
	"strings"
)

// ModelRejectionRules 提供候选调度资格、已配置模型和模型支持判断。
type ModelRejectionRules interface {
	IsSchedulable() bool
	GetConfiguredRequestModels() []string
	IsModelSupported(string) bool
}

// ModelRejectionSource 保留逐提供商懒读取，默认目录由平台适配器提供。
type ModelRejectionSource struct {
	Platform string
	Rules    ModelRejectionRules
	Defaults func(string) ([]string, error)
}

// AvailableModelsForRejection 返回用于拒绝消息的可用模型目录。
func AvailableModelsForRejection(providers []ModelRejectionSource, platform string) []string {
	modelSet := make(map[string]struct{})
	hasConfiguredModels := false
	for i := range providers {
		value := &providers[i]
		if !value.Rules.IsSchedulable() || !matchesRejectionPlatform(value, platform) {
			continue
		}
		requestModels := value.Rules.GetConfiguredRequestModels()
		if len(requestModels) == 0 {
			if value.Defaults == nil {
				continue
			}
			defaultModels, err := value.Defaults(value.Platform)
			if err != nil {
				continue
			}
			for _, model := range defaultModels {
				if model = strings.TrimSpace(model); model != "" && !strings.Contains(model, "*") && value.Rules.IsModelSupported(model) {
					modelSet[model] = struct{}{}
				}
			}
			continue
		}
		hasConfiguredModels = true
		for _, model := range requestModels {
			if model = strings.TrimSpace(model); model != "" && !strings.Contains(model, "*") && value.Rules.IsModelSupported(model) {
				modelSet[model] = struct{}{}
			}
		}
	}
	if len(modelSet) == 0 && !hasConfiguredModels {
		return nil
	}
	models := make([]string, 0, len(modelSet))
	for model := range modelSet {
		models = append(models, model)
	}
	sort.Strings(models)
	return models
}

func matchesRejectionPlatform(value *ModelRejectionSource, platform string) bool {
	return platform == "" || value.Platform == platform
}

// NewGroupModelRejection 生成分组模型拒绝错误，空请求或空候选时返回 nil。
func NewGroupModelRejection(platform, requested string, providers []ModelRejectionSource) error {
	requested = strings.TrimSpace(requested)
	if requested == "" || len(providers) == 0 {
		return nil
	}
	return &GroupModelUnsupportedError{Platform: platform, RequestedModel: requested, AvailableModels: AvailableModelsForRejection(providers, platform)}
}
