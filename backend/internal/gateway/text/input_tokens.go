package text

import (
	"context"

	"github.com/TokenFlux/TokenRouter/internal/gateway/failover"
)

// InputTokensPorts 不提供用户并发、资金提交或完成任务；选择器若交付提供商槽，单次 Forward 负责释放。
type InputTokensPorts interface {
	Context() context.Context
	Select(map[int64]struct{}) (Selection, bool, error)
	SelectionFailed(error, *AttemptFailure, bool)
	Forward(Selection) *AttemptFailure
	ForwardFailed(Selection, error)
	Exhausted(*AttemptFailure)
}

// RunInputTokens 按计数端点的独立预算选择提供商并计数。
func RunInputTokens(p InputTokensPorts, maxSwitches int) {
	excluded := make(map[int64]struct{})
	same := make(map[int64]int)
	var last *AttemptFailure
	switches := 0
	for {
		selected, available, err := p.Select(excluded)
		if err != nil || !available {
			p.SelectionFailed(err, last, available)
			return
		}
		failure := p.Forward(selected)
		if failure == nil {
			return
		}
		if failure.Policy == nil {
			p.ForwardFailed(selected, failure.Cause)
			return
		}
		if !failure.Policy.RetryNext {
			p.Exhausted(failure)
			return
		}
		if failure.Policy.RetryableOnSameProvider && same[selected.Provider.ID] < selected.RetryLimit {
			same[selected.Provider.ID]++
			if !failover.SleepWithContext(p.Context(), failover.SameProviderRetryDelay) {
				return
			}
			continue
		}
		excluded[selected.Provider.ID] = struct{}{}
		last = failure
		if switches >= maxSwitches {
			p.Exhausted(failure)
			return
		}
		switches++
	}
}
