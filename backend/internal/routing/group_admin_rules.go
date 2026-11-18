package routing

import (
	"context"
	"fmt"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/protocol"
)

// ValidateUnavailableFallbackGroup 校验分组不可用时的指定回退分组。
// 该回退会继承入口平台语义，因此必须指向同平台且当前可用的分组。
func (s *GroupAdmin) ValidateUnavailableFallbackGroup(ctx context.Context, currentGroupID int64, platform string, fallbackGroupID int64) error {
	if currentGroupID > 0 && currentGroupID == fallbackGroupID {
		return fmt.Errorf("cannot set self as unavailable fallback group")
	}
	fallbackGroup, err := s.groupRepo.GetByIDLite(ctx, fallbackGroupID)
	if err != nil {
		return fmt.Errorf("unavailable fallback group not found: %w", err)
	}
	if !fallbackGroup.IsActive() {
		return fmt.Errorf("unavailable fallback group must be active")
	}
	return nil
}

func FilterModelsListCandidates(candidates []string, selectedModels []string) []string {
	normalizedSelected := NormalizeGroupModelsListConfig(GroupModelsListConfig{
		Enabled: true,
		Models:  selectedModels,
	}).Models
	if len(normalizedSelected) == 0 {
		return nil
	}

	if len(candidates) == 0 {
		return nil
	}

	allowed := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" {
			allowed = append(allowed, candidate)
		}
	}

	// 按自定义模型列表顺序输出，确保探测下拉与管理员配置顺序一致。
	filtered := make([]string, 0, len(normalizedSelected))
	for _, model := range normalizedSelected {
		if ModelsListCandidateAllowsModel(allowed, model) {
			filtered = append(filtered, model)
		}
	}
	return filtered
}

func ModelsListCandidateAllowsModel(availablePatterns []string, model string) bool {
	for _, pattern := range availablePatterns {
		if pattern == model {
			return true
		}
		if strings.HasSuffix(pattern, "*") && strings.HasPrefix(model, strings.TrimSuffix(pattern, "*")) {
			return true
		}
	}
	return false
}

func SanitizeGroupReasoningEffortPolicy(group *Group) {
	if group == nil {
		return
	}
	maxEffort, maxErr := NormalizeMaxReasoningEffortForPlatform(PlatformOpenAI, group.MaxReasoningEffort)
	mappings, mappingsErr := NormalizeReasoningEffortMappings(PlatformOpenAI, group.ReasoningEffortMappings)
	if maxErr != nil {
		maxEffort = ""
	}
	if mappingsErr != nil {
		mappings = []ReasoningEffortMapping{}
	}
	overLimit := NormalizeMaxReasoningEffortOverLimit(group.MaxReasoningEffortOverLimit)
	if overLimit == "" {
		overLimit = ReasoningEffortOverLimitDowngrade
	}
	group.MaxReasoningEffort = maxEffort
	group.MaxReasoningEffortOverLimit = overLimit
	group.ReasoningEffortMappings = mappings
}

func SanitizeGroupMessagesDispatchFields(g *Group) {
	if g == nil {
		return
	}
	// 派生镜像只表达入口准入；模型映射是否适用由实际执行提供商判断。
	g.AllowMessagesDispatch = g.AllowsClientProtocol(protocol.ProtocolAnthropicMessages)
}
