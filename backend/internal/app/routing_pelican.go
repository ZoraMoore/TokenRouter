package app

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/apikey"
	keyhttp "github.com/TokenFlux/TokenRouter/internal/apikey/httpapi"
	"github.com/TokenFlux/TokenRouter/internal/app/lifecycle"
	"github.com/TokenFlux/TokenRouter/internal/config"
	"github.com/TokenFlux/TokenRouter/internal/egress"
	gatewayprovider "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	"github.com/TokenFlux/TokenRouter/internal/gateway/provider/selection"
	"github.com/TokenFlux/TokenRouter/internal/gateway/requeststate"
	"github.com/TokenFlux/TokenRouter/internal/identity/httpapi/authctx"
	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	providerpostgres "github.com/TokenFlux/TokenRouter/internal/provider/postgres"
	"github.com/TokenFlux/TokenRouter/internal/routing"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
	routingpostgres "github.com/TokenFlux/TokenRouter/internal/routing/postgres"
	"github.com/TokenFlux/TokenRouter/internal/scheduler"
	"github.com/TokenFlux/TokenRouter/internal/server/clientip"
	"github.com/TokenFlux/TokenRouter/internal/server/middleware"
)

// pelicanTargets 在领取任务后按当前分组配置选号，管理员指定的账号也经过调度校验。
type pelicanTargets struct {
	groups    *routingpostgres.GroupStore
	providers *providerpostgres.ProviderStore
	selector  *selection.Compatible
}

// pelicanReadAuth 校验作品读取凭据和 IP 限制。读取已保存的 SVG 无需申请模型并发或检查余额。
func pelicanReadAuth(keys *apikey.APIKeyService, cfg *config.Config) gin.HandlerFunc {
	options := keyhttp.AuthenticationOptions{
		ClientIP: func(c *gin.Context) string {
			return clientip.GetSecurityClientIP(c, cfg.TrustForwardedIPForAPIKeyACL())
		},
		AbuseClientKey: middleware.InvalidAuthClientKey,
		NonConsuming:   func(*gin.Context) bool { return true },
	}
	return func(c *gin.Context) {
		access, ok := keyhttp.Authenticate(c, keys, options)
		if !ok {
			return
		}
		key := access.KeyView()
		if key.TeamOwnerDisabled {
			keyhttp.AbortWithError(c, http.StatusUnauthorized, "API_KEY_DISABLED", "API key is disabled")
			return
		}
		if key.Status == apikey.StatusAPIKeyExpired || key.IsExpired() {
			keyhttp.AbortWithError(c, http.StatusForbidden, "API_KEY_EXPIRED", "The API key has expired.")
			return
		}
		c.Set(string(keyhttp.ContextKeyAPIKey), key)
		c.Set(authctx.ContextKeyUser, authctx.AuthSubject{UserID: access.ActorUserID})
		keyhttp.SetAccessPrincipal(c, access)
		c.Next()
	}
}

// pelicanKeyScope 只传递已认证 Key 的所有者和绑定分组；服务还会核对当前分组权限。
func pelicanKeyScope(c *gin.Context) (int64, []int64, bool) {
	key, ok := keyhttp.GetAPIKeyFromContext(c)
	if !ok || key == nil || key.UserID <= 0 {
		return 0, nil, false
	}
	var groups []int64
	if key.IsComposite {
		for _, binding := range key.CompositeGroups {
			groups = append(groups, binding.GroupID)
		}
	} else if key.GroupID != nil {
		groups = append(groups, *key.GroupID)
	}
	return key.UserID, groups, true
}

// providePelicanService 为智能运维绑定测试队列、账号调度和用户分组权限。
func providePelicanService(
	repo *routingpostgres.PelicanTestStore,
	groups *routingpostgres.GroupStore,
	providers *providerpostgres.ProviderStore,
	keys *apikey.APIKeyService,
	tests *provider.TestService,
	selector *selection.Compatible,
	cfg *config.Config,
	manager *lifecycle.Manager,
) *routing.PelicanService {
	targets := pelicanTargets{groups: groups, providers: providers, selector: selector}
	repo.BindCandyDisabler(providers.DisableAfterCandyAnswer)
	location := time.Local
	if configured, err := time.LoadLocation(cfg.Timezone); cfg.Timezone != "" && err == nil {
		location = configured
	}
	service := routing.NewPelicanService(repo, routing.PelicanOptions{
		Now:             func() time.Time { return time.Now().In(location) },
		Providers:       targets.list,
		Validate:        targets.validate,
		Acquire:         targets.acquire,
		Run:             tests.RunConfiguredPelican,
		RunStateProbe:   tests.RunStateProbe,
		UserGroups:      keys.GetAvailableGroups,
		Candy:           repo,
		CandyCandidates: targets.candyCandidates,
		CandyChanged:    providers.PublishCandyScheduling,
	})
	runner := routing.NewPelicanRunner(service)
	manager.Register(lifecycle.Hook{
		Name: "PelicanGroupTests", StartOrder: 981, StopOrder: 19,
		Start: runner.Start, Stop: runner.Stop,
	})
	return service
}

func (t pelicanTargets) list(ctx context.Context, groupID int64) ([]routing.PelicanProvider, error) {
	rows, err := t.providers.ListByGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	choices := make([]routing.PelicanProvider, 0, len(rows))
	for i := range rows {
		choices = append(choices, routing.PelicanProvider{
			ID: rows[i].ID, Name: rows[i].Name, SupportsStateProbe: provider.SupportsStateProbe(&rows[i]),
		})
	}
	return choices, nil
}

func (t pelicanTargets) validate(ctx context.Context, plan routing.PelicanPlan) error {
	group, err := t.groups.GetByIDLite(ctx, plan.GroupID)
	if err != nil {
		return err
	}
	if plan.Enabled && !group.IsActive() {
		return apperror.BadRequest("PELICAN_GROUP_DISABLED", "Enable the group before scheduling a test.")
	}
	if plan.ProviderID == nil {
		return nil
	}
	groupIDs, err := t.providers.LoadProviderGroupIDs(ctx, *plan.ProviderID)
	if err != nil {
		return err
	}
	if !slices.Contains(groupIDs, plan.GroupID) {
		return apperror.BadRequest("PELICAN_PROVIDER_OUTSIDE_GROUP", "Select a provider that belongs to this group.")
	}
	if plan.QuestionKind == routing.PelicanStateProbe {
		record, err := t.providers.GetByID(ctx, *plan.ProviderID)
		if err != nil {
			return err
		}
		if !pelicanProbeTarget(record, plan, group.ModelPolicyView()) {
			return apperror.BadRequest("PELICAN_PROBE_UNSUPPORTED", "这个账号或映射后的模型不支持 Codex 探针")
		}
	}
	return nil
}

// pelicanProbeTarget 核对固定账号和两层映射后的文字模型。计划保存后，分组和账号设置仍可能变化。
func pelicanProbeTarget(record *provider.Record, plan routing.PelicanPlan, policy *routing.GroupPolicyView) bool {
	if plan.ProviderID == nil || record == nil || record.ID != *plan.ProviderID || !provider.SupportsStateProbe(record) {
		return false
	}
	requested := policy.ResolveModel(plan.ModelID)
	mapped := (gatewayprovider.ModelPolicy{Record: record}).Mapped(requested)
	return provider.ValidStateProbeModelName(requested) && provider.SupportsStateProbeModel(mapped)
}

// candyCandidates 冻结本轮允许测试的账号，实际测试仍检查空闲槽位。
func (t pelicanTargets) candyCandidates(ctx context.Context, plan routing.PelicanPlan) ([]int64, error) {
	group, err := t.groups.GetByIDLite(ctx, plan.GroupID)
	if err != nil {
		return nil, err
	}
	if !group.IsActive() {
		return nil, routing.ErrPelicanNotFound
	}
	records, err := t.providers.ListSchedulableByGroupID(ctx, plan.GroupID)
	if err != nil {
		return nil, err
	}
	candidates := pelicanCandidates(ctx, plan, group, group.ModelPolicyView(), records)
	ids := make(map[int64]struct{})
	for _, protocol := range group.AllowedProtocols {
		for id := range candidates[protocol] {
			ids[id] = struct{}{}
		}
	}
	result := make([]int64, 0, len(ids))
	for id := range ids {
		result = append(result, id)
	}
	slices.Sort(result)
	return result, nil
}

// acquire 在当前分组中查找能运行该文字题的账号，并持有它的并发槽位直到作答结束。
func (t pelicanTargets) acquire(ctx context.Context, plan routing.PelicanPlan) (routing.PelicanTarget, func(), error) {
	group, err := t.groups.GetByIDLite(ctx, plan.GroupID)
	if err != nil {
		return routing.PelicanTarget{}, nil, err
	}
	if !group.IsActive() {
		return routing.PelicanTarget{}, nil, routing.ErrPelicanNotFound
	}
	records, err := t.providers.ListSchedulableByGroupID(ctx, plan.GroupID)
	if err != nil {
		return routing.PelicanTarget{}, nil, err
	}
	policy := group.ModelPolicyView()
	candidates := pelicanCandidates(ctx, plan, group, policy, records)
	mapped := policy.ResolveModel(plan.ModelID)
	ctx = requeststate.WithGroup(ctx, group)
	for _, source := range group.AllowedProtocols {
		eligible := candidates[source]
		if len(eligible) == 0 {
			continue
		}
		excluded := make(map[int64]struct{}, len(records)-len(eligible))
		for i := range records {
			if _, ok := eligible[records[i].ID]; !ok {
				excluded[records[i].ID] = struct{}{}
			}
		}
		route := routing.Plan(routing.PlanInput{
			Group:          group,
			RequestedModel: plan.ModelID,
			ClientProtocol: source,
			GroupMapping: routing.GroupMappingResult{
				MappedModel:            mapped,
				Mapped:                 mapped != plan.ModelID,
				RestrictModels:         policy.RestrictModels,
				RestrictionModelSource: policy.RestrictionSource(),
			},
		})
		selectCtx := requeststate.WithRoutePlan(requeststate.WithClientProtocol(ctx, source), route)
		// 使用网关的并发感知调度。测试只使用空闲槽位，忙碌的账号留给现有请求。
		selected, _, err := t.selector.SelectProviderWithScheduler(selectCtx, &plan.GroupID, "", "", plan.ModelID, excluded, egress.OpenAIUpstreamTransportAny, false)
		if err != nil || selected == nil || !selected.Acquired || selected.ReleaseFunc == nil {
			continue
		}
		record := &selected.Provider.Record
		chosen, ok := route.ResolveCandidate(record.RoutingSnapshot())
		if _, allowed := eligible[record.ID]; !ok || !allowed || plan.QuestionKind == routing.PelicanStateProbe && chosen.UpstreamProtocol != capability.ProtocolOpenAIResponses {
			selected.ReleaseFunc()
			continue
		}
		return routing.PelicanTarget{
			ProviderID: record.ID, ModelID: mapped,
			Protocol: pelicanTestProtocol(record, chosen.UpstreamProtocol), GroupUpdatedAt: group.UpdatedAt,
		}, selected.ReleaseFunc, nil
	}
	return routing.PelicanTarget{}, nil, scheduler.ErrNoAvailableProviders
}

// pelicanCandidates 按管理员选择的账号和分组模型白名单筛选各文字协议的候选。
func pelicanCandidates(ctx context.Context, plan routing.PelicanPlan, group *routing.Group, policy *routing.GroupPolicyView, records []provider.Record) map[capability.ProtocolID]map[int64]struct{} {
	out := make(map[capability.ProtocolID]map[int64]struct{})
	allowed := make(map[int64]struct{}, len(plan.CandidateIDs))
	for _, id := range plan.CandidateIDs {
		allowed[id] = struct{}{}
	}
	for i := range records {
		record := &records[i]
		if _, ok := allowed[record.ID]; plan.CandidateIDs != nil && !ok {
			continue
		}
		if plan.ProviderID != nil && *plan.ProviderID != record.ID || group.RequirePrivacySet && !record.IsPrivacySet() {
			continue
		}
		if plan.QuestionKind == routing.PelicanStateProbe && !pelicanProbeTarget(record, plan, policy) {
			continue
		}
		candidate := gatewayprovider.CatalogueProvider(record, requeststate.AttemptRoute{})
		for _, protocol := range policy.PelicanProtocols(ctx, plan.ModelID, candidate) {
			if out[protocol] == nil {
				out[protocol] = make(map[int64]struct{})
			}
			out[protocol][record.ID] = struct{}{}
		}
	}
	return out
}

// pelicanTestProtocol 把调度选定的上游协议传给支持多种文字端点的测试适配器。
func pelicanTestProtocol(record *provider.Record, target capability.ProtocolID) string {
	switch record.Platform {
	case provider.PlatformOpenAI, provider.PlatformKimi, provider.PlatformZhipu, provider.PlatformDeepseek:
		switch target {
		case capability.ProtocolAnthropicMessages:
			return provider.APIProtocolAnthropic
		case capability.ProtocolOpenAIResponses:
			return provider.APIProtocolResponses
		case capability.ProtocolOpenAIChatCompletions:
			return provider.APIProtocolChatCompletions
		}
	}
	return ""
}
