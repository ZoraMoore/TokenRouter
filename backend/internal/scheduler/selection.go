package scheduler

import (
	"context"

	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing"
)

// SelectionCandidate 保存候选快照和本次观测，关联号由当前选择调用管理。
type SelectionCandidate struct {
	Snapshot     *provider.ProviderSnapshot
	ProjectionID uint64
	Load         *ProviderLoadInfo
	LoadKnown    bool
	Plan         *routing.CandidatePlan
}

// SelectionInput 保存本次选择的最终分组、请求能力和候选列表。
type SelectionInput struct {
	GroupID               *int64
	RequestedModel        string
	RoutePlan             routing.RoutePlan
	Candidates            []SelectionCandidate
	ExcludedIDs           map[int64]struct{}
	RequireCompact        bool
	SessionHash           string
	PreserveStickyBinding bool
}

// AttemptSelectionPorts 提供获取槽位、刷新候选和数据库复核的函数，各步骤分别检查预算。
// gateway 根据返回的候选执行上游请求和重试。
type AttemptSelectionPorts struct {
	Acquire        func(context.Context, int64, int) (*AcquireResult, bool, error)
	Fresh          func(context.Context, SelectionCandidate) (SelectionCandidate, bool)
	CanRecheck     func() bool
	Recheck        func(context.Context, SelectionCandidate) (SelectionCandidate, bool)
	CompactAllowed func(SelectionCandidate) bool
	BindSticky     func(context.Context, SelectionCandidate)
}
type SelectionResult struct {
	Candidate      SelectionCandidate
	Attempt        *AttemptLease
	CompactBlocked bool
}

// Select 接管每次已取得槽位，所有补全、协议与资格复核失败均通过 AttemptLease 归还。
// 候选按传入顺序尝试，上游请求和重试由 gateway 执行。
func (l *Lease) Select(ctx context.Context, input SelectionInput, ports AttemptSelectionPorts) (SelectionResult, bool, error) {
	ctx = WithRequestLease(ctx, l)
	output := SelectionResult{}
	for _, candidate := range input.Candidates {
		if candidate.Snapshot == nil {
			continue
		}
		if _, excluded := input.ExcludedIDs[candidate.Snapshot.ID]; excluded {
			continue
		}
		if candidate.LoadKnown && candidate.Snapshot.Concurrency > 0 && candidate.Load.CurrentConcurrency >= candidate.Snapshot.Concurrency {
			continue
		}
		acquired, attempted, err := ports.Acquire(ctx, candidate.Snapshot.ID, candidate.Snapshot.Concurrency)
		if !attempted {
			break
		}
		if err != nil {
			return output, false, err
		}
		if acquired == nil || !acquired.Acquired {
			continue
		}
		attempt := NewAttemptLease(l, nil, acquired.ReleaseFunc)
		fresh, ok := ports.Fresh(ctx, candidate)
		if !ok {
			attempt.Release()
			continue
		}
		if !ports.CanRecheck() {
			attempt.Release()
			break
		}
		fresh, ok = ports.Recheck(ctx, fresh)
		if !ok {
			attempt.Release()
			continue
		}
		if input.RequireCompact && !ports.CompactAllowed(fresh) {
			output.CompactBlocked = true
			attempt.Release()
			continue
		}
		if fresh.Snapshot.Concurrency != candidate.Snapshot.Concurrency {
			attempt.Release()
			acquired, attempted, err = ports.Acquire(ctx, fresh.Snapshot.ID, fresh.Snapshot.Concurrency)
			if !attempted {
				continue
			}
			if err != nil {
				return output, false, err
			}
			if acquired == nil || !acquired.Acquired {
				continue
			}
			attempt = NewAttemptLease(l, nil, acquired.ReleaseFunc)
		}
		if input.SessionHash != "" && !input.PreserveStickyBinding && ports.BindSticky != nil {
			ports.BindSticky(ctx, fresh)
		}
		output.Candidate = fresh
		output.Attempt = attempt
		return output, true, nil
	}
	return output, false, nil
}
