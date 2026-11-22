package app

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	gatewayprovider "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	"github.com/TokenFlux/TokenRouter/internal/gateway/requeststate"
	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	providerpostgres "github.com/TokenFlux/TokenRouter/internal/provider/postgres"
	provideradapter "github.com/TokenFlux/TokenRouter/internal/provider/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
)

type maintenanceTargetProviders interface {
	GetByID(context.Context, int64) (*provider.Record, error)
	GetByIDs(context.Context, []int64) ([]*provider.Record, error)
	QueryProvidersByGroup(context.Context, int64, providerpostgres.ProviderGroupQueryOptions) ([]provider.Record, error)
	ListAllWithFilters(context.Context, string, string, string, string, int64, string) ([]provider.Record, error)
}

// maintenanceTargets 将计划范围展开为实际账号和模型，保留管理员可见的跳过原因。
type maintenanceTargets struct {
	providers maintenanceTargetProviders
	groups    interface {
		GetByIDLite(context.Context, int64) (*routing.Group, error)
	}
}

func (t maintenanceTargets) list(ctx context.Context, draft provider.MaintenanceDraft) ([]provider.MaintenanceTarget, error) {
	switch draft.Scope {
	case "groups":
		return t.listByGroups(ctx, draft)
	case "providers":
		return t.listByProviders(ctx, draft)
	case "all":
		if draft.Kind != provider.MaintenanceRelogin {
			return nil, provider.ErrMaintenanceInvalid
		}
		records, err := t.providers.ListAllWithFilters(ctx, provider.PlatformOpenAI, provider.ProviderTypeOAuth, "", "", 0, "")
		if err != nil {
			return nil, err
		}
		slices.SortFunc(records, func(left, right provider.Record) int {
			return cmp.Compare(left.ID, right.ID)
		})
		targets := make([]provider.MaintenanceTarget, 0)
		for i := range records {
			if provider.HasMaintenance401(&records[i]) {
				targets = append(targets, describeMaintenanceTarget(ctx, draft, &records[i], nil))
			}
		}
		return targets, nil
	default:
		return nil, provider.ErrMaintenanceInvalid
	}
}

func (t maintenanceTargets) listByGroups(ctx context.Context, draft provider.MaintenanceDraft) ([]provider.MaintenanceTarget, error) {
	targets := make([]provider.MaintenanceTarget, 0)
	positions := make(map[int64]int)
	for _, groupID := range draft.GroupIDs {
		group, err := t.groups.GetByIDLite(ctx, groupID)
		if apperror.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		records, err := t.providers.QueryProvidersByGroup(ctx, groupID, providerpostgres.ProviderGroupQueryOptions{})
		if err != nil {
			return nil, err
		}
		for i := range records {
			record := &records[i]
			if draft.Kind == provider.MaintenanceRelogin && !provider.HasMaintenance401(record) {
				continue
			}
			pos, exists := positions[record.ID]
			if exists && targets[pos].Reason == "" {
				continue
			}
			target := describeMaintenanceTarget(ctx, draft, record, group)
			if !exists {
				positions[record.ID] = len(targets)
				targets = append(targets, target)
			} else if target.Reason == "" {
				targets[pos] = target
			}
		}
	}
	return targets, nil
}

func (t maintenanceTargets) listByProviders(ctx context.Context, draft provider.MaintenanceDraft) ([]provider.MaintenanceTarget, error) {
	records, err := t.providers.GetByIDs(ctx, draft.ProviderIDs)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]*provider.Record, len(records))
	for _, record := range records {
		byID[record.ID] = record
	}
	targets := make([]provider.MaintenanceTarget, 0, len(draft.ProviderIDs))
	for _, id := range draft.ProviderIDs {
		record := byID[id]
		if record == nil {
			targets = append(targets, provider.MaintenanceTarget{ProviderID: id, Reason: "changed"})
			continue
		}
		targets = append(targets, describeMaintenanceTarget(ctx, draft, record, nil))
	}
	return targets, nil
}

// resolve 复核冻结的账号、归组和模型，配置变化时标记当前轮次的结果。
func (t maintenanceTargets) resolve(ctx context.Context, draft provider.MaintenanceDraft, target provider.MaintenanceTarget) (provider.MaintenanceTarget, error) {
	record, err := t.providers.GetByID(ctx, target.ProviderID)
	if apperror.IsNotFound(err) {
		target.Reason = "changed"
		return target, nil
	}
	if err != nil {
		return target, err
	}
	var group *routing.Group
	if draft.Scope == "groups" {
		if !slices.Contains(draft.GroupIDs, target.GroupID) || !slices.Contains(record.GroupIDs, target.GroupID) {
			target.Reason = "changed"
			return target, nil
		}
		group, err = t.groups.GetByIDLite(ctx, target.GroupID)
		if apperror.IsNotFound(err) {
			target.Reason = "changed"
			return target, nil
		}
		if err != nil {
			return target, err
		}
	}
	current := describeMaintenanceTarget(ctx, draft, record, group)
	target.Reason = current.Reason
	if target.Reason == "" && (target.Model != current.Model || target.ActualModel != current.ActualModel) {
		target.Reason = "changed"
	}
	return target, nil
}

func describeMaintenanceTarget(ctx context.Context, draft provider.MaintenanceDraft, record *provider.Record, group *routing.Group) provider.MaintenanceTarget {
	target := provider.MaintenanceTarget{ProviderID: record.ID, ProviderName: record.Name}
	if group != nil {
		target.GroupID = group.ID
	}
	switch {
	case !provider.SupportsStateProbe(record):
		target.Reason = "unsupported"
	case record.ExpiresAt != nil && !time.Now().Before(*record.ExpiresAt):
		target.Reason = "inactive"
	case draft.Kind == provider.MaintenanceIQ && !record.IsSchedulable():
		target.Reason = "inactive"
	case draft.Kind == provider.MaintenanceRelogin && !record.AllowsMaintenanceAuthCheck():
		target.Reason = "inactive"
	case draft.Kind == provider.MaintenanceRelogin && !provider.HasMaintenance401(record):
		target.Reason = "no_401"
	case group != nil && !group.IsActive():
		target.Reason = "no_group"
	default:
		target.Model, target.ActualModel = maintenanceTargetModel(ctx, draft, record, group)
		if target.Model == "" {
			target.Reason = "model_unavailable"
		}
	}
	return target
}

func maintenanceTargetModel(ctx context.Context, draft provider.MaintenanceDraft, record *provider.Record, group *routing.Group) (string, string) {
	requested := strings.TrimSpace(draft.Model)
	if requested == "" {
		requested = provider.StateProbeDefaultModel
	}
	modelPolicy := gatewayprovider.ModelPolicy{Record: record}
	if group != nil && draft.Kind == provider.MaintenanceIQ {
		// 分组的隐私要求和模型规则决定能否测试。探针和糖果题都发给 Responses。
		if group.RequirePrivacySet && !record.IsPrivacySet() {
			return "", ""
		}
		policy := group.ModelPolicyView()
		mapped := strings.TrimSpace(policy.ResolveModel(requested))
		actual := modelPolicy.Mapped(mapped)
		if !provider.ValidStateProbeModelName(mapped) || !provider.SupportsStateProbeModel(actual) {
			return "", ""
		}
		candidate := gatewayprovider.CatalogueProvider(record, requeststate.AttemptRoute{})
		for _, protocol := range policy.PelicanProtocols(ctx, requested, candidate) {
			if resolved, ok := capability.ResolveRoute(candidate.Protocols(), protocol, policy.ProtocolFallbacks); ok && resolved == capability.ProtocolOpenAIResponses {
				return mapped, actual
			}
		}
		return "", ""
	}
	actual := modelPolicy.Mapped(requested)
	if provider.ValidStateProbeModelName(requested) && provider.SupportsStateProbeModel(actual) && modelPolicy.Supports(ctx, requested) {
		return requested, actual
	}
	if draft.Kind == provider.MaintenanceRelogin && draft.Model == "" {
		// 复检使用账号允许的文字模型。账号的白名单可能未包含探针默认型号。
		for _, model := range record.GetConfiguredRequestModels(provideradapter.ModelDefaults()) {
			actual = modelPolicy.Mapped(model)
			if provider.ValidStateProbeModelName(model) && provider.SupportsStateProbeModel(actual) && modelPolicy.Supports(ctx, model) {
				return model, actual
			}
		}
	}
	return "", ""
}
