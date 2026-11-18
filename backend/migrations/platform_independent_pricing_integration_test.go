//go:build integration

package migrations_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"testing"
	"testing/fstest"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/TokenFlux/TokenRouter/internal/billing/pricing"
	"github.com/TokenFlux/TokenRouter/internal/infra/postgres"
	"github.com/TokenFlux/TokenRouter/migrations"
)

// pricingMigrationPreview 记录各作用域的价格、合并冲突和未绑定 Key 的编号。
type pricingMigrationPreview struct {
	Scopes        []pricingMigrationScope `json:"scopes"`
	UnboundKeyIDs []int64                 `json:"unbound_key_ids"`
	Blocked       bool                    `json:"blocked"`
}

type pricingMigrationScope struct {
	Kind      string                      `json:"kind"`
	ID        int64                       `json:"id"`
	Before    json.RawMessage             `json:"before"`
	After     []pricing.ModelPricingEntry `json:"after"`
	Conflicts []pricingMergeConflict      `json:"conflicts,omitempty"`
}

// TestPlatformIndependentPricingMigration 检查迁移 SQL 的价格合并、作用域隔离和冲突回滚。
func TestPlatformIndependentPricingMigration(t *testing.T) {
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, "postgres:18.1-alpine3.23", tcpostgres.WithDatabase("pricing_migration"), tcpostgres.WithUsername("postgres"), tcpostgres.WithPassword("postgres"), tcpostgres.BasicWaitStrategies())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Terminate(ctx)) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	before := fstest.MapFS{}
	files, err := fs.ReadDir(migrations.FS, ".")
	require.NoError(t, err)
	for _, file := range files {
		if file.Name() >= "276_" {
			continue
		}
		data, err := migrations.FS.ReadFile(file.Name())
		require.NoError(t, err)
		before[file.Name()] = &fstest.MapFile{Data: data}
	}
	require.NoError(t, postgres.ApplyMigrations(ctx, db, before))
	migration, err := migrations.FS.ReadFile("277_platform_independent_pricing.sql")
	require.NoError(t, err)
	t.Run("highest_prices_and_scoped_costs", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { require.NoError(t, tx.Rollback()) }()
		_, err = tx.ExecContext(ctx, `
INSERT INTO pricing_configs(id,name) VALUES (76001,'merge pricing');
INSERT INTO pricing_config_model_pricing(pricing_config_id,platform,models,input_price,output_price,cache_read_price) VALUES
 (76001,'anthropic','["claude-test","unique-test"]',3,15,0),
 (76001,'openai','["CLAUDE-TEST"]',5,10,0);
INSERT INTO pricing_config_account_stats_pricing_rules(id,pricing_config_id,name,account_ids) VALUES
 (76001,76001,'first',ARRAY[1]::BIGINT[]),(76002,76001,'second',ARRAY[2]::BIGINT[]);
INSERT INTO pricing_config_account_stats_model_pricing(rule_id,platform,models,input_price) VALUES
 (76001,'anthropic','["model"]',1),(76001,'openai','["model"]',2),(76002,'openai','["model"]',7);
INSERT INTO groups(name,model_pricing) VALUES ('group price','[{"platform":"anthropic","models":["model"],"input_price":3,"output_price":15},{"platform":"openai","models":["model"],"input_price":5,"output_price":10}]');
`)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, string(migration))
		require.NoError(t, err)
		var input, output, cache float64
		require.NoError(t, tx.QueryRowContext(ctx, `SELECT input_price,output_price,cache_read_price FROM pricing_config_model_pricing WHERE pricing_config_id=76001 AND models='["claude-test"]'`).Scan(&input, &output, &cache))
		require.Equal(t, 5.0, input)
		require.Equal(t, 15.0, output)
		require.Zero(t, cache)
		require.NoError(t, tx.QueryRowContext(ctx, `SELECT input_price FROM pricing_config_account_stats_model_pricing WHERE rule_id=76001`).Scan(&input))
		require.Equal(t, 2.0, input)
		require.NoError(t, tx.QueryRowContext(ctx, `SELECT input_price FROM pricing_config_account_stats_model_pricing WHERE rule_id=76002`).Scan(&input))
		require.Equal(t, 7.0, input)
		var groupPrice string
		require.NoError(t, tx.QueryRowContext(ctx, `SELECT model_pricing::TEXT FROM groups WHERE name='group price'`).Scan(&groupPrice))
		require.JSONEq(t, `[{"models":["model"],"billing_mode":"token","input_price":5,"output_price":15}]`, groupPrice)
		var archived int
		require.NoError(t, tx.QueryRowContext(ctx, `SELECT count(*) FROM platform_independent_pricing_archive WHERE scope='config' AND scope_id=76001`).Scan(&archived))
		require.Equal(t, 1, archived)
		// 再次执行迁移后，管理员修改的价格和条目 ID 保持不变。
		var priceID int64
		require.NoError(t, tx.QueryRowContext(ctx, `UPDATE pricing_config_model_pricing SET input_price=9 WHERE pricing_config_id=76001 AND models='["claude-test"]' RETURNING id`).Scan(&priceID))
		_, err = tx.ExecContext(ctx, string(migration))
		require.NoError(t, err)
		var replayID int64
		require.NoError(t, tx.QueryRowContext(ctx, `SELECT id,input_price FROM pricing_config_model_pricing WHERE pricing_config_id=76001 AND models='["claude-test"]'`).Scan(&replayID, &input))
		require.Equal(t, priceID, replayID)
		require.Equal(t, 9.0, input)
	})
	t.Run("readonly_preflight_matches_merge_and_reports_conflicts", func(t *testing.T) {
		_, err := db.ExecContext(ctx, `INSERT INTO pricing_configs(id,name) VALUES (78001,'preview'); INSERT INTO pricing_config_model_pricing(pricing_config_id,platform,models,input_price,output_price) VALUES (78001,'anthropic','["preview-model"]',3,15),(78001,'openai','["preview-model"]',5,10)`)
		require.NoError(t, err)
		defer func() {
			_, err := db.ExecContext(ctx, `DELETE FROM pricing_configs WHERE id=78001`)
			require.NoError(t, err)
		}()
		preview, err := previewPricingMigration(ctx, db)
		require.NoError(t, err)
		require.False(t, preview.Blocked)
		found := false
		for _, scope := range preview.Scopes {
			if scope.Kind != "config" || scope.ID != 78001 {
				continue
			}
			found = true
			require.Len(t, scope.After, 1)
			require.Equal(t, 5.0, *scope.After[0].InputPrice)
			require.Equal(t, 15.0, *scope.After[0].OutputPrice)
			var original []map[string]any
			require.NoError(t, json.Unmarshal(scope.Before, &original))
			require.Equal(t, "anthropic", original[0]["platform"])
			require.Equal(t, "openai", original[1]["platform"])
		}
		require.True(t, found)
		var count int
		require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM pricing_config_model_pricing WHERE pricing_config_id=78001`).Scan(&count))
		require.Equal(t, 2, count)
		_, err = db.ExecContext(ctx, `UPDATE pricing_config_model_pricing SET input_price=NULL WHERE pricing_config_id=78001 AND platform='openai'`)
		require.NoError(t, err)
		preview, err = previewPricingMigration(ctx, db)
		require.NoError(t, err)
		require.True(t, preview.Blocked)
		var archive sql.NullString
		require.NoError(t, db.QueryRowContext(ctx, `SELECT to_regclass('platform_independent_pricing_archive')::text`).Scan(&archive))
		require.False(t, archive.Valid)
	})
	for _, tc := range []struct{ name, patch string }{
		{"inheritance", `UPDATE pricing_config_model_pricing SET input_price=NULL WHERE platform='openai'`},
		{"billing_mode", `UPDATE pricing_config_model_pricing SET billing_mode='image',per_request_price=2 WHERE platform='openai'`},
		{"multiplier", `UPDATE pricing_config_model_pricing SET price_multiplier=2 WHERE platform='openai'`},
		{"overlapping_patterns", `UPDATE pricing_config_model_pricing SET models='["model*"]' WHERE platform='openai'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx, err := db.BeginTx(ctx, nil)
			require.NoError(t, err)
			_, err = tx.ExecContext(ctx, `INSERT INTO pricing_configs(id,name) VALUES (77001,'conflict'); INSERT INTO pricing_config_model_pricing(pricing_config_id,platform,models,input_price) VALUES (77001,'anthropic','["model"]',1),(77001,'openai','["model"]',2);`+tc.patch)
			require.NoError(t, err)
			_, err = tx.ExecContext(ctx, string(migration))
			require.ErrorContains(t, err, "PRICE_MIGRATION_CONFLICT")
			require.NoError(t, tx.Rollback())
			var exists bool
			require.NoError(t, db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='pricing_config_model_pricing' AND column_name='platform')`).Scan(&exists))
			require.True(t, exists)
		})
	}
}

// previewPricingMigration 通过只读的可重复读事务检查迁移 276 之前的数据库。
// 该版本数据库中的提供商表名为 accounts。
func previewPricingMigration(ctx context.Context, db *sql.DB) (pricingMigrationPreview, error) {
	report := pricingMigrationPreview{Scopes: []pricingMigrationScope{}, UnboundKeyIDs: []int64{}}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return report, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `
SELECT 'config', pricing_config_id, jsonb_agg(to_jsonb(p) || jsonb_build_object('intervals', COALESCE((
 SELECT jsonb_agg(to_jsonb(i) ORDER BY i.sort_order, i.id) FROM pricing_config_pricing_intervals i WHERE i.pricing_id = p.id
), '[]')) ORDER BY p.id) FROM pricing_config_model_pricing p GROUP BY pricing_config_id
UNION ALL
SELECT 'account_rule', rule_id, jsonb_agg(to_jsonb(p) || jsonb_build_object('intervals', COALESCE((
 SELECT jsonb_agg(to_jsonb(i) ORDER BY i.sort_order, i.id) FROM pricing_config_account_stats_pricing_intervals i WHERE i.pricing_id = p.id
), '[]')) ORDER BY p.id) FROM pricing_config_account_stats_model_pricing p GROUP BY rule_id
UNION ALL
SELECT 'group', id, model_pricing FROM groups WHERE model_pricing IS NOT NULL
ORDER BY 1, 2`)
	if err != nil {
		return report, fmt.Errorf("read migration price cards: %w", err)
	}
	for rows.Next() {
		var scope pricingMigrationScope
		var raw []byte
		if err := rows.Scan(&scope.Kind, &scope.ID, &raw); err != nil {
			_ = rows.Close()
			return report, err
		}
		var original []pricing.ModelPricingEntry
		if err := json.Unmarshal(raw, &original); err != nil {
			_ = rows.Close()
			return report, fmt.Errorf("decode %s %d: %w", scope.Kind, scope.ID, err)
		}
		// 迁移前价卡的平台标签与行 ID 帮助管理员定位待合并条目。
		scope.Before = append(json.RawMessage(nil), raw...)
		scope.After, scope.Conflicts = mergePriceCards(original)
		report.Blocked = report.Blocked || len(scope.Conflicts) > 0
		report.Scopes = append(report.Scopes, scope)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return report, err
	}
	keys, err := tx.QueryContext(ctx, `SELECT id FROM api_keys WHERE group_id IS NULL AND NOT is_composite AND deleted_at IS NULL ORDER BY id`)
	if err != nil {
		return report, err
	}
	for keys.Next() {
		var id int64
		if err := keys.Scan(&id); err != nil {
			_ = keys.Close()
			return report, err
		}
		report.UnboundKeyIDs = append(report.UnboundKeyIDs, id)
	}
	err = keys.Err()
	_ = keys.Close()
	if err != nil {
		return report, err
	}
	return report, tx.Commit()
}
