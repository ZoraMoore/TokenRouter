package scheduler

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/provider"
)

// selectBasicOnlyRoutes 按基础调度的 LRU、粘性溢出和复核顺序选择提供商。
func (s *PlatformSelector) selectBasicOnlyRoutes(ctx context.Context, groupID *int64, platform string, sessionHash string, requestedModel string, routingModel string, excludedIDs map[int64]struct{}, requireCompact bool, stickyProviderID int64, requiredCapability provider.OpenAIEndpointCapability) (*FlowProvider, error) {
	platform = strings.TrimSpace(platform)
	if s.ports.CheckPricing(ctx, groupID, requestedModel) {
		s.diagnostics.event("warn", "group model restriction blocked request",
			"group_id", derefGroupID(groupID),
			"model", requestedModel)
		return nil, fmt.Errorf("%w supporting model: %s (group model restriction)", ErrNoAvailableProviders, requestedModel)
	}

	if provider := s.tryBasicSticky(ctx, groupID, platform, sessionHash, requestedModel, routingModel, excludedIDs, requireCompact, stickyProviderID, requiredCapability); provider != nil {
		return provider, nil
	}

	providers, err := s.ports.ListCandidates(ctx, groupID, platform)
	if err != nil {
		return nil, fmt.Errorf("query providers failed: %w", err)
	}

	selected, compactBlocked := s.SelectBestBasic(ctx, groupID, platform, providers, requestedModel, routingModel, excludedIDs, requireCompact, requiredCapability)

	if selected == nil {
		return nil, s.ports.Unavailable(ctx, requestedModel, routingModel, compactBlocked, "", providers)
	}

	hydrated, err := s.ports.Hydrate(ctx, selected)
	if err != nil {
		return nil, err
	}

	if sessionHash != "" {
		_ = s.ports.SetSticky(ctx, groupID, sessionHash, selected.ID, s.ports.BasicStickyTTL)
	}

	return hydrated, nil
}

func (s *PlatformSelector) tryBasicSticky(ctx context.Context, groupID *int64, platform string, sessionHash, requestedModel string, routingModel string, excludedIDs map[int64]struct{}, requireCompact bool, stickyProviderID int64, requiredCapability provider.OpenAIEndpointCapability) *FlowProvider {
	if sessionHash == "" {
		return nil
	}
	platform = strings.TrimSpace(platform)

	providerID := stickyProviderID
	if providerID <= 0 {
		var err error
		providerID, err = s.ports.GetSticky(ctx, groupID, sessionHash)
		if err != nil || providerID <= 0 {
			return nil
		}
	}

	if _, excluded := excludedIDs[providerID]; excluded {
		return nil
	}

	provider, err := s.ports.GetSchedulable(ctx, providerID)
	if err != nil {
		return nil
	}

	// 检查提供商是否需要清理粘性会话
	if s.ports.ClearSticky(provider, routingModel) || !s.ports.MatchesGroup(provider, groupID) {
		_ = s.ports.DeleteSticky(ctx, groupID, sessionHash)
		return nil
	}

	// 验证提供商是否可用于当前请求
	if !s.ports.BasicEligible(ctx, provider, platform, routingModel, false, requiredCapability) {
		return nil
	}
	if !s.ports.PrivacyAllowed(ctx, groupID, provider) {
		return nil
	}
	if !s.ports.ShadowAllowed(ctx, provider) || !s.ports.ParentHealthy(provider, s.ports.ParentLookup(ctx)) {
		_ = s.ports.DeleteSticky(ctx, groupID, sessionHash)
		return nil
	}
	if s.ports.RuntimeBlocked(provider, routingModel) {
		_ = s.ports.DeleteSticky(ctx, groupID, sessionHash)
		return nil
	}
	provider = s.ports.Recheck(ctx, provider, groupID, platform, routingModel, requireCompact, requiredCapability)
	if provider == nil || !s.ports.MatchesGroup(provider, groupID) {
		_ = s.ports.DeleteSticky(ctx, groupID, sessionHash)
		return nil
	}
	if groupID != nil && s.ports.NeedsGroupCheck(ctx, groupID) &&
		s.ports.GroupModelRestricted(ctx, *groupID, provider, routingModel, requireCompact) {
		_ = s.ports.DeleteSticky(ctx, groupID, sessionHash)
		return nil
	}

	// 刷新会话 TTL 并返回提供商
	// Refresh session TTL and return provider
	_ = s.ports.RefreshSticky(ctx, groupID, sessionHash, s.ports.BasicStickyTTL)
	return provider
}

func (s *PlatformSelector) SelectBestBasic(ctx context.Context, groupID *int64, platform string, providers []FlowProvider, requestedModel string, routingModel string, excludedIDs map[int64]struct{}, requireCompact bool, requiredCapability provider.OpenAIEndpointCapability) (*FlowProvider, bool) {
	platform = strings.TrimSpace(platform)
	compactBlocked := false
	needsUpstreamCheck := s.ports.NeedsGroupCheck(ctx, groupID)
	eligible := make([]*FlowProvider, 0, len(providers))

	for i := range providers {
		acc := &providers[i]

		// 跳过被排除的提供商
		// 跳过已排除的提供商。
		if _, excluded := excludedIDs[acc.ID]; excluded {
			continue
		}

		fresh := s.ports.Fresh(ctx, acc, platform, routingModel, false, requiredCapability)
		if fresh == nil {
			continue
		}
		fresh = s.ports.Recheck(ctx, fresh, groupID, platform, routingModel, false, requiredCapability)
		if fresh == nil {
			continue
		}
		if !s.ports.PrivacyAllowed(ctx, groupID, fresh) {
			continue
		}
		if needsUpstreamCheck && s.ports.GroupModelRestricted(ctx, *groupID, fresh, routingModel, requireCompact) {
			continue
		}
		if requireCompact && !s.ports.CompactAllowed(fresh) {
			compactBlocked = true
			continue
		}

		eligible = append(eligible, fresh)
	}

	if len(eligible) == 0 {
		return nil, compactBlocked
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		a, b := eligible[i], eligible[j]
		return s.isBetterBasic(a, b)
	})
	return eligible[0], compactBlocked
}

func (s *PlatformSelector) isBetterBasic(candidate, current *FlowProvider) bool {
	// 优先级更高（数值更小）
	if candidate.Priority < current.Priority {
		return true
	}
	if candidate.Priority > current.Priority {
		return false
	}

	// 同优先级，比较最后使用时间
	switch {
	case candidate.LastUsedAt == nil && current.LastUsedAt != nil:
		// candidate 从未使用，优先
		return true
	case candidate.LastUsedAt != nil && current.LastUsedAt == nil:
		// current 从未使用，保持
		return false
	case candidate.LastUsedAt == nil && current.LastUsedAt == nil:
		// 都未使用，保持
		return false
	default:
		// 都使用过，选择最久未使用的
		return candidate.LastUsedAt.Before(*current.LastUsedAt)
	}
}

func (s *PlatformSelector) selectBasicRoutes(ctx context.Context, groupID *int64, platform string, sessionHash string, requestedModel string, routingModel string, excludedIDs map[int64]struct{}, requireCompact bool, requiredCapability provider.OpenAIEndpointCapability) (*FlowSelection, error) {
	platform = strings.TrimSpace(platform)
	if s.ports.CheckPricing(ctx, groupID, requestedModel) {
		s.diagnostics.event("warn", "group model restriction blocked request",
			"group_id", derefGroupID(groupID),
			"model", requestedModel)
		return nil, fmt.Errorf("%w supporting model: %s (group model restriction)", ErrNoAvailableProviders, requestedModel)
	}

	cfg := s.ports.Options()
	needsUpstreamCheck := s.ports.NeedsGroupCheck(ctx, groupID)
	var stickyProviderID int64
	if sessionHash != "" && s.ports.CacheAvailable {
		if providerID, err := s.ports.GetSticky(ctx, groupID, sessionHash); err == nil {
			stickyProviderID = providerID
		}
	}
	if s.concurrency == nil || !cfg.LoadBatchEnabled {
		provider, err := s.selectBasicOnlyRoutes(ctx, groupID, platform, sessionHash, requestedModel, routingModel, excludedIDs, requireCompact, stickyProviderID, requiredCapability)
		if err != nil {
			return nil, err
		}
		if !s.ports.PrivacyAllowed(ctx, groupID, provider) {
			return nil, s.ports.Unavailable(ctx, requestedModel, routingModel, false, "", nil)
		}
		result, err := s.ports.Acquire(ctx, provider.ID, provider.Concurrency)
		if err == nil && result != nil && result.Acquired {
			return s.ports.CompleteAcquired(ctx, provider, result.ReleaseFunc)
		}
		if stickyProviderID > 0 && stickyProviderID == provider.ID && s.concurrency != nil {
			waitingCount, _ := s.concurrency.GetProviderWaitingCount(ctx, provider.ID)
			if waitingCount < cfg.StickySessionMaxWaiting {
				return s.ports.Complete(ctx, provider, false, nil, &ProviderWaitPlan{
					ProviderID:     provider.ID,
					MaxConcurrency: provider.Concurrency,
					Timeout:        cfg.StickySessionWaitTimeout,
					MaxWaiting:     cfg.StickySessionMaxWaiting,
				})
			}
		}
		return s.ports.Complete(ctx, provider, false, nil, &ProviderWaitPlan{
			ProviderID:     provider.ID,
			MaxConcurrency: provider.Concurrency,
			Timeout:        cfg.FallbackWaitTimeout,
			MaxWaiting:     cfg.FallbackMaxWaiting,
		})
	}

	providers, err := s.ports.ListCandidates(ctx, groupID, platform)
	if err != nil {
		return nil, err
	}
	if len(providers) == 0 {
		return nil, s.ports.Unavailable(
			ctx,
			requestedModel,
			routingModel,
			false,
			PlatformFilterStats{}.Summary(""),
			providers,
		)
	}

	isExcluded := func(providerID int64) bool {
		if excludedIDs == nil {
			return false
		}
		_, excluded := excludedIDs[providerID]
		return excluded
	}

	// 粘性提供商的有界等待队列已满时，第二层可以为当前请求临时借用其它提供商；
	// 容量溢出的提供商供本次请求使用，会话持久绑定仍指向此前的提供商。
	stickySpillover := false
	if sessionHash != "" {
		providerID := stickyProviderID
		if providerID > 0 && !isExcluded(providerID) {
			provider, err := s.ports.GetSchedulable(ctx, providerID)
			if err == nil {
				clearSticky := s.ports.ClearSticky(provider, routingModel) || !s.ports.MatchesGroup(provider, groupID)
				if clearSticky {
					_ = s.ports.DeleteSticky(ctx, groupID, sessionHash)
				}
				if !clearSticky && s.ports.BasicEligible(ctx, provider, platform, routingModel, false, requiredCapability) && s.ports.PrivacyAllowed(ctx, groupID, provider) {
					provider = s.ports.Recheck(ctx, provider, groupID, platform, routingModel, requireCompact, requiredCapability)
					if provider == nil {
						_ = s.ports.DeleteSticky(ctx, groupID, sessionHash)
					} else if !s.ports.MatchesGroup(provider, groupID) {
						_ = s.ports.DeleteSticky(ctx, groupID, sessionHash)
					} else if s.ports.RuntimeBlocked(provider, routingModel) {
						_ = s.ports.DeleteSticky(ctx, groupID, sessionHash)
					} else if needsUpstreamCheck && s.ports.GroupModelRestricted(ctx, *groupID, provider, routingModel, requireCompact) {
						_ = s.ports.DeleteSticky(ctx, groupID, sessionHash)
					} else if !s.ports.ShadowAllowed(ctx, provider) || !s.ports.ParentHealthy(provider, s.ports.ParentLookup(ctx)) {
						_ = s.ports.DeleteSticky(ctx, groupID, sessionHash)
					} else {
						result, err := s.ports.Acquire(ctx, providerID, provider.Concurrency)
						if err == nil && result != nil && result.Acquired {
							selection, selectErr := s.ports.CompleteAcquired(ctx, provider, result.ReleaseFunc)
							if selectErr != nil {
								return nil, selectErr
							}
							_ = s.ports.RefreshSticky(ctx, groupID, sessionHash, s.ports.BasicStickyTTL)
							return selection, nil
						}

						waitingCount, _ := s.concurrency.GetProviderWaitingCount(ctx, providerID)
						if waitingCount < cfg.StickySessionMaxWaiting {
							return s.ports.Complete(ctx, provider, false, nil, &ProviderWaitPlan{
								ProviderID:     providerID,
								MaxConcurrency: provider.Concurrency,
								Timeout:        cfg.StickySessionWaitTimeout,
								MaxWaiting:     cfg.StickySessionMaxWaiting,
							})
						}
						stickySpillover = true
					}
				}
			}
		}
	}

	parentCacheL2 := make(map[int64]*FlowProvider)
	parentLookupL2 := func(id int64) *FlowProvider {
		if a, ok := parentCacheL2[id]; ok {
			return a
		}
		if s.ports.ReadProviderDB == nil {
			return nil
		}
		a, _ := s.ports.ReadProviderDB(ctx, id)
		parentCacheL2[id] = a
		return a
	}
	baseCandidateCount := 0
	filterStats := PlatformFilterStats{Pool: len(providers)}
	candidates := make([]*FlowProvider, 0, len(providers))
	for i := range providers {
		acc := &providers[i]
		if isExcluded(acc.ID) {
			filterStats.Exclude("excluded")
			continue
		}
		// 调度快照可能暂时过期；在批处理选择前重新检查模型、配额和可调度状态。
		if reason := s.ports.BasicFailureReason(ctx, acc, platform, routingModel, false, requiredCapability); reason != "" {
			filterStats.Exclude(reason)
			continue
		}
		if !s.ports.PrivacyAllowed(ctx, groupID, acc) {
			filterStats.Exclude("privacy_not_set")
			continue
		}
		if !s.ports.ShadowAllowed(ctx, acc) || !s.ports.ParentHealthy(acc, parentLookupL2) {
			filterStats.Exclude("shadow_parent_unhealthy")
			continue
		}
		if s.ports.RuntimeBlocked(acc, routingModel) {
			filterStats.Exclude("runtime_blocked")
			continue
		}
		if needsUpstreamCheck && s.ports.GroupModelRestricted(ctx, *groupID, acc, routingModel, requireCompact) {
			filterStats.Exclude("group_upstream_restricted")
			continue
		}
		baseCandidateCount++
		candidates = append(candidates, acc)
	}

	if len(candidates) == 0 {
		return nil, s.ports.Unavailable(
			ctx,
			requestedModel,
			routingModel,
			false,
			filterStats.Summary(""),
			providers,
		)
	}
	providerLoads := make([]ProviderWithConcurrency, 0, len(candidates))
	for _, acc := range candidates {
		providerLoads = append(providerLoads, ProviderWithConcurrency{
			ID:             acc.ID,
			MaxConcurrency: acc.EffectiveLoadFactor(),
		})
	}

	tryAcquireFromLoadMap := func(loadMap map[int64]*ProviderLoadInfo) (*FlowSelection, bool, error) {
		var available []FlowLoad
		for _, acc := range candidates {
			loadInfo := loadMap[acc.ID]
			if loadInfo == nil {
				loadInfo = &ProviderLoadInfo{ProviderID: acc.ID}
			}
			if loadInfo.LoadRate < 100 {
				available = append(available, FlowLoad{
					Provider: acc,
					LoadInfo: loadInfo,
				})
			}
		}

		if len(available) == 0 {
			return nil, false, nil
		}

		sort.SliceStable(available, func(i, j int) bool {
			a, b := available[i], available[j]
			if a.Provider.Priority != b.Provider.Priority {
				return a.Provider.Priority < b.Provider.Priority
			}
			if a.LoadInfo.LoadRate != b.LoadInfo.LoadRate {
				return a.LoadInfo.LoadRate < b.LoadInfo.LoadRate
			}
			switch {
			case a.Provider.LastUsedAt == nil && b.Provider.LastUsedAt != nil:
				return true
			case a.Provider.LastUsedAt != nil && b.Provider.LastUsedAt == nil:
				return false
			case a.Provider.LastUsedAt == nil && b.Provider.LastUsedAt == nil:
				return false
			default:
				return a.Provider.LastUsedAt.Before(*b.Provider.LastUsedAt)
			}
		})
		flowShuffleWithinSortGroups(available)
		selectionOrder := make([]FlowLoad, 0, len(available))
		if requireCompact {
			// 先尝试快照已启用的提供商，再复核可能已由管理员重新启用的旧快照。
			appendEnabled := func(out []FlowLoad, enabled bool) []FlowLoad {
				for _, item := range available {
					if s.ports.CompactAllowed(item.Provider) == enabled {
						out = append(out, item)
					}
				}
				return out
			}
			selectionOrder = appendEnabled(selectionOrder, true)
			selectionOrder = appendEnabled(selectionOrder, false)
		} else {
			selectionOrder = append(selectionOrder, available...)
		}

		for _, item := range selectionOrder {
			fresh := s.ports.Fresh(ctx, item.Provider, platform, routingModel, false, requiredCapability)
			if fresh == nil {
				continue
			}
			fresh = s.ports.Recheck(ctx, fresh, groupID, platform, routingModel, requireCompact, requiredCapability)
			if fresh == nil {
				continue
			}
			if needsUpstreamCheck && s.ports.GroupModelRestricted(ctx, *groupID, fresh, routingModel, requireCompact) {
				continue
			}
			result, err := s.ports.Acquire(ctx, fresh.ID, fresh.Concurrency)
			if err == nil && result != nil && result.Acquired {
				selection, selectErr := s.ports.CompleteAcquired(ctx, fresh, result.ReleaseFunc)
				if selectErr != nil {
					return nil, true, selectErr
				}
				if sessionHash != "" && !stickySpillover {
					_ = s.ports.SetSticky(ctx, groupID, sessionHash, fresh.ID, s.ports.BasicStickyTTL)
				}
				return selection, true, nil
			}
		}
		return nil, true, nil
	}

	loadMap, err := s.concurrency.GetProvidersLoadBatch(ctx, providerLoads)
	if err != nil {
		ordered := append([]*FlowProvider(nil), candidates...)
		flowSortProvidersByPriorityAndLastUsed(ordered, false)
		if requireCompact {
			ordered = s.prioritizeBasicCompact(ordered)
		}
		for _, acc := range ordered {
			fresh := s.ports.Fresh(ctx, acc, platform, routingModel, false, requiredCapability)
			if fresh == nil {
				continue
			}
			fresh = s.ports.Recheck(ctx, fresh, groupID, platform, routingModel, requireCompact, requiredCapability)
			if fresh == nil {
				continue
			}
			if needsUpstreamCheck && s.ports.GroupModelRestricted(ctx, *groupID, fresh, routingModel, requireCompact) {
				continue
			}
			result, err := s.ports.Acquire(ctx, fresh.ID, fresh.Concurrency)
			if err == nil && result != nil && result.Acquired {
				selection, selectErr := s.ports.CompleteAcquired(ctx, fresh, result.ReleaseFunc)
				if selectErr != nil {
					return nil, selectErr
				}
				if sessionHash != "" && !stickySpillover {
					_ = s.ports.SetSticky(ctx, groupID, sessionHash, fresh.ID, s.ports.BasicStickyTTL)
				}
				return selection, nil
			}
		}
	} else {
		if selection, attempted, selectErr := tryAcquireFromLoadMap(loadMap); selectErr != nil {
			return nil, selectErr
		} else if selection != nil {
			return selection, nil
		} else if attempted {
			if freshLoadMap, loadErr := s.concurrency.GetProvidersLoadBatchFresh(ctx, providerLoads); loadErr == nil {
				if selection, _, selectErr := tryAcquireFromLoadMap(freshLoadMap); selectErr != nil {
					return nil, selectErr
				} else if selection != nil {
					return selection, nil
				}
			}
		}
	}

	flowSortProvidersByPriorityAndLastUsed(candidates, false)
	if requireCompact {
		candidates = s.prioritizeBasicCompact(candidates)
	}
	for _, acc := range candidates {
		fresh := s.ports.Fresh(ctx, acc, platform, routingModel, false, requiredCapability)
		if fresh == nil {
			continue
		}
		fresh = s.ports.Recheck(ctx, fresh, groupID, platform, routingModel, requireCompact, requiredCapability)
		if fresh == nil {
			continue
		}
		if needsUpstreamCheck && s.ports.GroupModelRestricted(ctx, *groupID, fresh, routingModel, requireCompact) {
			continue
		}
		return s.ports.Complete(ctx, fresh, false, nil, &ProviderWaitPlan{
			ProviderID:     fresh.ID,
			MaxConcurrency: fresh.Concurrency,
			Timeout:        cfg.FallbackWaitTimeout,
			MaxWaiting:     cfg.FallbackMaxWaiting,
		})
	}

	if requireCompact && baseCandidateCount > 0 {
		return nil, ErrNoAvailableCompactProviders
	}
	return nil, s.ports.Unavailable(ctx, requestedModel, routingModel, false, "", providers)
}

func (s *PlatformSelector) prioritizeBasicCompact(providers []*FlowProvider) []*FlowProvider {
	if len(providers) == 0 {
		return nil
	}
	enabled := make([]*FlowProvider, 0, len(providers))
	disabled := make([]*FlowProvider, 0, len(providers))
	for _, provider := range providers {
		if s.ports.CompactAllowed(provider) {
			enabled = append(enabled, provider)
		} else {
			disabled = append(disabled, provider)
		}
	}
	return append(enabled, disabled...)
}

// SelectBasicOnly 选择并补全提供商，供无需占槽和注册会话的入口调用。
func (s *PlatformSelector) SelectBasicOnly(ctx context.Context, input PlatformSelectionInput) (*FlowProvider, error) {
	return s.selectBasicOnlyRoutes(ctx, input.GroupID, input.Platform, input.SessionHash, input.RequestedModel, input.RoutingModel, input.ExcludedIDs, input.RequireCompact, input.StickyProviderID, input.RequiredCapability)
}

func (s *PlatformSelector) SelectBasic(ctx context.Context, input PlatformSelectionInput) (*FlowSelection, error) {
	return s.selectBasicRoutes(ctx, input.GroupID, input.Platform, input.SessionHash, input.RequestedModel, input.RoutingModel, input.ExcludedIDs, input.RequireCompact, input.RequiredCapability)
}
