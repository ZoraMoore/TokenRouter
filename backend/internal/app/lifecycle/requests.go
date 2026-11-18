package lifecycle

import (
	"context"
	"net"
	"net/http"
	"sync"
)

// Requests 跟踪完整 handler 的返回，包括断开后仍按既有策略收集用量的请求。
// 这里包装 Handler，ResponseWriter 原样交给下游，下游仍能调用 Flush、Hijack 和 HTTP/2 相关接口。
type Requests struct {
	mu             sync.Mutex
	sealed         bool
	wg             sync.WaitGroup
	done           chan struct{}
	hijacked       map[net.Conn]struct{}
	closingHijacks bool
}

type requestConnectionKey struct{}

func TrackRequests(server *http.Server, manager *Manager) {
	r := &Requests{hijacked: make(map[net.Conn]struct{})}
	previousContext := server.ConnContext
	server.ConnContext = func(ctx context.Context, conn net.Conn) context.Context {
		if previousContext != nil {
			ctx = previousContext(ctx, conn)
		}
		return context.WithValue(ctx, requestConnectionKey{}, conn)
	}
	previousState := server.ConnState
	server.ConnState = func(conn net.Conn, state http.ConnState) {
		r.mu.Lock()
		closeHijack := state == http.StateHijacked && r.closingHijacks
		if state == http.StateHijacked {
			r.hijacked[conn] = struct{}{}
		}
		if state == http.StateClosed {
			delete(r.hijacked, conn)
		}
		r.mu.Unlock()
		// 关闭快照取得后才升级的连接，也进入断开清理。
		if closeHijack {
			_ = conn.Close()
		}
		if previousState != nil {
			previousState(conn, state)
		}
	}
	next := server.Handler
	if next == nil {
		next = http.DefaultServeMux
	}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		if r.sealed {
			r.mu.Unlock()
			http.Error(w, "server is shutting down", http.StatusServiceUnavailable)
			return
		}
		r.wg.Add(1)
		r.mu.Unlock()
		defer func() {
			if conn, ok := req.Context().Value(requestConnectionKey{}).(net.Conn); ok {
				r.mu.Lock()
				delete(r.hijacked, conn)
				r.mu.Unlock()
			}
			r.wg.Done()
		}()
		next.ServeHTTP(w, req)
	})
	// net/http 的 Shutdown 将 hijack 连接留给调用方关闭，此处触发这些连接的断开清理。
	manager.Register(Hook{Name: "HTTPHijackedConnections", StartOrder: 990, StopOrder: 10, Stop: func(context.Context) error {
		r.mu.Lock()
		r.closingHijacks = true
		connections := make([]net.Conn, 0, len(r.hijacked))
		for conn := range r.hijacked {
			connections = append(connections, conn)
		}
		r.mu.Unlock()
		for _, conn := range connections {
			_ = conn.Close()
		}
		return nil
	}})
	manager.Register(Hook{Name: "HTTPRequests", StartOrder: 985, StopOrder: 15, Stop: r.Wait})
}

// Wait 在停止监听后封闭请求入口，并等待 handler 自己完成清理。
func (r *Requests) Wait(ctx context.Context) error {
	r.mu.Lock()
	if r.done == nil {
		r.sealed = true
		r.done = make(chan struct{})
		go func() { r.wg.Wait(); close(r.done) }()
	}
	done := r.done
	r.mu.Unlock()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
