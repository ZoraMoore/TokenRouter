package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	dbent "github.com/TokenFlux/TokenRouter/ent"
	"github.com/TokenFlux/TokenRouter/internal/routing"
)

// ListMarketplaceGroups 批量读取公开分组的展示字段和模型策略。
func (r *GroupStore) ListMarketplaceGroups(ctx context.Context) ([]routing.Group, error) {
	rows, err := r.sqlExecutorFromContext(ctx).QueryContext(ctx, `
		SELECT row_to_json(g)
		FROM (
			SELECT id, name, description, display_brand, localization, rate_multiplier,
			       sort_order, allowed_protocols, protocol_fallbacks, require_oauth_only,
			       routing_policy, availability_probe_config,
			       CASE WHEN models_list_config->'enabled' = 'true'::jsonb
			            THEN models_list_config ELSE '{}'::jsonb END AS models_list_config
			FROM groups
			WHERE deleted_at IS NULL AND status = 'active' AND is_exclusive = false
		) AS g
		ORDER BY g.sort_order, g.id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	groups := make([]routing.Group, 0)
	groupIDs := make([]int64, 0)
	for rows.Next() {
		var body []byte
		var group dbent.Group
		if err := rows.Scan(&body); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(body, &group); err != nil {
			return nil, fmt.Errorf("decode marketplace group: %w", err)
		}
		groups = append(groups, *GroupFromEnt(&group))
		groupIDs = append(groupIDs, group.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	counts, err := r.loadProviderCounts(ctx, groupIDs)
	if err == nil {
		for i := range groups {
			groups[i].ActiveProviderCount = counts[groups[i].ID].Active
		}
	}
	return groups, nil
}
