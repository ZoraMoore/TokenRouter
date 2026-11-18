package provider

import (
	textflow "github.com/TokenFlux/TokenRouter/internal/gateway/text"
	"github.com/TokenFlux/TokenRouter/internal/scheduler"
	"github.com/TokenFlux/TokenRouter/internal/scheduler/policy"
)

// SelectionResult 保存选中的执行目标、并发槽和等待计划。
// Lease 管理并发资源，反馈使用本次选择时的参数快照。
type SelectionResult struct {
	Provider                  *ExecutionProvider
	Acquired                  bool
	ReleaseFunc               func()
	WaitPlan                  *scheduler.ProviderWaitPlan
	AdvancedScheduler         bool
	AdvancedSchedulerFeedback *policy.FeedbackConfig
}

// CaptureTextSelection 在候选返回时捕获路线计划、重试上限和提供商快照。
func CaptureTextSelection(provider *ExecutionProvider) textflow.Selection {
	plan, provided := ExecutionCandidatePlan(provider)
	return textflow.Selection{Provider: ExecutionSnapshot(provider), RetryLimit: provider.View().GetPoolModeRetryCount(), Plan: plan, PlanProvided: provided}
}
