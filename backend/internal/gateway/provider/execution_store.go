package provider

import (
	"context"
	"time"

	providercore "github.com/TokenFlux/TokenRouter/internal/provider"
)

// ExecutionProviderStore 定义执行过程中读取和更新提供商的存储操作。
type ExecutionProviderStore interface {
	GetByID(ctx context.Context, id int64) (*ExecutionProvider, error)
	// GetByIDs 保持单次批量查询，忽略缺失 ID。
	GetByIDs(ctx context.Context, ids []int64) ([]*ExecutionProvider, error)

	Update(ctx context.Context, provider *ExecutionProvider) error

	ListByPlatform(ctx context.Context, platform string) ([]ExecutionProvider, error)

	UpdateLastUsed(ctx context.Context, id int64) error
	BatchUpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error
	SetError(ctx context.Context, id int64, errorMsg string) error
	ClearError(ctx context.Context, id int64) error

	ListSchedulable(ctx context.Context) ([]ExecutionProvider, error)
	ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]ExecutionProvider, error)
	ListSchedulableByPlatform(ctx context.Context, platform string) ([]ExecutionProvider, error)
	ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]ExecutionProvider, error)
	ListSchedulableByPlatforms(ctx context.Context, platforms []string) ([]ExecutionProvider, error)
	ListSchedulableByGroupIDAndPlatforms(ctx context.Context, groupID int64, platforms []string) ([]ExecutionProvider, error)
	ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]ExecutionProvider, error)
	ListSchedulableUngroupedByPlatforms(ctx context.Context, platforms []string) ([]ExecutionProvider, error)
	// ListModelAvailabilityCandidates 返回持久配置为 active 且 schedulable 的模型诊断候选提供商，
	// 不过滤限流、过载、临时不可调度或到期窗口等瞬时状态。groupID 为 nil 时，
	// includeGrouped 决定查询全部匹配提供商，还是只查询没有分组绑定的提供商。
	ListModelAvailabilityCandidates(ctx context.Context, groupID *int64, platforms []string, includeGrouped bool) ([]ExecutionProvider, error)

	SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error
	SetModelRateLimit(ctx context.Context, id int64, scope string, resetAt time.Time, reason ...string) error
	SetOverloaded(ctx context.Context, id int64, until time.Time) error
	SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error
	ClearTempUnschedulable(ctx context.Context, id int64) error
	ClearRateLimit(ctx context.Context, id int64) error
	ClearAntigravityQuotaScopes(ctx context.Context, id int64) error
	ClearModelRateLimits(ctx context.Context, id int64) error
	UpdateSessionWindow(ctx context.Context, id int64, start, end *time.Time, status string) error
	// UpdateSessionWindowEnd 更新 5h 窗口的结束时间。
	// 用于 active poll 拿到新 ResetsAt 后回写，避免覆盖请求路径上记录的 status。
	UpdateSessionWindowEnd(ctx context.Context, id int64, end time.Time) error
	UpdateExtra(ctx context.Context, id int64, updates map[string]any) error

	// IncrementQuotaUsed 原子递增 API Key 提供商的配额用量（总/日/周）
	IncrementQuotaUsed(ctx context.Context, id int64, amount float64) error
	BulkUpdate(ctx context.Context, ids []int64, updates providercore.ProviderBulkUpdate) (int64, error)
}
