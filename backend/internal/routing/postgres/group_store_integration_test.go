//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/require"

	dbent "github.com/TokenFlux/TokenRouter/ent"
	_ "github.com/TokenFlux/TokenRouter/ent/runtime"
	"github.com/TokenFlux/TokenRouter/internal/pkg/pagination"
	"github.com/TokenFlux/TokenRouter/internal/routing"
	"github.com/TokenFlux/TokenRouter/internal/testutil/postgrescontainer"
)

// TestAdminGroupViews 对照表格分页、选项集和包含模型清单的分组详情。
func TestAdminGroupViews(t *testing.T) {
	ctx := context.Background()
	db := postgrescontainer.New(t)
	var queries []string
	client := dbent.NewClient(
		dbent.Driver(entsql.OpenDB(dialect.Postgres, db)),
		dbent.Log(func(entries ...any) { queries = append(queries, fmt.Sprint(entries...)) }),
	).Debug()
	store := NewGroupStore(client, db, GroupStoreOptions{})

	modelIDs := make([]string, 13529)
	for i := range modelIDs {
		modelIDs[i] = fmt.Sprintf("model-%05d", len(modelIDs)-i)
	}
	models, err := json.Marshal(routing.GroupModelsListConfig{Models: modelIDs})
	require.NoError(t, err)

	var active, inactive, earlier, deleted, providerID int64
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO groups(name, description, sort_order, is_exclusive, display_brand, rate_multiplier, claude_code_only, models_list_config, session_isolation_enabled)
		VALUES ('group-search-active', 'search description', 20, true, 'openai', 2.5, true, $1::jsonb, true) RETURNING id`, string(models)).Scan(&active))
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO groups(name, sort_order, status, fallback_group_id_on_invalid_request)
		VALUES ('group-search-inactive', 20, 'inactive', $1) RETURNING id`, active).Scan(&inactive))
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO groups(name, sort_order)
		VALUES ('group-search-earlier', 10) RETURNING id`).Scan(&earlier))
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO groups(name, sort_order, deleted_at)
		VALUES ('group-search-deleted', 0, now()) RETURNING id`).Scan(&deleted))
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO providers(name, platform, type, status, schedulable)
		VALUES ('group-view-provider', 'openai', 'apikey', 'active', true) RETURNING id`).Scan(&providerID))
	_, err = db.ExecContext(ctx, `INSERT INTO provider_groups(group_id, provider_id) VALUES ($1, $2)`, active, providerID)
	require.NoError(t, err)

	options, err := store.ListAdminGroupOptions(ctx, false)
	require.NoError(t, err)
	require.NotContains(t, strings.Join(queries, "\n"), "models_list_config")
	var fixtureIDs []int64
	for _, option := range options {
		if option.ID == active || option.ID == inactive || option.ID == earlier || option.ID == deleted {
			fixtureIDs = append(fixtureIDs, option.ID)
		}
	}
	require.Equal(t, []int64{earlier, active}, fixtureIDs)

	options, err = store.ListAdminGroupOptions(ctx, true)
	require.NoError(t, err)
	fixtureIDs = nil
	for _, option := range options {
		switch option.ID {
		case active, inactive, earlier, deleted:
			fixtureIDs = append(fixtureIDs, option.ID)
		}
		if option.ID == inactive {
			require.Equal(t, &active, option.FallbackGroupIDOnInvalidRequest)
		}
		if option.ID == active {
			require.Equal(t, "search description", option.Description)
			require.Equal(t, "openai", option.DisplayBrand)
			require.Equal(t, 2.5, option.RateMultiplier)
			require.EqualValues(t, 1, option.ProviderCount)
			require.True(t, option.IsExclusive)
			require.True(t, option.ClaudeCodeOnly)
		}
	}
	require.Equal(t, []int64{earlier, active, inactive}, fixtureIDs)

	for _, sortBy := range []string{"sort_order", "provider_count", "created_at", "name", "display_brand", "rate_multiplier", "is_exclusive", "session_isolation_enabled", "status", "id"} {
		t.Run(sortBy, func(t *testing.T) {
			for _, sortOrder := range []string{"asc", "desc"} {
				params := pagination.PaginationParams{Page: 1, PageSize: 2, SortBy: sortBy, SortOrder: sortOrder}
				full, fullPage, err := store.ListWithFilters(ctx, params, "", "", "group-search", nil)
				require.NoError(t, err)
				queries = nil
				table, tablePage, err := store.ListAdminGroupTable(ctx, params, "", "group-search", nil)
				require.NoError(t, err)
				require.NotContains(t, strings.Join(queries, "\n"), "models_list_config")
				require.EqualValues(t, 3, tablePage.Total)
				require.Equal(t, fullPage, tablePage)
				require.Len(t, table, len(full))
				for i, row := range table {
					require.Equal(t, full[i].ID, row.ID)
					require.Equal(t, full[i].ProviderCount, row.ProviderCount)
					require.Equal(t, full[i].ActiveProviderCount, row.ActiveProviderCount)
					require.Equal(t, full[i].RateLimitedProviderCount, row.RateLimitedProviderCount)
				}
			}
		})
	}

	exclusive := true
	table, filteredPage, err := store.ListAdminGroupTable(ctx,
		pagination.PaginationParams{Page: 1, PageSize: 20},
		routing.StatusActive, "SEARCH DESCRIPTION", &exclusive)
	require.NoError(t, err)
	require.EqualValues(t, 1, filteredPage.Total)
	require.Len(t, table, 1)
	require.Equal(t, active, table[0].ID)
	require.True(t, table[0].SessionIsolationEnabled)

	detail, err := store.GetByID(ctx, active)
	require.NoError(t, err)
	require.False(t, detail.ModelsListConfig.Enabled)
	require.Equal(t, modelIDs, detail.ModelsListConfig.Models)
}
