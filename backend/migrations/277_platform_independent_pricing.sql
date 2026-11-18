-- 同版本停机升级：归档原价卡，合并可比较的价格后取消平台维度。
-- 任一复杂冲突都会使整个迁移事务回滚，不得用后写覆盖解决冲突。
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

-- 重放不能用迁移归档覆盖升级后调整的价卡，旧列只在首次切换前存在。
DROP TABLE IF EXISTS pg_temp.tr_pricing_migration_state;
CREATE TEMP TABLE tr_pricing_migration_state ON COMMIT DROP AS
SELECT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = current_schema()
      AND table_name = 'pricing_config_model_pricing' AND column_name = 'platform'
) AS needed;

CREATE TABLE IF NOT EXISTS platform_independent_pricing_archive (
    scope TEXT NOT NULL,
    scope_id BIGINT NOT NULL,
    entries JSONB NOT NULL,
    archived_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (scope, scope_id)
);

-- 临时函数随迁移连接释放，不构成长期数据库接口。
CREATE OR REPLACE FUNCTION pg_temp.tr_price_rules(card JSONB) RETURNS JSONB
LANGUAGE plpgsql AS $$
DECLARE
    result JSONB;
    intervals JSONB;
BEGIN
    result := card - ARRAY['id', 'pricing_config_id', 'pricing_id', 'rule_id', 'platform', 'models', 'created_at', 'updated_at',
        'input_price', 'output_price', 'cache_write_price', 'cache_write_1h_price', 'cache_read_price',
        'image_input_price', 'image_output_price', 'per_request_price'];
    result := jsonb_set(result, '{billing_mode}', to_jsonb(COALESCE(NULLIF(card->>'billing_mode', ''), 'token')));
    SELECT COALESCE(jsonb_agg(jsonb_strip_nulls(value - ARRAY['id', 'pricing_id', 'created_at', 'updated_at',
        'input_price', 'output_price', 'cache_write_price', 'cache_write_1h_price', 'cache_read_price', 'per_request_price']) ORDER BY ordinal), '[]')
    INTO intervals FROM jsonb_array_elements(COALESCE(NULLIF(card->'intervals', 'null'), '[]')) WITH ORDINALITY AS item(value, ordinal);
    RETURN jsonb_strip_nulls(jsonb_set(result, '{intervals}', intervals));
END $$;

CREATE OR REPLACE FUNCTION pg_temp.tr_merge_price_card(left_card JSONB, right_card JSONB) RETURNS JSONB
LANGUAGE plpgsql AS $$
DECLARE
    result JSONB := left_card - 'platform';
    key TEXT;
    left_value NUMERIC;
    right_value NUMERIC;
    left_intervals JSONB := COALESCE(NULLIF(left_card->'intervals', 'null'), '[]');
    right_intervals JSONB := COALESCE(NULLIF(right_card->'intervals', 'null'), '[]');
    merged_intervals JSONB := '[]';
    i INTEGER;
BEGIN
    IF pg_temp.tr_price_rules(left_card) <> pg_temp.tr_price_rules(right_card) THEN
        RAISE EXCEPTION 'PRICE_MIGRATION_CONFLICT models=% entries=%,%: billing mode, intervals, multipliers or time rules differ',
            left_card->'models', left_card->'id', right_card->'id';
    END IF;
    FOREACH key IN ARRAY ARRAY['input_price', 'output_price', 'cache_write_price', 'cache_write_1h_price', 'cache_read_price',
        'image_input_price', 'image_output_price', 'per_request_price'] LOOP
        left_value := (left_card->>key)::NUMERIC;
        right_value := (right_card->>key)::NUMERIC;
        IF (left_value IS NULL) <> (right_value IS NULL) THEN
            RAISE EXCEPTION 'PRICE_MIGRATION_CONFLICT models=% entries=%,% bucket=%: inherited and explicit prices cannot be compared',
                left_card->'models', left_card->'id', right_card->'id', key;
        END IF;
        IF left_value IS NOT NULL THEN
            IF left_value < 0 OR right_value < 0 OR left_value::TEXT IN ('NaN', 'Infinity', '-Infinity') OR right_value::TEXT IN ('NaN', 'Infinity', '-Infinity') THEN
                RAISE EXCEPTION 'PRICE_MIGRATION_CONFLICT: invalid price for %', key;
            END IF;
            result := jsonb_set(result, ARRAY[key], to_jsonb(GREATEST(left_value, right_value)));
        END IF;
    END LOOP;
    FOR i IN 0..jsonb_array_length(left_intervals)-1 LOOP
        merged_intervals := merged_intervals || jsonb_build_array(pg_temp.tr_merge_price_card(left_intervals->i, right_intervals->i));
    END LOOP;
    IF left_card ? 'intervals' OR right_card ? 'intervals' THEN
        result := jsonb_set(result, '{intervals}', merged_intervals);
    END IF;
    RETURN result;
END $$;

CREATE OR REPLACE FUNCTION pg_temp.tr_merge_price_cards(entries JSONB) RETURNS JSONB
LANGUAGE plpgsql AS $$
DECLARE
    by_model JSONB := '{}';
    entry JSONB;
    model TEXT;
    left_model TEXT;
    right_model TEXT;
    result JSONB;
BEGIN
    FOR entry IN SELECT value FROM jsonb_array_elements(COALESCE(NULLIF(entries, 'null'), '[]')) LOOP
        FOR model IN SELECT lower(btrim(value)) FROM jsonb_array_elements_text(COALESCE(NULLIF(entry->'models', 'null'), '[]')) LOOP
            IF model LIKE 'claude-%' THEN model := replace(model, '.', '-'); END IF;
            IF model = '' OR position('*' IN rtrim(model, '*')) > 0 OR model LIKE '%**' THEN
                RAISE EXCEPTION 'PRICE_MIGRATION_CONFLICT: invalid model pattern %', model;
            END IF;
            entry := jsonb_set(entry - 'platform', '{billing_mode}', to_jsonb(COALESCE(NULLIF(entry->>'billing_mode', ''), 'token')));
            IF by_model ? model THEN
                by_model := jsonb_set(by_model, ARRAY[model], pg_temp.tr_merge_price_card(by_model->model, jsonb_set(entry, '{models}', jsonb_build_array(model))));
            ELSE
                by_model := jsonb_set(by_model, ARRAY[model], jsonb_set(entry, '{models}', jsonb_build_array(model)));
            END IF;
        END LOOP;
    END LOOP;
    FOR left_model, right_model IN SELECT a.key, b.key FROM jsonb_each(by_model) a CROSS JOIN jsonb_each(by_model) b WHERE a.key < b.key LOOP
        IF (right(left_model, 1) = '*' AND starts_with(rtrim(right_model, '*'), left(left_model, length(left_model)-1))) OR
           (right(right_model, 1) = '*' AND starts_with(rtrim(left_model, '*'), left(right_model, length(right_model)-1))) THEN
            RAISE EXCEPTION 'PRICE_MIGRATION_CONFLICT: overlapping model patterns % and %', left_model, right_model;
        END IF;
    END LOOP;
    SELECT COALESCE(jsonb_agg(value ORDER BY key), '[]') INTO result FROM jsonb_each(by_model);
    RETURN result;
END $$;

INSERT INTO platform_independent_pricing_archive(scope, scope_id, entries)
SELECT 'config', pricing_config_id, jsonb_agg(to_jsonb(p) || jsonb_build_object('intervals', COALESCE((
    SELECT jsonb_agg(to_jsonb(i) ORDER BY i.sort_order, i.id) FROM pricing_config_pricing_intervals i WHERE i.pricing_id = p.id
), '[]')) ORDER BY p.id)
FROM pricing_config_model_pricing p WHERE (SELECT needed FROM tr_pricing_migration_state)
GROUP BY pricing_config_id ON CONFLICT DO NOTHING;

INSERT INTO platform_independent_pricing_archive(scope, scope_id, entries)
SELECT 'account_rule', rule_id, jsonb_agg(to_jsonb(p) || jsonb_build_object('intervals', COALESCE((
    SELECT jsonb_agg(to_jsonb(i) ORDER BY i.sort_order, i.id) FROM pricing_config_account_stats_pricing_intervals i WHERE i.pricing_id = p.id
), '[]')) ORDER BY p.id)
FROM pricing_config_account_stats_model_pricing p WHERE (SELECT needed FROM tr_pricing_migration_state)
GROUP BY rule_id ON CONFLICT DO NOTHING;

INSERT INTO platform_independent_pricing_archive(scope, scope_id, entries)
SELECT 'group', id, model_pricing FROM groups
WHERE model_pricing IS NOT NULL AND (SELECT needed FROM tr_pricing_migration_state) ON CONFLICT DO NOTHING;

-- 先计算全部作用域的合并结果；后续写入不再遇到业务冲突。
DROP TABLE IF EXISTS pg_temp.tr_merged_prices;
CREATE TEMP TABLE tr_merged_prices ON COMMIT DROP AS
SELECT scope, scope_id, pg_temp.tr_merge_price_cards(entries) AS entries
FROM platform_independent_pricing_archive WHERE (SELECT needed FROM tr_pricing_migration_state);

ALTER TABLE pricing_config_model_pricing DROP COLUMN IF EXISTS platform;
ALTER TABLE pricing_config_account_stats_model_pricing DROP COLUMN IF EXISTS platform;

DO $$
DECLARE
    item RECORD;
    entry JSONB;
    interval JSONB;
    table_name TEXT;
    interval_table TEXT;
    scope_column TEXT;
    new_id BIGINT;
BEGIN
    FOR item IN SELECT * FROM tr_merged_prices ORDER BY scope, scope_id LOOP
        IF item.scope = 'group' THEN
            UPDATE groups SET model_pricing = item.entries WHERE id = item.scope_id;
            CONTINUE;
        END IF;
        IF item.scope = 'config' THEN
            table_name := 'pricing_config_model_pricing';
            interval_table := 'pricing_config_pricing_intervals';
            scope_column := 'pricing_config_id';
        ELSE
            table_name := 'pricing_config_account_stats_model_pricing';
            interval_table := 'pricing_config_account_stats_pricing_intervals';
            scope_column := 'rule_id';
        END IF;
        EXECUTE format('DELETE FROM %I WHERE %I = $1', table_name, scope_column) USING item.scope_id;
        FOR entry IN SELECT value FROM jsonb_array_elements(item.entries) LOOP
            new_id := nextval(pg_get_serial_sequence(table_name, 'id'));
            entry := entry || jsonb_build_object('id', new_id, scope_column, item.scope_id);
            EXECUTE format('INSERT INTO %I SELECT * FROM jsonb_populate_record(NULL::%I, $1)', table_name, table_name) USING entry;
            FOR interval IN SELECT value FROM jsonb_array_elements(COALESCE(NULLIF(entry->'intervals', 'null'), '[]')) LOOP
                interval := interval || jsonb_build_object('id', nextval(pg_get_serial_sequence(interval_table, 'id')), 'pricing_id', new_id);
                EXECUTE format('INSERT INTO %I SELECT * FROM jsonb_populate_record(NULL::%I, $1)', interval_table, interval_table) USING interval;
            END LOOP;
        END LOOP;
    END LOOP;
END $$;
