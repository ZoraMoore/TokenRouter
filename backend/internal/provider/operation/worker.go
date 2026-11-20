package operation

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"
)

const (
	pollInterval = 3 * time.Second
	runTimeout   = 4 * time.Minute
)

// WorkerStore 每次领取任务后保留其检查点，进程重启后仍可领取 running 任务。
type WorkerStore interface {
	Claim(context.Context, Kind) (Job, error)
	SaveCheckpoint(context.Context, string, string, json.RawMessage) error
	Finish(context.Context, string, State, string, Result, time.Duration) error
}

// Worker 在同一个数据库锁内执行同类任务，任一节点都可以接手续跑。
type Worker struct {
	store   WorkerStore
	acquire func(context.Context, string) (func(), bool)
	runners map[Kind]Runner
	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
}

// NewWorker 绑定两个任务执行器和数据库的会话锁。
func NewWorker(store WorkerStore, acquire func(context.Context, string) (func(), bool), relogin, traffic Runner) *Worker {
	return &Worker{store: store, acquire: acquire, runners: map[Kind]Runner{
		Relogin:     relogin,
		TrafficPush: traffic,
	}}
}

// Start 给两种任务分别启动一个后台轮询。
func (w *Worker) Start(context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.done != nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.done = make(chan struct{})
	go func() {
		defer close(w.done)
		var group sync.WaitGroup
		for kind, run := range w.runners {
			if run != nil {
				group.Go(func() { w.loop(ctx, kind, run) })
			}
		}
		group.Wait()
	}()
	return nil
}

// StopContext 取消轮询和在途操作，检查点由下一次启动继续。
func (w *Worker) StopContext(ctx context.Context) error {
	w.mu.Lock()
	cancel, done := w.cancel, w.done
	w.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *Worker) loop(ctx context.Context, kind Kind, run Runner) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		if w.process(ctx, kind, run) {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *Worker) process(ctx context.Context, kind Kind, run Runner) bool {
	claimCtx, cancelClaim := context.WithTimeout(ctx, 5*time.Second)
	defer cancelClaim()
	unlock, acquired := w.acquire(claimCtx, "tokenrouter:provider-operations:"+string(kind))
	if !acquired {
		return false
	}
	defer unlock()
	job, err := w.store.Claim(claimCtx, kind)
	if err != nil {
		if !errors.Is(err, ErrNotFound) && ctx.Err() == nil {
			slog.Warn("领取提供商任务失败", "kind", kind, "error", err)
		}
		return false
	}

	runCtx, cancelRun := context.WithTimeout(ctx, runTimeout)
	defer cancelRun()
	checkpoint := func(stage string, data any) error {
		raw, err := json.Marshal(data)
		if err != nil {
			return err
		}
		if err := w.store.SaveCheckpoint(runCtx, job.ID, stage, raw); err != nil {
			slog.Warn("保存提供商任务阶段失败", "id", job.ID, "stage", stage)
			return RetryLater("暂时无法保存执行进度，任务将在下一轮继续")
		}
		return nil
	}
	result, err := run(runCtx, job, checkpoint)
	if ctx.Err() != nil {
		return false
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) && err != nil {
		err = RetryLater("任务将在下一轮继续")
	}
	state, message, delay := outcome(err)
	if state == Failed {
		// 完整上游响应可能含凭据，任务日志记录对象和执行阶段供定位。
		slog.Warn("提供商任务失败", "id", job.ID, "kind", job.Kind, "provider_id", job.ProviderID, "stage_at_claim", job.Stage)
	}
	finishCtx, cancelFinish := context.WithTimeout(ctx, 5*time.Second)
	defer cancelFinish()
	if err := w.store.Finish(finishCtx, job.ID, state, message, result, delay); err != nil {
		slog.Warn("保存提供商任务结果失败", "id", job.ID, "error", err)
		return false
	}
	return true
}

func outcome(err error) (State, string, time.Duration) {
	if err == nil {
		return Succeeded, "", 0
	}
	var known *publicError
	if errors.As(err, &known) {
		if known.retry > 0 {
			return Queued, known.message, known.retry
		}
		return RequiresAction, known.message, 0
	}
	return Failed, "操作失败，请查看服务端日志", 0
}
