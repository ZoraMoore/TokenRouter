package provider

import (
	"context"
	"errors"
	"strings"
)

var ErrOpenAIOAuthModelNotAllowed = errors.New("model is outside the OpenAI OAuth whitelist")

// OpenAIOAuthModelPolicyReader 读取本站当前生效的 OAuth 模型白名单。
type OpenAIOAuthModelPolicyReader func(context.Context) (OpenAIOAuthModelPolicy, error)

// ModelPlatformRules 只在平台专有资格分支按需调用，模型配置与一跳匹配由 provider 拥有。
type ModelPlatformRules struct {
	NormalizeQoder      func(string) string
	QoderCompatible     func(string) bool
	OpenAIOAuthServable func(string, OpenAIOAuthModelPolicy) bool
}

// IsModelSupported 在提供商映射后检查白名单和平台执行资格。
func (r *Record) IsModelSupported(requestedModel string, defaults ModelMappingDefaults, rules ModelPlatformRules) bool {
	return r.SupportsModel(context.Background(), requestedModel, defaults, rules)
}

// SupportsModel 用当前请求的上下文读取全局设置，再按最终型号检查账号资格。
func (r *Record) SupportsModel(ctx context.Context, requestedModel string, defaults ModelMappingDefaults, rules ModelPlatformRules) bool {
	if r == nil {
		return false
	}
	mapping := ResolveModelMapping(r, defaults)
	model, _ := ResolveMappedModel(mapping, requestedModel)
	scope := r.effectiveModelScope(defaults, mapping)
	if !ModelInFinalWhitelist(r.Platform, model, scope, rules.NormalizeQoder) {
		return false
	}
	global := r.OpenAIOAuthFinalModelPolicy(ctx)
	return global.Allows(strings.TrimSpace(model)) && r.modelPlatformAllows(model, rules, global)
}

// OpenAIOAuthFinalModelPolicy 在核对模型时读取策略，旧调度快照和长连接也能取得最新配置。
func (r *Record) OpenAIOAuthFinalModelPolicy(ctx context.Context) OpenAIOAuthModelPolicy {
	if r == nil || !r.IsOpenAIOAuth() || r.OpenAIOAuthModelWhitelist == nil {
		return OpenAIOAuthModelPolicy{}
	}
	policy, err := r.OpenAIOAuthModelWhitelist(ctx)
	if err != nil {
		return NewOpenAIOAuthModelPolicy(nil)
	}
	return policy
}

// AllowsOpenAIOAuthTarget 在全局白名单启用时复核映射或回退后的具体型号。
func (r *Record) AllowsOpenAIOAuthTarget(policy OpenAIOAuthModelPolicy, finalModel string, defaults ModelMappingDefaults) bool {
	return r.OpenAIOAuthTargetFilter(policy, defaults)(finalModel)
}

// OpenAIOAuthTargetFilter 为同一请求的主模型和工具复用账号白名单。
func (r *Record) OpenAIOAuthTargetFilter(policy OpenAIOAuthModelPolicy, defaults ModelMappingDefaults) func(string) bool {
	if !policy.Configured() {
		return func(string) bool { return true }
	}
	scope := r.effectiveModelScope(defaults, ResolveModelMapping(r, defaults))
	return func(finalModel string) bool {
		model := strings.TrimSpace(finalModel)
		return policy.Allows(model) && IsOpenAIOAuthServableModel(model) && ModelInFinalWhitelist(r.Platform, model, scope, nil)
	}
}

// modelPlatformAllows 检查型号的站点和账号资格。
func (r *Record) modelPlatformAllows(model string, rules ModelPlatformRules, global OpenAIOAuthModelPolicy) bool {
	if r.Platform == PlatformQoder && rules.QoderCompatible != nil && !rules.QoderCompatible(model) {
		return false
	}
	if r.IsOpenAIOAuth() && rules.OpenAIOAuthServable != nil {
		return rules.OpenAIOAuthServable(model, global)
	}
	return true
}

// FinalModelWhitelisted 直接检查已映射的上游模型名是否在白名单中。
func (r *Record) FinalModelWhitelisted(model string, defaults ModelMappingDefaults, rules ModelPlatformRules) bool {
	if r == nil {
		return false
	}
	if r.Platform == PlatformQoder && rules.QoderCompatible != nil && !rules.QoderCompatible(model) {
		return false
	}
	return ModelInFinalWhitelist(r.Platform, model, r.effectiveModelScope(defaults, ResolveModelMapping(r, defaults)), rules.NormalizeQoder) &&
		r.OpenAIOAuthFinalModelPolicy(context.Background()).Allows(strings.TrimSpace(model))
}

// effectiveModelScope 空白名单允许任意型号，Spark 影子使用独立的硬限制。
func (r *Record) effectiveModelScope(defaults ModelMappingDefaults, mapping map[string]string) map[string]struct{} {
	whitelist, explicit := ResolveFinalModelWhitelist(r.Platform, r.Credentials, mapping)
	// Spark 影子只有独立模型配额，明确的通配符也不能扩大其硬能力。
	if r.IsShadow() && r.Platform == PlatformOpenAI {
		scope := make(map[string]struct{})
		if defaults.Models != nil {
			for _, model := range defaults.Models(r) {
				if len(whitelist) == 0 || ModelInFinalWhitelist(r.Platform, model, whitelist, nil) {
					scope[model] = struct{}{}
				}
			}
		}
		return scope
	}
	if explicit && len(whitelist) > 0 {
		return whitelist
	}
	return map[string]struct{}{"*": {}}
}

// GetConfiguredRequestModels 枚举配置中的具体型号和执行路由别名。
func (r *Record) GetConfiguredRequestModels(defaults ModelMappingDefaults) []string {
	if r == nil {
		return nil
	}
	mapping := ResolveModelMapping(r, defaults)
	scope := r.effectiveModelScope(defaults, mapping)
	var platformModels []string
	if defaults.Models != nil {
		platformModels = defaults.Models(r)
	}
	global := r.OpenAIOAuthFinalModelPolicy(context.Background())
	return configuredRequestModels(mapping, scope, platformModels, global, func(model string) bool {
		return global.Allows(model) && ModelInFinalWhitelist(r.Platform, model, scope, nil)
	})
}

// ResolveMappedModel 获取映射后的模型名，并返回是否命中了提供商级映射。
// matched=true 表示命中了精确映射或通配符映射，即使映射结果与原模型名相同。
func ResolveMappedModel(mapping map[string]string, requestedModel string) (mappedModel string, matched bool) {
	if len(mapping) == 0 {
		return requestedModel, false
	}
	if mappedModel, matched := ResolveRequestedModelInMapping(mapping, requestedModel); matched {
		return mappedModel, true
	}
	normalized := strings.TrimSpace(requestedModel)
	if normalized != requestedModel {
		if mappedModel, matched := ResolveRequestedModelInMapping(mapping, normalized); matched {
			return mappedModel, true
		}
	}
	return requestedModel, false
}

// ModelInFinalWhitelist 检查最终模型是否命中白名单，Qoder 按路由键比较别名。
func ModelInFinalWhitelist(platform, model string, whitelist map[string]struct{}, normalizeQoder func(string) string) bool {
	if len(whitelist) == 0 {
		return false
	}
	model = strings.TrimSpace(model)
	for pattern := range whitelist {
		pattern = strings.TrimSpace(pattern)
		if strings.EqualFold(model, pattern) || (strings.HasSuffix(pattern, "*") && strings.HasPrefix(strings.ToLower(model), strings.ToLower(strings.TrimSuffix(pattern, "*")))) {
			return true
		}
		if platform == PlatformQoder && normalizeQoder != nil && normalizeQoder(pattern) == normalizeQoder(model) {
			return true
		}
	}
	return false
}

// HasUnrestrictedModelScope 判断当前提供商是否允许任意型号。
func (r *Record) HasUnrestrictedModelScope(defaults ModelMappingDefaults) bool {
	if r == nil {
		return false
	}
	_, all := r.effectiveModelScope(defaults, ResolveModelMapping(r, defaults))["*"]
	return all && !r.OpenAIOAuthFinalModelPolicy(context.Background()).Configured()
}
