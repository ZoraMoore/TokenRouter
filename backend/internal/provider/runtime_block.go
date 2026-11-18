package provider

import (
	"sync"
	"sync/atomic"
	"time"
)

// RuntimeBlockFallback 和 RuntimeRetryWindow 分别控制内存停调与同提供商重试预算。
const (
	RuntimeBlockFallback = 2 * time.Minute
	RuntimeRetryWindow   = 2 * time.Minute
)

// RuntimeBlockState 统一持有停调、刷新失败与恢复代次，所有权不依附 HTTP 执行器。
type RuntimeBlockState struct {
	until                         sync.Map
	locks                         sync.Map
	generation                    sync.Map
	sequence                      atomic.Uint64
	retryStarted                  sync.Map
	refreshFailureBlocks          RefreshFailureBlocks
	refreshFailureClearGeneration sync.Map
	now                           func() time.Time
}

// NewRuntimeBlockState 使用装配传入的时钟创建运行状态。
func NewRuntimeBlockState(now func() time.Time) *RuntimeBlockState {
	if now == nil {
		now = time.Now
	}
	return &RuntimeBlockState{now: now}
}

// Block 只延长已有停调，重复声明仍推进代次，保护后续回滚所有权。
func (s *RuntimeBlockState) Block(id int64, until time.Time, reason string) {
	if s == nil {
		return
	}
	mu := s.lock(id)
	mu.Lock()
	defer mu.Unlock()
	_, _ = s.blockLocked(id, until, reason)
}

// BlockProviderScheduling 对支持的平台设置内存停调。
func (s *RuntimeBlockState) BlockProviderScheduling(value *Record, until time.Time, reason string) {
	if value == nil || (value.Platform != PlatformOpenAI && value.Platform != PlatformGrok) {
		return
	}
	s.Block(value.ID, until, reason)
}

// ResetRetry 在原成功回报位置清除同提供商恢复窗口。
func (s *RuntimeBlockState) ResetRetry(id int64) { s.retryStarted.Delete(id) }

// RetryWindowActive 按原首次观测时点创建同提供商恢复窗口。
func (s *RuntimeBlockState) RetryWindowActive(providerID int64) bool {
	if s == nil {
		return false
	}
	now := s.now()
	value, _ := s.retryStarted.LoadOrStore(providerID, now)
	startedAt, ok := value.(time.Time)
	if !ok {
		s.retryStarted.Store(providerID, now)
		startedAt = now
	}
	return now.Before(startedAt.Add(RuntimeRetryWindow))
}

// RetryDeadline 返回已有重试窗口的截止时间，窗口缺失时返回零值。
func (s *RuntimeBlockState) RetryDeadline(providerID int64) time.Time {
	if s == nil {
		return time.Time{}
	}
	value, ok := s.retryStarted.Load(providerID)
	if !ok {
		return time.Time{}
	}
	startedAt, ok := value.(time.Time)
	if !ok {
		return time.Time{}
	}
	return startedAt.Add(RuntimeRetryWindow)
}

func (s *RuntimeBlockState) lock(providerID int64) *sync.Mutex {
	actual, _ := s.locks.LoadOrStore(providerID, &sync.Mutex{})
	mu, ok := actual.(*sync.Mutex)
	if !ok {
		mu = &sync.Mutex{}
		s.locks.Store(providerID, mu)
	}
	return mu
}

func (s *RuntimeBlockState) blockLocked(providerID int64, until time.Time, _ string) (uint64, bool) {
	generation := s.sequence.Add(1)
	s.generation.Store(providerID, generation)
	now := s.now()
	blockUntil := until
	if blockUntil.IsZero() || !blockUntil.After(now) {
		blockUntil = now.Add(RuntimeBlockFallback)
	}

	for {
		current, loaded := s.until.Load(providerID)
		if !loaded {
			actual, stored := s.until.LoadOrStore(providerID, blockUntil)
			if !stored {
				return generation, true
			}
			current = actual
		}

		currentUntil, ok := current.(time.Time)
		if !ok || currentUntil.IsZero() {
			if s.until.CompareAndSwap(providerID, current, blockUntil) {
				return generation, true
			}
			continue
		}
		if !blockUntil.After(currentUntil) {
			return generation, false
		}
		if s.until.CompareAndSwap(providerID, current, blockUntil) {
			return generation, true
		}
	}
}

// ClearProviderSchedulingBlock 清除运行阻断和恢复预算，并递增清理代次。
func (s *RuntimeBlockState) ClearProviderSchedulingBlock(providerID int64) {
	if s == nil || providerID <= 0 {
		return
	}
	mu := s.lock(providerID)
	mu.Lock()
	defer mu.Unlock()
	s.clearLocked(providerID)
}

// ManagedRecoveryFence 取得管理员恢复使用的当前代次。
func (s *RuntimeBlockState) ManagedRecoveryFence(providerID int64) uint64 {
	if s == nil || providerID <= 0 {
		return 0
	}
	mu := s.lock(providerID)
	mu.Lock()
	defer mu.Unlock()
	value, _ := s.generation.Load(providerID)
	generation, _ := value.(uint64)
	return generation
}

// ClearProviderSchedulingBlockIfFence 仅清理仍属于本次恢复观察的状态。
func (s *RuntimeBlockState) ClearProviderSchedulingBlockIfFence(providerID int64, expected uint64) bool {
	if s == nil || providerID <= 0 {
		return false
	}
	mu := s.lock(providerID)
	mu.Lock()
	defer mu.Unlock()
	value, _ := s.generation.Load(providerID)
	generation, _ := value.(uint64)
	if generation != expected {
		return false
	}
	s.clearLocked(providerID)
	return true
}

func (s *RuntimeBlockState) clearLocked(providerID int64) {
	s.refreshFailureBlocks.Clear(providerID)
	s.until.Delete(providerID)
	s.retryStarted.Delete(providerID)
	generation := s.sequence.Add(1)
	s.generation.Store(providerID, generation)
	s.refreshFailureClearGeneration.Store(providerID, generation)
}

// Blocked 先核对刷新凭据身份，再判断尚未到期的本地停调。
func (s *RuntimeBlockState) Blocked(providerID int64, identity func() string) bool {
	if s == nil {
		return false
	}
	mu := s.lock(providerID)
	mu.Lock()
	defer mu.Unlock()
	if s.refreshFailureBlocks.Blocked(providerID, s.now(), identity) {
		return true
	}
	value, ok := s.until.Load(providerID)
	if !ok {
		return false
	}
	cooldownUntil, ok := value.(time.Time)
	if !ok || cooldownUntil.IsZero() {
		s.until.Delete(providerID)
		s.generation.Store(providerID, s.sequence.Add(1))
		return false
	}
	if s.now().Before(cooldownUntil) {
		return true
	}
	s.until.Delete(providerID)
	s.generation.Store(providerID, s.sequence.Add(1))
	return false
}

// BlockRollback 返回本次暂定状态的撤销函数，执行时检查状态仍属于本次操作。
func (s *RuntimeBlockState) BlockRollback(providerID int64, until time.Time, reason string) func() {
	if s == nil {
		return func() {}
	}
	mu := s.lock(providerID)
	mu.Lock()
	before, hadBefore := s.until.Load(providerID)
	installedGeneration, changed := s.blockLocked(providerID, until, reason)
	installed, installedOK := s.until.Load(providerID)
	installedUntil, isTime := installed.(time.Time)
	mu.Unlock()
	if !changed || !installedOK || !isTime {
		return func() {}
	}
	if hadBefore {
		if beforeUntil, ok := before.(time.Time); ok && beforeUntil.Equal(installedUntil) {
			return func() {}
		}
	}
	return func() {
		mu.Lock()
		defer mu.Unlock()
		generation, ok := s.generation.Load(providerID)
		if !ok || generation != installedGeneration {
			return
		}
		current, ok := s.until.Load(providerID)
		currentUntil, isTime := current.(time.Time)
		if !ok || !isTime || !currentUntil.Equal(installedUntil) {
			return
		}
		if hadBefore {
			s.until.Store(providerID, before)
			s.generation.Store(providerID, s.sequence.Add(1))
			return
		}
		s.until.Delete(providerID)
		s.generation.Store(providerID, s.sequence.Add(1))
	}
}

// PrepareRefreshFailure 在存储前记录清理代次，阻止迟到通知恢复已清除状态。
func (s *RuntimeBlockState) PrepareRefreshFailure(id int64) func(RefreshFailureNotice) {
	if s == nil {
		return func(RefreshFailureNotice) {}
	}
	mu := s.lock(id)
	mu.Lock()
	generation, _ := s.refreshFailureClearGeneration.Load(id)
	mu.Unlock()
	return func(notice RefreshFailureNotice) {
		if notice.ProviderID != id || notice.Identity == "" || (notice.Platform != PlatformOpenAI && notice.Platform != PlatformGrok) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		current, _ := s.refreshFailureClearGeneration.Load(id)
		if current != generation {
			return
		}
		now := s.now()
		until := notice.Until
		if !until.After(now) {
			until = now.Add(RuntimeBlockFallback)
		}
		s.refreshFailureBlocks.Block(id, notice.Identity, until, now)
	}
}
