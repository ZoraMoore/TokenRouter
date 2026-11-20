package operation

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

const (
	// Relogin 和 TrafficPush 的值也用于数据库和管理 API。
	Relogin     Kind = "relogin"
	TrafficPush Kind = "traffic_push"

	Queued         State = "queued"
	Running        State = "running"
	Succeeded      State = "succeeded"
	Failed         State = "failed"
	RequiresAction State = "needs_action"
)

var (
	ErrNotFound = errors.New("provider operation not found")
	ErrBusy     = errors.New("provider already has an active operation")
)

// Kind 区分允许同时运行的两类工作。
type Kind string

// State 是管理页面看到的任务状态。
type State string

// Job 的 Data 仅用于恢复执行，包含内部操作标识和执行检查点。
type Job struct {
	ID           string          `json:"id"`
	ProviderID   int64           `json:"provider_id"`
	ProviderName string          `json:"provider_name"`
	GroupID      int64           `json:"group_id,omitempty"`
	GroupIDs     []int64         `json:"group_ids,omitempty"`
	Kind         Kind            `json:"kind"`
	State        State           `json:"state"`
	Stage        string          `json:"stage,omitempty"`
	Message      string          `json:"message,omitempty"`
	Data         json.RawMessage `json:"-"`
	Result       Result          `json:"result"`
}

// Result 分开记录替换后的提供商和推流的实测数据。
type Result struct {
	ProviderID          int64 `json:"provider_id,omitempty"`
	MigratedSessions    int   `json:"migrated_sessions"`
	SuccessfulRequests  int   `json:"successful_requests"`
	PeakConcurrency     int   `json:"peak_concurrency"`
	TargetConcurrency   int   `json:"target_concurrency,omitempty"`
	UnconfirmedSessions *int  `json:"unconfirmed_sessions,omitempty"`
}

// PreviewItem 记录所选账号是否满足重登条件。
type PreviewItem struct {
	ProviderID int64  `json:"provider_id"`
	Name       string `json:"name"`
	Eligible   bool   `json:"eligible"`
	Reason     string `json:"reason,omitempty"`
}

// Checkpoint 在调用外部写接口或迁移会话前保存继续执行所需的数据。
type Checkpoint func(stage string, data any) error

// Runner 执行一个任务，或返回下一轮该何时继续。
type Runner func(context.Context, Job, Checkpoint) (Result, error)

type publicError struct {
	message string
	retry   time.Duration
}

// TrafficGroups 返回任务的来源分组，兼容只保存一个分组的任务。
func (j Job) TrafficGroups() []int64 {
	if len(j.GroupIDs) != 0 {
		return j.GroupIDs
	}
	if j.GroupID > 0 {
		return []int64{j.GroupID}
	}
	return nil
}

// NeedsAction 结束本次操作并给管理员显示可处理的原因。
func NeedsAction(message string) error {
	return &publicError{message: message}
}

// RetryLater 在后台查询下一次任务进度。
func RetryLater(message string) error {
	return RetryAfter(message, 4*time.Second)
}

// RetryAfter 给外部限流和异步操作保留要求的等待时间。
func RetryAfter(message string, delay time.Duration) error {
	return &publicError{message: message, retry: max(time.Second, delay)}
}

// PublicMessage 只取可向管理员展示的错误文本。
func PublicMessage(err error) (string, bool) {
	if known, ok := errors.AsType[*publicError](err); ok {
		return known.message, true
	}
	return "", false
}

func (e *publicError) Error() string { return e.message }
