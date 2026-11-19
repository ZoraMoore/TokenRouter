//go:build integration

package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/testutil/postgrescontainer"
)

// TestListMarketplaceGroups 关闭时跳过历史清单，启用时读取清单和模型策略。
func TestListMarketplaceGroups(t *testing.T) {
	db := postgrescontainer.New(t)
	disabledList := `{"enabled":false,"models":["` + strings.Repeat("x", 400000) + `"]}`
	_, err := db.Exec(`
		INSERT INTO groups(name, sort_order, models_list_config, routing_policy, is_exclusive, status, deleted_at)
		VALUES ('disabled-list', 2, $1::jsonb, '{}', false, 'active', NULL),
		       ('enabled-list', 1, '{"enabled":true,"models":["b","a"]}',
		        '{"enabled":true,"model_mapping":{"b":"target"}}', false, 'active', NULL),
		       ('exclusive', 3, '{}', '{}', true, 'active', NULL),
		       ('inactive', 4, '{}', '{}', false, 'inactive', NULL),
		       ('deleted', 5, '{}', '{}', false, 'active', now())
	`, disabledList)
	require.NoError(t, err)
	store := NewGroupStore(nil, db, GroupStoreOptions{})
	items, err := store.ListMarketplaceGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, "enabled-list", items[0].Name)
	require.Equal(t, []string{"b", "a"}, items[0].ModelsListConfig.Models)
	require.Equal(t, "target", items[0].ModelPolicyView().ResolveModel("b"))
	require.Equal(t, "disabled-list", items[1].Name)
	require.Empty(t, items[1].ModelsListConfig.Models)
}
