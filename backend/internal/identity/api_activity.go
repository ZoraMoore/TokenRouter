package identity

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

const (
	apiActivityWindow    = 5 * time.Minute
	apiActivityInterval  = 15 * time.Second
	apiActivityFreshness = time.Minute
	apiActivityRetention = 10 * time.Minute
)

type APIActivityCache interface {
	Get(context.Context, string) (string, error)
	Put(context.Context, string, string, time.Duration) error
}

type APIActivityOptions struct {
	NodeID  string
	NodeIDs []string
	Cache   APIActivityCache
	Now     func() time.Time
	Seed    func(context.Context, time.Time) (map[int64]time.Time, error)
}

// APIActivityStats 统计实际调用身份，多个 Key、客户端和节点按用户去重。
type APIActivityStats struct {
	ActiveUsers    int       `json:"active_users"`
	WindowSeconds  int       `json:"window_seconds"`
	ReportingNodes int       `json:"reporting_nodes"`
	TotalNodes     int       `json:"total_nodes"`
	Complete       bool      `json:"complete"`
	GeneratedAt    time.Time `json:"generated_at"`
}

type apiActivityUser struct {
	LastActivityAt time.Time `json:"last_activity_at"`
	Active         int       `json:"active"`
}

type apiActivitySnapshot struct {
	NodeID      string                    `json:"node_id"`
	SampledAt   time.Time                 `json:"sampled_at"`
	Initialized bool                      `json:"initialized"`
	Users       map[int64]apiActivityUser `json:"users"`
}

// APIActivityTracker 在请求内更新本地计数，由周期任务批量发布到共享缓存。
type APIActivityTracker struct {
	mu          sync.Mutex
	publishMu   sync.Mutex
	options     APIActivityOptions
	users       map[int64]apiActivityUser
	initialized bool
	started     bool
	stopped     bool
	stop        chan struct{}
	done        chan struct{}
}

func NewAPIActivityTracker(options APIActivityOptions) *APIActivityTracker {
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.NodeID == "" {
		options.NodeID = "local"
	}
	seen := make(map[string]bool)
	ids := make([]string, 0, len(options.NodeIDs)+1)
	for _, id := range append(options.NodeIDs, options.NodeID) {
		id = strings.TrimSpace(id)
		if id != "" && !seen[id] && len(ids) < 32 {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	options.NodeIDs = ids
	return &APIActivityTracker{options: options, users: make(map[int64]apiActivityUser)}
}

// Track 从已认证的调用身份登记请求，释放函数可重复调用。
func (t *APIActivityTracker) Track(userID int64) func() {
	if t == nil || userID <= 0 {
		return func() {}
	}
	t.mu.Lock()
	if t.stopped {
		t.mu.Unlock()
		return func() {}
	}
	user := t.users[userID]
	user.LastActivityAt = t.options.Now().UTC()
	user.Active++
	t.users[userID] = user
	t.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			t.mu.Lock()
			defer t.mu.Unlock()
			user := t.users[userID]
			if user.Active > 0 {
				user.Active--
			}
			user.LastActivityAt = t.options.Now().UTC()
			t.users[userID] = user
		})
	}
}

func (t *APIActivityTracker) Start(context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.started || t.stopped {
		return nil
	}
	t.started = true
	t.stop = make(chan struct{})
	t.done = make(chan struct{})
	go t.run()
	return nil
}

func (t *APIActivityTracker) run() {
	defer close(t.done)
	ticker := time.NewTicker(apiActivityInterval)
	defer ticker.Stop()
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = t.Publish(ctx)
		cancel()
		select {
		case <-ticker.C:
		case <-t.stop:
			return
		}
	}
}

func (t *APIActivityTracker) Stop(ctx context.Context) error {
	t.mu.Lock()
	if !t.stopped {
		t.stopped = true
		if t.started {
			close(t.stop)
		}
	}
	started, done := t.started, t.done
	t.mu.Unlock()
	if started {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	t.mu.Lock()
	for id, user := range t.users {
		user.Active = 0
		t.users[id] = user
	}
	t.mu.Unlock()
	return t.Publish(ctx)
}

func apiActivityKey(nodeID string) string {
	return "identity:api_activity:v1:" + nodeID
}

func (t *APIActivityTracker) snapshot(now time.Time) apiActivitySnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := apiActivitySnapshot{NodeID: t.options.NodeID, SampledAt: now, Initialized: t.initialized, Users: make(map[int64]apiActivityUser)}
	for id, user := range t.users {
		if user.Active == 0 && user.LastActivityAt.Before(now.Add(-apiActivityWindow)) {
			delete(t.users, id)
			continue
		}
		result.Users[id] = user
	}
	return result
}

// Publish 合并重启前的活动时间，历史在途计数由新进程重新建立。
func (t *APIActivityTracker) Publish(ctx context.Context) error {
	t.publishMu.Lock()
	defer t.publishMu.Unlock()
	now := t.options.Now().UTC()
	t.mu.Lock()
	initialized := t.initialized
	t.mu.Unlock()
	if !initialized {
		history := make(map[int64]time.Time)
		if t.options.Cache != nil {
			if raw, err := t.options.Cache.Get(ctx, apiActivityKey(t.options.NodeID)); err == nil && len(raw) <= 2*1024*1024 {
				var prior apiActivitySnapshot
				if json.Unmarshal([]byte(raw), &prior) == nil && prior.NodeID == t.options.NodeID {
					for id, user := range prior.Users {
						history[id] = user.LastActivityAt
					}
				}
			}
		}
		if t.options.Seed != nil {
			seed, err := t.options.Seed(ctx, now.Add(-apiActivityWindow))
			if err != nil {
				return err
			}
			for id, at := range seed {
				if at.After(history[id]) {
					history[id] = at
				}
			}
		}
		t.mu.Lock()
		for id, at := range history {
			if id <= 0 || at.Before(now.Add(-apiActivityWindow)) || at.After(now.Add(5*time.Second)) {
				continue
			}
			user := t.users[id]
			if at.After(user.LastActivityAt) {
				user.LastActivityAt = at
				t.users[id] = user
			}
		}
		t.initialized = true
		t.mu.Unlock()
	}
	if t.options.Cache == nil {
		return nil
	}
	raw, err := json.Marshal(t.snapshot(now))
	if err != nil {
		return err
	}
	return t.options.Cache.Put(ctx, apiActivityKey(t.options.NodeID), string(raw), apiActivityRetention)
}

// Count 保留近五分钟活动，失联节点的旧在途计数按采样新鲜度失效。
func (t *APIActivityTracker) Count(ctx context.Context) (*APIActivityStats, error) {
	now := t.options.Now().UTC()
	result := &APIActivityStats{WindowSeconds: int(apiActivityWindow.Seconds()), TotalNodes: len(t.options.NodeIDs), GeneratedAt: now}
	users := make(map[int64]bool)
	for _, id := range t.options.NodeIDs {
		var snapshot apiActivitySnapshot
		if id == t.options.NodeID {
			snapshot = t.snapshot(now)
		} else if t.options.Cache != nil {
			raw, err := t.options.Cache.Get(ctx, apiActivityKey(id))
			if err != nil || len(raw) > 2*1024*1024 || json.Unmarshal([]byte(raw), &snapshot) != nil || snapshot.NodeID != id {
				continue
			}
		}
		age := now.Sub(snapshot.SampledAt)
		fresh := !snapshot.SampledAt.IsZero() && age >= -5*time.Second && age <= apiActivityFreshness
		if fresh && snapshot.Initialized {
			result.ReportingNodes++
		}
		for userID, user := range snapshot.Users {
			activeAge := now.Sub(user.LastActivityAt)
			recent := activeAge >= -5*time.Second && activeAge <= apiActivityWindow
			if userID > 0 && (recent || fresh && user.Active > 0) {
				users[userID] = true
			}
		}
	}
	result.ActiveUsers = len(users)
	result.Complete = result.ReportingNodes == result.TotalNodes
	return result, nil
}
