package provider

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/TokenFlux/TokenRouter/internal/egress"
	"github.com/TokenFlux/TokenRouter/internal/provider"
)

// TestRun 保存单次平台测试的输出和执行状态。
type TestRun struct {
	Context context.Context
	Cancel  context.CancelFunc
	Headers http.Header
	// 这些状态供本次同步测试流程使用。
	RequestedProtocol  provider.TextProtocol
	TaskRecoveryTried  bool
	ModelFallbackTried bool
	Automatic          bool
	sink               provider.TestEventSink
	mu                 sync.Mutex
	err                error
	automaticRoute     egress.TLSFingerprintRouterMatchResult
	hasAutomaticRoute  bool
	suppressCompletion bool
}

func NewTestRun(ctx context.Context, headers http.Header, sink provider.TestEventSink) *TestRun {
	ctx, cancel := context.WithCancel(ctx)
	if headers == nil {
		headers = make(http.Header)
	}
	return &TestRun{Context: ctx, Cancel: cancel, Headers: headers, sink: sink}
}

func (r *TestRun) GetHeader(name string) string { return r.Headers.Get(name) }

func (r *TestRun) SetAutomaticRoute(value egress.TLSFingerprintRouterMatchResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.automaticRoute = value
	r.hasAutomaticRoute = true
}

func (r *TestRun) AutomaticRoute() (egress.TLSFingerprintRouterMatchResult, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.automaticRoute, r.hasAutomaticRoute
}

func (r *TestRun) SetSuppressCompletion(value bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.suppressCompletion = value
}

func (r *TestRun) SuppressCompletion() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.suppressCompletion
}

// output 在首次输出失败时记录错误并取消执行，后续写入直接返回。
func (r *TestRun) output(write func() error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err == nil {
		if err := write(); err != nil {
			r.err = err
			r.Cancel()
		}
	}
	return r.err
}

func (r *TestRun) Begin(commit bool) {
	_ = r.output(func() error { return r.sink.Begin(r.Context, commit) })
}

func (r *TestRun) Emit(event provider.TestEvent) error {
	return r.output(func() error { return r.sink.Emit(r.Context, event) })
}

func (r *TestRun) Result(err error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil && !errors.Is(err, r.err) {
		return errors.Join(err, r.err)
	}
	return err
}
