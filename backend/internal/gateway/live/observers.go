package live

import (
	"context"
	"sync"
)

// ObserverState 引用进程中的观察者登记状态，四个字段需要来自同一个管理实例。
type ObserverState struct {
	Mutex   *sync.Mutex
	Stopped *bool
	Cancels *map[string]context.CancelFunc
	Wait    *sync.WaitGroup
}

// Begin 在同一锁内登记观察者和等待计数，停止后返回 false。
func (s ObserverState) Begin(owner string) (context.Context, func(), bool) {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()
	if *s.Stopped {
		return nil, nil, false
	}
	if *s.Cancels == nil {
		*s.Cancels = make(map[string]context.CancelFunc)
	}
	ctx, cancel := context.WithCancel(context.Background())
	(*s.Cancels)[owner] = cancel
	s.Wait.Add(1)
	return ctx, func() {
		cancel()
		s.Mutex.Lock()
		delete(*s.Cancels, owner)
		s.Mutex.Unlock()
		s.Wait.Done()
	}, true
}

// Stop 取消本地观察循环，并在应用剩余预算内等待结束。远端会话按自身生命周期结束。
func (s ObserverState) Stop(ctx context.Context) error {
	s.Mutex.Lock()
	*s.Stopped = true
	for _, cancel := range *s.Cancels {
		cancel()
	}
	s.Mutex.Unlock()
	done := make(chan struct{})
	go func() { s.Wait.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
