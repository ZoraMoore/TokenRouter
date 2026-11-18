package media

import (
	"context"
	"time"

	protocolopenai "github.com/TokenFlux/TokenRouter/internal/protocol/openai"
	"github.com/TokenFlux/TokenRouter/internal/provider"
)

// EmbeddingResult 固化本次已选提供商执行的计费观测，不携带旧实体或连接。
type EmbeddingResult struct {
	RequestID, Model, BillingModel, UpstreamModel string
	Headers                                       map[string][]string
	Usage                                         protocolopenai.ForwardUsage
	Duration                                      time.Duration
}

// EmbeddingOutcome 分别保存响应提交状态和可切换错误。
type EmbeddingOutcome struct {
	Result        *EmbeddingResult
	Err           error
	Failover      bool
	OutputChanged bool
	StatusCode    int
}

// EmbeddingEvent 只报告编排时点，日志和 HTTP 输出由入口处理。
type EmbeddingEvent struct {
	Kind                            string
	Provider                        provider.ProviderSnapshot
	Outcome                         EmbeddingOutcome
	Excluded, Switches, MaxSwitches int
	Elapsed                         time.Duration
}

// EmbeddingsPorts 提供单次请求的操作，等待阶段复用已选中的提供商。
type EmbeddingsPorts interface {
	SelectEmbedding(context.Context, map[int64]struct{}) (provider.ProviderSnapshot, bool, error)
	AcquireEmbedding(context.Context, provider.ProviderSnapshot) (func(), bool)
	ForwardEmbedding(context.Context, provider.ProviderSnapshot, []byte) EmbeddingOutcome
	ReportEmbedding(context.Context, provider.ProviderSnapshot, *EmbeddingResult, bool, error)
	CompleteEmbedding(context.Context, provider.ProviderSnapshot, *EmbeddingResult)
	SwitchEmbedding(provider.ProviderSnapshot)
	ObserveEmbedding(EmbeddingEvent)
	ClientGone() bool
}

// EmbeddingFailure 记录失败的输出阶段，由 HTTP 适配器生成协议错误。
type EmbeddingFailure struct {
	Stage    string
	Err      error
	Outcome  EmbeddingOutcome
	Excluded int
}

// RunEmbeddings 执行 Embeddings 提供商尝试循环，等待失败时结束当前请求。
// 准入已按原顺序完成，返回前先释放提供商槽，成功后仅提交一次完成处理。
func RunEmbeddings(ctx context.Context, body []byte, maxSwitches int, ports EmbeddingsPorts) *EmbeddingFailure {
	if maxSwitches <= 0 {
		maxSwitches = 3
	}
	excluded := make(map[int64]struct{})
	var last EmbeddingOutcome
	switches := 0
	routingStart := time.Now()
	for {
		selected, present, err := ports.SelectEmbedding(ctx, excluded)
		if err != nil {
			if ports.ClientGone() {
				ports.ObserveEmbedding(EmbeddingEvent{Kind: "select_canceled", Outcome: EmbeddingOutcome{Err: err}})
				return nil
			}
			ports.ObserveEmbedding(EmbeddingEvent{Kind: "select_failed", Outcome: EmbeddingOutcome{Err: err}, Excluded: len(excluded)})
			return &EmbeddingFailure{Stage: "selection", Err: err, Outcome: last, Excluded: len(excluded)}
		}
		if !present {
			return &EmbeddingFailure{Stage: "empty_selection"}
		}
		ports.ObserveEmbedding(EmbeddingEvent{Kind: "selected", Provider: selected})
		release, acquired := ports.AcquireEmbedding(ctx, selected)
		if !acquired {
			return nil
		}
		ports.ObserveEmbedding(EmbeddingEvent{Kind: "routing", Elapsed: time.Since(routingStart)})
		started := time.Now()
		outcome := func() EmbeddingOutcome {
			if release != nil {
				defer release()
			}
			return ports.ForwardEmbedding(ctx, selected, body)
		}()
		ports.ObserveEmbedding(EmbeddingEvent{Kind: "response", Elapsed: time.Since(started)})
		if outcome.Err != nil {
			if outcome.Failover {
				if outcome.OutputChanged {
					return &EmbeddingFailure{Stage: "exhausted", Outcome: outcome}
				}
				ports.ReportEmbedding(ctx, selected, outcome.Result, false, outcome.Err)
				if ports.ClientGone() {
					ports.ObserveEmbedding(EmbeddingEvent{Kind: "forward_canceled", Provider: selected, Outcome: outcome})
					return nil
				}
				ports.SwitchEmbedding(selected)
				excluded[selected.ID] = struct{}{}
				last = outcome
				if switches >= maxSwitches {
					return &EmbeddingFailure{Stage: "exhausted", Outcome: outcome}
				}
				switches++
				ports.ObserveEmbedding(EmbeddingEvent{Kind: "switch", Provider: selected, Outcome: outcome, Switches: switches, MaxSwitches: maxSwitches})
				continue
			}
			ports.ReportEmbedding(ctx, selected, outcome.Result, false, outcome.Err)
			return &EmbeddingFailure{Stage: "forward", Outcome: outcome, Err: outcome.Err}
		}
		ports.ReportEmbedding(ctx, selected, outcome.Result, true, nil)
		ports.CompleteEmbedding(ctx, selected, outcome.Result)
		ports.ObserveEmbedding(EmbeddingEvent{Kind: "completed", Provider: selected, Switches: switches})
		return nil
	}
}
