package lifecycle

import (
	"sync"
	"time"
)

// Restarter 请求主循环关闭，入口负责进程退出，supervisor 负责重启。
type Restarter struct {
	mu       sync.Mutex
	platform string
	request  func()
	timer    *time.Timer
	closed   bool
	wg       sync.WaitGroup
}

func NewRestarter(platform string, request func()) *Restarter {
	return &Restarter{platform: platform, request: request}
}

// RequestRestart 在 Linux 上安排 600ms 后请求关闭，为 HTTP 响应发送留出时间，其他平台直接返回。
func (r *Restarter) RequestRestart() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.platform != "linux" || r.closed || r.timer != nil {
		return nil
	}
	r.wg.Add(1)
	r.timer = time.AfterFunc(600*time.Millisecond, func() {
		defer r.wg.Done()
		r.request()
	})
	return nil
}

// Close 取消尚未发出的重启请求，并等待已经触发的回调返回。
func (r *Restarter) Close() {
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		if r.timer != nil && r.timer.Stop() {
			r.wg.Done()
		}
	}
	r.mu.Unlock()
	r.wg.Wait()
}
