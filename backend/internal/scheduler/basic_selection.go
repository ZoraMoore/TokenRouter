package scheduler

import (
	mathrand "math/rand"
	"sort"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
)

// BasicProvider 保存基础排序使用的提供商身份和观测值。
type BasicProvider struct {
	ID               int64
	Type             string
	Priority         int
	LastUsedAt       *time.Time
	SessionWindowEnd *time.Time
}
type BasicCandidate struct {
	Provider *BasicProvider
	Load     *ProviderLoadInfo
}

func FilterByMinPriority(providers []BasicCandidate) []BasicCandidate {
	if len(providers) == 0 {
		return providers
	}
	minPriority := providers[0].Provider.Priority
	for _, acc := range providers[1:] {
		if acc.Provider.Priority < minPriority {
			minPriority = acc.Provider.Priority
		}
	}
	result := make([]BasicCandidate, 0, len(providers))
	for _, acc := range providers {
		if acc.Provider.Priority == minPriority {
			result = append(result, acc)
		}
	}
	return result
}

func FilterByMinLoadRate(providers []BasicCandidate) []BasicCandidate {
	if len(providers) == 0 {
		return providers
	}
	minLoadRate := providers[0].Load.LoadRate
	for _, acc := range providers[1:] {
		if acc.Load.LoadRate < minLoadRate {
			minLoadRate = acc.Load.LoadRate
		}
	}
	result := make([]BasicCandidate, 0, len(providers))
	for _, acc := range providers {
		if acc.Load.LoadRate == minLoadRate {
			result = append(result, acc)
		}
	}
	return result
}

func FilterBySoonestReset(providers []BasicCandidate, now func() time.Time) []BasicCandidate {
	if len(providers) <= 1 {
		return providers
	}
	instant := now()
	var minEnd *time.Time
	for _, acc := range providers {
		end := acc.Provider.SessionWindowEnd
		if end == nil || !instant.Before(*end) {
			continue
		}
		if minEnd == nil || end.Before(*minEnd) {
			minEnd = end
		}
	}
	if minEnd == nil {
		// 所有提供商均无活跃窗口，返回输入集合。
		return providers
	}
	result := make([]BasicCandidate, 0, len(providers))
	for _, acc := range providers {
		end := acc.Provider.SessionWindowEnd
		if end != nil && instant.Before(*end) && end.Equal(*minEnd) {
			result = append(result, acc)
		}
	}
	return result
}

func SelectByLRU(providers []BasicCandidate, preferOAuth bool) *BasicCandidate {
	if len(providers) == 0 {
		return nil
	}
	if len(providers) == 1 {
		return &providers[0]
	}

	// 1. 找到最小的 LastUsedAt（nil 被视为最小）
	var minTime *time.Time
	hasNil := false
	for _, acc := range providers {
		if acc.Provider.LastUsedAt == nil {
			hasNil = true
			break
		}
		if minTime == nil || acc.Provider.LastUsedAt.Before(*minTime) {
			minTime = acc.Provider.LastUsedAt
		}
	}

	// 2. 收集所有具有最小 LastUsedAt 的提供商索引
	var candidateIdxs []int
	for i, acc := range providers {
		if hasNil {
			if acc.Provider.LastUsedAt == nil {
				candidateIdxs = append(candidateIdxs, i)
			}
		} else {
			if acc.Provider.LastUsedAt != nil && acc.Provider.LastUsedAt.Equal(*minTime) {
				candidateIdxs = append(candidateIdxs, i)
			}
		}
	}

	// 3. 如果只有一个候选，直接返回
	if len(candidateIdxs) == 1 {
		return &providers[candidateIdxs[0]]
	}

	// 4. 如果有多个候选且 preferOAuth，优先选择 OAuth 类型
	if preferOAuth {
		var oauthIdxs []int
		for _, idx := range candidateIdxs {
			if providers[idx].Provider.Type == capability.ProviderTypeOAuth {
				oauthIdxs = append(oauthIdxs, idx)
			}
		}
		if len(oauthIdxs) > 0 {
			candidateIdxs = oauthIdxs
		}
	}

	// 5. 随机选择一个
	selectedIdx := candidateIdxs[mathrand.Intn(len(candidateIdxs))]
	return &providers[selectedIdx]
}

func SortProvidersByPriorityAndLastUsed(providers []*BasicProvider, preferOAuth bool) {
	sort.SliceStable(providers, func(i, j int) bool {
		a, b := providers[i], providers[j]
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		switch {
		case a.LastUsedAt == nil && b.LastUsedAt != nil:
			return true
		case a.LastUsedAt != nil && b.LastUsedAt == nil:
			return false
		case a.LastUsedAt == nil && b.LastUsedAt == nil:
			if preferOAuth && a.Type != b.Type {
				return a.Type == capability.ProviderTypeOAuth
			}
			return false
		default:
			return a.LastUsedAt.Before(*b.LastUsedAt)
		}
	})
	ShuffleWithinPriorityAndLastUsed(providers, preferOAuth)
}

func ShuffleWithinSortGroups(providers []BasicCandidate) {
	if len(providers) <= 1 {
		return
	}
	i := 0
	for i < len(providers) {
		j := i + 1
		for j < len(providers) && SameProviderWithLoadGroup(providers[i], providers[j]) {
			j++
		}
		if j-i > 1 {
			mathrand.Shuffle(j-i, func(a, b int) {
				providers[i+a], providers[i+b] = providers[i+b], providers[i+a]
			})
		}
		i = j
	}
}

func SameProviderWithLoadGroup(a, b BasicCandidate) bool {
	if a.Provider.Priority != b.Provider.Priority {
		return false
	}
	if a.Load.LoadRate != b.Load.LoadRate {
		return false
	}
	return SameLastUsedAt(a.Provider.LastUsedAt, b.Provider.LastUsedAt)
}

func ShuffleWithinPriorityAndLastUsed(providers []*BasicProvider, preferOAuth bool) {
	if len(providers) <= 1 {
		return
	}
	i := 0
	for i < len(providers) {
		j := i + 1
		for j < len(providers) && SameProviderGroup(providers[i], providers[j]) {
			j++
		}
		if j-i > 1 {
			if preferOAuth {
				oauth := make([]*BasicProvider, 0, j-i)
				others := make([]*BasicProvider, 0, j-i)
				for _, acc := range providers[i:j] {
					if acc.Type == capability.ProviderTypeOAuth {
						oauth = append(oauth, acc)
					} else {
						others = append(others, acc)
					}
				}
				if len(oauth) > 1 {
					mathrand.Shuffle(len(oauth), func(a, b int) { oauth[a], oauth[b] = oauth[b], oauth[a] })
				}
				if len(others) > 1 {
					mathrand.Shuffle(len(others), func(a, b int) { others[a], others[b] = others[b], others[a] })
				}
				copy(providers[i:], oauth)
				copy(providers[i+len(oauth):], others)
			} else {
				mathrand.Shuffle(j-i, func(a, b int) {
					providers[i+a], providers[i+b] = providers[i+b], providers[i+a]
				})
			}
		}
		i = j
	}
}

func SameProviderGroup(a, b *BasicProvider) bool {
	if a.Priority != b.Priority {
		return false
	}
	return SameLastUsedAt(a.LastUsedAt, b.LastUsedAt)
}

func SameLastUsedAt(a, b *time.Time) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return a.Unix() == b.Unix()
	}
}

func SortProvidersByPriorityOnly(providers []*BasicProvider, preferOAuth bool) {
	sort.SliceStable(providers, func(i, j int) bool {
		a, b := providers[i], providers[j]
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		if preferOAuth && a.Type != b.Type {
			return a.Type == capability.ProviderTypeOAuth
		}
		return false
	})
}

func ShuffleWithinPriority(providers []*BasicProvider, now func() time.Time) {
	if len(providers) <= 1 {
		return
	}
	r := mathrand.New(mathrand.NewSource(now().UnixNano()))
	start := 0
	for start < len(providers) {
		priority := providers[start].Priority
		end := start + 1
		for end < len(providers) && providers[end].Priority == priority {
			end++
		}
		// 对 [start, end) 范围内的提供商随机打乱
		if end-start > 1 {
			r.Shuffle(end-start, func(i, j int) {
				providers[start+i], providers[start+j] = providers[start+j], providers[start+i]
			})
		}
		start = end
	}
}
