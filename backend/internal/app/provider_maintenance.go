package app

import (
	"context"
	"database/sql"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/app/lifecycle"
	"github.com/TokenFlux/TokenRouter/internal/config"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	providerhttp "github.com/TokenFlux/TokenRouter/internal/provider/httpapi"
	providerpostgres "github.com/TokenFlux/TokenRouter/internal/provider/postgres"
	provideradapter "github.com/TokenFlux/TokenRouter/internal/provider/provider"
	routingpostgres "github.com/TokenFlux/TokenRouter/internal/routing/postgres"
	"github.com/TokenFlux/TokenRouter/internal/scheduler"
)

// provideProviderMaintenance 将定时任务绑定到现有测试、调度和重登服务。
func provideProviderMaintenance(
	db *sql.DB,
	providers *providerpostgres.ProviderStore,
	groups *routingpostgres.GroupStore,
	tests *provider.TestService,
	concurrency *scheduler.ConcurrencyService,
	cfg *config.Config,
	manager *lifecycle.Manager,
) *providerhttp.MaintenanceHandler {
	ops := providerpostgres.NewOperationStore(db)
	store := providerpostgres.NewMaintenanceStore(db, providers, time.Duration(cfg.RateLimit.OAuth401CooldownMinutes)*time.Minute)
	targets := maintenanceTargets{providers: providers, groups: groups}
	location := time.Local
	if zone, err := time.LoadLocation(cfg.Timezone); cfg.Timezone != "" && err == nil {
		location = zone
	}
	plans := provider.NewMaintenanceService(store, provider.MaintenancePlanOptions{
		Location: location,
		NextRun:  provideradapter.NextScheduledTestRun,
		Targets:  targets.list,
	})
	runner := provider.NewMaintenanceRunner(plans, store, provider.MaintenanceRunnerOptions{
		Resolve: targets.resolve,
		Load:    providers.GetByID,
		Acquire: func(ctx context.Context, record *provider.Record) (func(), error) {
			slot, err := concurrency.AcquireProviderSlot(ctx, record.ID, record.Concurrency)
			if err != nil || slot == nil || !slot.Acquired {
				return nil, err
			}
			return slot.ReleaseFunc, nil
		},
		Observe: tests.ObserveScheduledAuth,
		Probe:   tests.RunStateProbe,
		Candy: func(ctx context.Context, id int64, model string) provider.PelicanEvidence {
			return tests.RunPelican(ctx, id, model, "candy", "", provider.APIProtocolResponses)
		},
		ActiveOperation:  ops.Active,
		Operation:        ops.Get,
		ReloginAvailable: reviveAPIKey() != "",
	})
	manager.Register(lifecycle.Hook{
		Name: "ProviderMaintenance", StartOrder: 982, StopOrder: 18,
		Start: runner.Start, Stop: runner.Stop,
	})
	return providerhttp.NewMaintenanceHandler(plans)
}
