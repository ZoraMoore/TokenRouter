package completion

import (
	"context"
	"time"
)

// SubmissionEvent 描述提交降级或 panic，通过 app 提供的日志回调记录。
type SubmissionEvent struct {
	Mandatory bool
	Panic     any
}

// SubmissionOptions 配置工作池停止、请求取消和 panic 时的处理方式。
// 异步任务交给传入的 WorkerPool 执行。
type SubmissionOptions struct {
	FallbackWhenStopped  bool
	PreserveSourceValues bool
	RecoverPanic         bool
	Observe              func(SubmissionEvent)
}

// SubmitTask 复制本次请求的关联数据后提交异步任务。
// mandatory 指定的任务可转为同步执行，普通 drop/sample 任务继续按配置丢弃。
func SubmitTask(pool *UsageRecordWorkerPool, source context.Context, task UsageRecordTask, mandatory bool, options SubmissionOptions) {
	if task == nil {
		return
	}
	task = WrapTaskContext(source, task)
	if pool != nil {
		mode := pool.Submit(task)
		fallback := mandatory && mode.Dropped() || options.FallbackWhenStopped && mode == UsageRecordSubmitModeDroppedStopped
		if !fallback {
			return
		}
		if options.Observe != nil {
			options.Observe(SubmissionEvent{Mandatory: mandatory})
		}
	}
	base := context.Background()
	if options.PreserveSourceValues && source != nil {
		base = context.WithoutCancel(source)
	}
	ctx, cancel := context.WithTimeout(base, 10*time.Second)
	defer cancel()
	if options.RecoverPanic {
		defer func() {
			if value := recover(); value != nil && options.Observe != nil {
				options.Observe(SubmissionEvent{Mandatory: mandatory, Panic: value})
			}
		}()
	}
	task(ctx)
}
