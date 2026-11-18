package querycache

import (
	"context"
	"fmt"
)

// contextLoad 将共享查询的存活时间与当前等待者数量关联。
type contextLoad struct {
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
	waiters  int
	finished bool
	entry    Entry
	hit      bool
	err      error
}

// GetOrLoadContext 允许等待者独立取消，最后一个等待者退出时取消共享查询。
// load 使用传入的共享上下文，查询结果按调用方复制。
func (c *Cache) GetOrLoadContext(ctx context.Context, key string, load func(context.Context) (any, error)) (Entry, bool, error) {
	if err := ctx.Err(); err != nil {
		return Entry{}, false, err
	}
	if load == nil {
		return Entry{}, false, nil
	}
	if entry, ok := c.Get(key); ok {
		return entry, true, nil
	}
	if c == nil || key == "" {
		payload, err := load(ctx)
		if err != nil {
			return Entry{}, false, err
		}
		if err = ctx.Err(); err != nil {
			return Entry{}, false, err
		}
		if c != nil {
			return c.Set(key, payload), false, nil
		}
		return Entry{Payload: Clone(payload)}, false, nil
	}
	c.loadMu.Lock()
	if c.loads == nil {
		c.loads = make(map[string]*contextLoad)
	}
	flight := c.loads[key]
	if flight == nil {
		shared, cancel := context.WithCancel(context.WithoutCancel(ctx))
		flight = &contextLoad{ctx: shared, cancel: cancel, done: make(chan struct{})}
		c.loads[key] = flight
		go c.runContextLoad(key, flight, load)
	}
	flight.waiters++
	c.loadMu.Unlock()
	defer c.leaveContextLoad(key, flight)
	select {
	case <-ctx.Done():
		return Entry{}, false, ctx.Err()
	case <-flight.done:
		if err := ctx.Err(); err != nil {
			return Entry{}, false, err
		}
		return Clone(flight.entry), flight.hit, flight.err
	}
}

// leaveContextLoad 先移除无人等待的查询，让后续请求能立即建立自己的查询。
func (c *Cache) leaveContextLoad(key string, flight *contextLoad) {
	c.loadMu.Lock()
	defer c.loadMu.Unlock()
	flight.waiters--
	if flight.waiters == 0 && !flight.finished {
		if c.loads[key] == flight {
			delete(c.loads, key)
		}
		flight.cancel()
	}
}

// runContextLoad 在独立协程中加载，取消后的迟到结果不进入缓存。
func (c *Cache) runContextLoad(key string, flight *contextLoad, load func(context.Context) (any, error)) {
	var entry Entry
	var payload any
	var hit bool
	var err error
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("缓存查询加载异常: %v", recovered)
		}
		c.loadMu.Lock()
		defer c.loadMu.Unlock()
		if flight.ctx.Err() != nil {
			err = flight.ctx.Err()
		}
		if err == nil && !hit && flight.waiters > 0 {
			entry = c.Set(key, payload)
		}
		flight.entry, flight.hit, flight.err = entry, hit, err
		flight.finished = true
		if c.loads[key] == flight {
			delete(c.loads, key)
		}
		flight.cancel()
		close(flight.done)
	}()
	if entry, hit = c.Get(key); hit {
		return
	}
	payload, err = load(flight.ctx)
}
