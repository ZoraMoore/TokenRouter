package provider

import (
	"context"
	"errors"
	"sync"
	"time"
)

// RefreshRepository 只读取刷新时需要的持久化提供商值；条件写入能力单独检测。
type RefreshRepository interface {
	GetByID(context.Context, int64) (*Record, error)
}

// OAuthRefreshExecutor 是已选平台的凭据交换接口。
type OAuthRefreshExecutor interface {
	CanRefresh(*Record) bool
	NeedsRefresh(*Record, time.Duration) bool
	Refresh(context.Context, *Record) (map[string]any, error)
	CacheKey(*Record) string
}

// RefreshCache 提供刷新缓存和锁操作，由 app 绑定 Redis 实现。
type RefreshCache interface {
	AcquireRefreshLock(context.Context, string, time.Duration) (bool, error)
	ReleaseRefreshLock(context.Context, string) error
	DeleteAccessToken(context.Context, string) error
}

// RefreshPlatformPolicy 判断刷新资格和错误类型，回调接收提供商数据副本。
type RefreshPlatformPolicy struct {
	Eligibility         func(*Record) error
	MissingRefreshToken func() error
	SnapshotError       func(error, *Record) error
	ConfigurationError  func(error) error
	ContainmentError    func(error) error
}

// RefreshOptions 只包含时间、锁预算、观察接口与已选平台的错误策略。
type RefreshOptions struct {
	LockTTL  time.Duration
	Now      func() time.Time
	Warn     func(string, ...any)
	Info     func(string, ...any)
	Error    func(string, ...any)
	Platform RefreshPlatformPolicy
}

// OAuthRefreshResult 保持每次交换自己的值，不对外序列化凭据。
type OAuthRefreshResult struct {
	Refreshed      bool
	NewCredentials map[string]any `json:"-"`
	Provider       *Record        `json:"-"`
	LockHeld       bool
}

// OAuthRefreshAPI 持有唯一进程锁表；构造本身不启动任务。
type OAuthRefreshAPI struct {
	activity     refreshActivity
	providerRepo RefreshRepository
	tokenCache   RefreshCache
	lockTTL      time.Duration
	localLocks   sync.Map
	options      RefreshOptions
}

func NewOAuthRefreshAPI(repo RefreshRepository, cache RefreshCache, options RefreshOptions) *OAuthRefreshAPI {
	if options.LockTTL <= 0 {
		options.LockTTL = defaultRefreshLockTTL
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	noop := func(string, ...any) {}
	if options.Warn == nil {
		options.Warn = noop
	}
	if options.Info == nil {
		options.Info = noop
	}
	if options.Error == nil {
		options.Error = noop
	}
	if options.Platform.Eligibility == nil {
		options.Platform.Eligibility = func(*Record) error { return errors.New("oauth refresh request eligibility policy is not configured") }
	}
	if options.Platform.MissingRefreshToken == nil {
		options.Platform.MissingRefreshToken = func() error { return errors.New("refresh token is missing") }
	}
	if options.Platform.SnapshotError == nil {
		options.Platform.SnapshotError = func(err error, _ *Record) error { return err }
	}
	if options.Platform.ConfigurationError == nil {
		options.Platform.ConfigurationError = func(err error) error { return err }
	}
	if options.Platform.ContainmentError == nil {
		options.Platform.ContainmentError = func(err error) error { return err }
	}
	return &OAuthRefreshAPI{providerRepo: repo, tokenCache: cache, lockTTL: options.LockTTL, options: options}
}

// LockLease 给周期执行器提供现有锁租约，便于把尝试预算约束在租约内。
func (api *OAuthRefreshAPI) LockLease() (time.Duration, bool) {
	if api == nil {
		return 0, false
	}
	return api.lockTTL, api.tokenCache != nil
}
