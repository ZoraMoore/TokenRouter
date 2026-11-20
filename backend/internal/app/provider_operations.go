package app

import (
	"context"
	"database/sql"
	"os"
	"strings"

	"github.com/redis/go-redis/v9"

	"github.com/TokenFlux/TokenRouter/internal/app/lifecycle"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	providerhttp "github.com/TokenFlux/TokenRouter/internal/provider/httpapi"
	"github.com/TokenFlux/TokenRouter/internal/provider/operation"
	"github.com/TokenFlux/TokenRouter/internal/provider/operation/traffic"
	providerpostgres "github.com/TokenFlux/TokenRouter/internal/provider/postgres"
	provideradapter "github.com/TokenFlux/TokenRouter/internal/provider/provider"
	routingpostgres "github.com/TokenFlux/TokenRouter/internal/routing/postgres"
	"github.com/TokenFlux/TokenRouter/internal/scheduler"
	schedulerredis "github.com/TokenFlux/TokenRouter/internal/scheduler/rediscache"
)

// provideProviderOperations 绑定管理员任务、应用生命周期和跨实例的数据库锁。
// @project-doc docs/operations/provider_maintenance.md#provider_operations
func provideProviderOperations(db *sql.DB, redisClient *redis.Client, store *providerpostgres.ProviderStore, groups *routingpostgres.GroupStore, admin *provider.Admin, concurrency scheduler.ConcurrencyCache, quota *provider.QuotaSettingsCache, manager *lifecycle.Manager) *providerhttp.OperationHandler {
	taskStore := providerpostgres.NewOperationStore(db)
	key := strings.TrimSpace(os.Getenv("REVIVE_API_KEY"))
	relogin := provider.NewReloginService(admin, provideradapter.NewReviveClient(key))
	var reloginRunner operation.Runner
	if key != "" {
		reloginRunner = relogin.Run
	}
	push := traffic.New(store, groups, providerpostgres.NewTrafficStore(db), schedulerredis.NewStickyCache(redisClient), concurrency, func(record *provider.Record) *provider.ModelRulesSnapshot {
		return provider.PrepareModelRules(record, provideradapter.ModelDefaults(), provideradapter.ModelRules(record))
	}, quota.GetOpenAIQuotaAutoPauseSettings)
	worker := operation.NewWorker(taskStore, databaseAdvisoryLease(db), reloginRunner, push.Run)
	manager.Register(lifecycle.Hook{Name: "ProviderOperations", StartOrder: 160, StopOrder: 20, Start: worker.Start, Stop: worker.StopContext})
	service := operation.NewService(taskStore, relogin.Preview, push.Validate, func(ctx context.Context, id int64) (string, error) {
		record, err := store.GetByID(ctx, id)
		if err != nil {
			return "", err
		}
		return record.Name, nil
	})
	return providerhttp.NewOperationHandler(service)
}
