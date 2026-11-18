package app

import (
	"context"
	"database/sql"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/TokenFlux/TokenRouter/internal/app/lifecycle"
	"github.com/TokenFlux/TokenRouter/internal/config"
	"github.com/TokenFlux/TokenRouter/internal/identity"
	opsredis "github.com/TokenFlux/TokenRouter/internal/ops/rediscache"
	usagequery "github.com/TokenFlux/TokenRouter/internal/usage/postgres/query"
)

// provideAPIActivity 将请求内计数与周期发布分开，HTTP 处理过程使用本地内存。
func provideAPIActivity(cfg *config.Config, db *sql.DB, r *redis.Client, manager *lifecycle.Manager) *identity.APIActivityTracker {
	tracker := identity.NewAPIActivityTracker(identity.APIActivityOptions{
		NodeID: cfg.Ops.NodeID, NodeIDs: cfg.Ops.ClusterNodeIDs, Cache: opsredis.NewRuntime(r),
		Seed: func(ctx context.Context, since time.Time) (map[int64]time.Time, error) {
			return usagequery.GetRecentUserActivity(ctx, db, since)
		},
	})
	manager.Register(lifecycle.Hook{Name: "APIUserActivity", StartOrder: 940, StopOrder: 95, Start: tracker.Start, Stop: tracker.Stop})
	return tracker
}
