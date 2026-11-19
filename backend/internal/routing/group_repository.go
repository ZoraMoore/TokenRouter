package routing

import (
	"context"

	infraerrors "github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
	"github.com/TokenFlux/TokenRouter/internal/pkg/pagination"
)

var (
	ErrGroupNotFound = infraerrors.NotFound("GROUP_NOT_FOUND", "group not found")
	ErrGroupExists   = infraerrors.Conflict("GROUP_EXISTS", "group name already exists")
)

type GroupRepository interface {
	Create(ctx context.Context, group *Group) error
	GetByID(ctx context.Context, id int64) (*Group, error)
	GetByIDLite(ctx context.Context, id int64) (*Group, error)
	Update(ctx context.Context, group *Group) error
	Delete(ctx context.Context, id int64) error
	DeleteCascade(ctx context.Context, id int64) ([]int64, error)

	List(ctx context.Context, params pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error)
	ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, status, search string, isExclusive *bool) ([]Group, *pagination.PaginationResult, error)
	ListActive(ctx context.Context) ([]Group, error)

	ExistsByName(ctx context.Context, name string) (bool, error)
	GetProviderCount(ctx context.Context, groupID int64) (total int64, active int64, err error)
	DeleteProviderGroupsByGroupID(ctx context.Context, groupID int64) (int64, error)
	// GetProviderIDsByGroupIDs 获取多个分组的所有提供商 ID（去重）
	GetProviderIDsByGroupIDs(ctx context.Context, groupIDs []int64) ([]int64, error)
	// BindProvidersToGroup 将多个提供商绑定到指定分组
	BindProvidersToGroup(ctx context.Context, groupID int64, providerIDs []int64) error
	// UpdateSortOrders 批量更新分组排序
	UpdateSortOrders(ctx context.Context, updates []GroupSortOrderUpdate) error
}

// GroupAdminViews 读取管理页面和选择器需要的分组字段。
type GroupAdminViews interface {
	ListAdminGroupOptions(context.Context, bool) ([]AdminGroupOption, error)
	ListAdminGroupTable(context.Context, pagination.PaginationParams, string, string, *bool) ([]AdminGroupTableRow, *pagination.PaginationResult, error)
}

type GroupDuplicateRepository interface {
	// FindByDuplicateOperationID 在幂等存储结果不明确时执行只读恢复查询。
	FindByDuplicateOperationID(ctx context.Context, operationID string) (*Group, error)
	// CreateFromSource 原子保存分组、源分组提供商优先级和调度 outbox 事件。
	CreateFromSource(ctx context.Context, group *Group, sourceGroupID int64) error
}

// GroupSortOrderRepository 串行化新建分组的末尾排序位置分配。
type GroupSortOrderRepository interface {
	LockGroupSortOrder(ctx context.Context) error
}

// GroupSortOrderUpdate 分组排序更新。
type GroupSortOrderUpdate struct {
	ID        int64 `json:"id"`
	SortOrder int   `json:"sort_order"`
}

// AdminGroupOption 提供分组绑定、排序和回退选项需要的字段。
type AdminGroupOption struct {
	ID                              int64   `json:"id"`
	Name                            string  `json:"name"`
	Description                     string  `json:"description"`
	Status                          string  `json:"status"`
	IsExclusive                     bool    `json:"is_exclusive"`
	DisplayBrand                    string  `json:"display_brand"`
	RateMultiplier                  float64 `json:"rate_multiplier"`
	SortOrder                       int     `json:"sort_order"`
	ClaudeCodeOnly                  bool    `json:"claude_code_only"`
	FallbackGroupIDOnInvalidRequest *int64  `json:"fallback_group_id_on_invalid_request"`
	ProviderCount                   int64   `json:"provider_count"`
}

// AdminGroupTableRow 提供管理表格的分组属性和提供商计数。
type AdminGroupTableRow struct {
	ID                       int64   `json:"id"`
	Name                     string  `json:"name"`
	Status                   string  `json:"status"`
	IsExclusive              bool    `json:"is_exclusive"`
	DisplayBrand             string  `json:"display_brand"`
	RateMultiplier           float64 `json:"rate_multiplier"`
	SessionIsolationEnabled  bool    `json:"session_isolation_enabled"`
	RPMLimit                 int     `json:"rpm_limit"`
	ProviderCount            int64   `json:"provider_count"`
	ActiveProviderCount      int64   `json:"active_provider_count"`
	RateLimitedProviderCount int64   `json:"rate_limited_provider_count"`
}
