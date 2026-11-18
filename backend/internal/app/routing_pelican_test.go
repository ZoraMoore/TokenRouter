package app

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/apikey"
	keyhttp "github.com/TokenFlux/TokenRouter/internal/apikey/httpapi"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
)

// TestPelicanKeyScope 检查普通 Key、复合 Key 和未绑定分组的访问范围。
func TestPelicanKeyScope(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	_, _, ok := pelicanKeyScope(c)
	require.False(t, ok)
	groupID := int64(9)
	key := &apikey.APIKey{UserID: 17, GroupID: &groupID}
	c.Set(string(keyhttp.ContextKeyAPIKey), key)
	owner, groups, ok := pelicanKeyScope(c)
	require.True(t, ok)
	require.EqualValues(t, 17, owner)
	require.Equal(t, []int64{9}, groups)
	key.IsComposite = true
	key.CompositeGroups = []apikey.APIKeyCompositeGroup{{GroupID: 3}, {GroupID: 4}}
	_, groups, ok = pelicanKeyScope(c)
	require.True(t, ok)
	require.Equal(t, []int64{3, 4}, groups)
	key.IsComposite = false
	key.GroupID = nil
	_, groups, ok = pelicanKeyScope(c)
	require.True(t, ok)
	require.Empty(t, groups)
}

// TestPelicanCandidatesHonorAccountAndModel 检查分组和提供商两层映射，以及指定账号不可用时的候选集合。
func TestPelicanCandidatesHonorAccountAndModel(t *testing.T) {
	group := &routing.Group{
		ID: 8, AllowedProtocols: []capability.ProtocolID{capability.ProtocolOpenAIResponses},
		RoutingPolicy: routing.GroupRoutingPolicy{
			Enabled: true, RestrictModels: true, RestrictionModelSource: routing.BillingModelSourceUpstream,
			ModelMapping: map[string]string{"public-model": "group-model"}, AllowedModels: []string{"vendor-model"},
		},
	}
	records := []provider.Record{
		{ID: 31, Platform: capability.PlatformOpenAI, Type: capability.ProviderTypeAPIKey, Credentials: map[string]any{
			"model_mapping": map[string]any{"group-model": "vendor-model"}, "model_whitelist": []string{"vendor-model"},
		}},
		{ID: 32, Platform: capability.PlatformOpenAI, Type: capability.ProviderTypeAPIKey, Credentials: map[string]any{
			"model_mapping": map[string]any{"group-model": "other-vendor-model"},
		}},
	}
	plan := routing.PelicanPlan{GroupID: group.ID, ModelID: "public-model"}
	candidates := pelicanCandidates(t.Context(), plan, group, group.ModelPolicyView(), records)
	require.Equal(t, map[int64]struct{}{31: {}}, candidates[capability.ProtocolOpenAIResponses])

	plan.ProviderID = &records[1].ID
	require.Empty(t, pelicanCandidates(t.Context(), plan, group, group.ModelPolicyView(), records))
	plan.ProviderID = &records[0].ID
	require.NotEmpty(t, pelicanCandidates(t.Context(), plan, group, group.ModelPolicyView(), records))

	group.AllowedProtocols = []capability.ProtocolID{capability.ProtocolEmbeddings}
	require.Empty(t, pelicanCandidates(t.Context(), plan, group, group.ModelPolicyView(), records))
}

// TestPelicanStateProbeCandidates 验证定时执行时会重新检查账号类型与两层模型映射。
func TestPelicanStateProbeCandidates(t *testing.T) {
	group := &routing.Group{
		ID: 8, AllowedProtocols: []capability.ProtocolID{capability.ProtocolOpenAIResponses},
		RoutingPolicy: routing.GroupRoutingPolicy{Enabled: true, ModelMapping: map[string]string{"public": "group-model"}},
	}
	rows := []provider.Record{
		{ID: 1, Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeOAuth},
		{ID: 2, Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeAPIKey},
		{
			ID: 3, Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeOAuth,
			Credentials: map[string]any{"model_mapping": map[string]any{"group-model": "gpt-image-1"}},
		},
	}
	plan := routing.PelicanPlan{QuestionKind: routing.PelicanStateProbe, GroupID: group.ID, ModelID: "public"}
	candidates := func() map[capability.ProtocolID]map[int64]struct{} {
		return pelicanCandidates(t.Context(), plan, group, group.ModelPolicyView(), rows)
	}
	require.Empty(t, candidates())
	plan.ProviderID = &rows[0].ID
	require.Equal(t, map[int64]struct{}{1: {}}, candidates()[capability.ProtocolOpenAIResponses])
	plan.ProviderID = &rows[1].ID
	require.Empty(t, candidates())
	plan.ProviderID = &rows[2].ID
	require.Empty(t, candidates())
	group.RoutingPolicy.ModelMapping["public"] = "invalid!"
	rows[2].Credentials["model_mapping"] = map[string]any{"invalid!": "gpt-6-astra"}
	require.False(t, pelicanProbeTarget(&rows[2], plan, group.ModelPolicyView()))
}
