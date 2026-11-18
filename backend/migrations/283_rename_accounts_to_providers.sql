-- 提供商改名只修改历史大表的元数据；不回填用量或重建索引。
SET LOCAL lock_timeout = '10s';
SET LOCAL statement_timeout = '120s';

DO $$
DECLARE
    item RECORD;
    target_tables OID[];
BEGIN
    -- 名称来自当前应用 schema，限定在 public，避免触及扩展与其它租户 schema。
    FOR item IN
        SELECT c.oid, c.relname FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = 'public' AND c.relkind = 'r'
          AND c.relname IN ('accounts', 'account_groups',
              'pricing_config_account_stats_pricing_rules',
              'pricing_config_account_stats_model_pricing',
              'pricing_config_account_stats_pricing_intervals')
        ORDER BY c.relname
    LOOP
        EXECUTE format('ALTER TABLE public.%I RENAME TO %I', item.relname, replace(item.relname, 'account', 'provider'));
    END LOOP;

    -- 元数据范围只包含本应用的提供商相关表，不修改其它 public 表的同名词汇。
    SELECT array_agg(c.oid) INTO target_tables FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = 'public' AND c.relkind = 'r' AND c.relname IN ('providers', 'provider_groups', 'batch_image_jobs',
              'content_moderation_cyber_warnings', 'creative_runs',
              'group_availability_probe_results', 'ops_error_logs', 'ops_system_logs',
              'ops_system_metrics', 'pricing_config_provider_stats_pricing_rules',
              'scheduled_test_plans', 'scheduler_outbox', 'usage_analytics_daily',
              'usage_analytics_hourly', 'usage_dashboard_daily', 'usage_dashboard_hourly', 'usage_logs');
    SELECT target_tables || COALESCE(array_agg(c.oid), '{}'::oid[]) INTO target_tables
    FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = 'public' AND c.relname IN ('pricing_config_provider_stats_model_pricing', 'pricing_config_provider_stats_pricing_intervals');

    FOR item IN
        SELECT c.relname, a.attname FROM pg_attribute a
        JOIN pg_class c ON c.oid = a.attrelid
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = 'public' AND c.relkind = 'r'
          AND a.attnum > 0 AND NOT a.attisdropped AND a.attname LIKE '%account%'
          AND c.relname IN ('providers', 'provider_groups', 'batch_image_jobs',
              'content_moderation_cyber_warnings', 'creative_runs',
              'group_availability_probe_results', 'ops_error_logs', 'ops_system_logs',
              'ops_system_metrics', 'pricing_config_provider_stats_pricing_rules',
              'scheduled_test_plans', 'scheduler_outbox', 'usage_analytics_daily',
              'usage_analytics_hourly', 'usage_dashboard_daily', 'usage_dashboard_hourly', 'usage_logs')
        ORDER BY c.relname, a.attnum
    LOOP
        EXECUTE format('ALTER TABLE public.%I RENAME COLUMN %I TO %I', item.relname, item.attname, replace(item.attname, 'account', 'provider'));
    END LOOP;

    -- 创作与批量图片中的旧 provider 字段表示平台，不是提供商实体。
    FOR item IN
        SELECT table_name FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name IN ('creative_runs', 'batch_image_jobs') AND column_name = 'provider'
    LOOP
        EXECUTE format('ALTER TABLE public.%I RENAME COLUMN provider TO platform', item.table_name);
    END LOOP;

    -- 外键自动跟随表和列；约束及索引改名保留物理索引与验证状态。
    FOR item IN
        SELECT c.relname, con.conname FROM pg_constraint con
        JOIN pg_class c ON c.oid = con.conrelid
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = 'public' AND con.conname LIKE '%account%' AND con.conrelid = ANY(target_tables)
        ORDER BY c.relname, con.conname
    LOOP
        EXECUTE format('ALTER TABLE public.%I RENAME CONSTRAINT %I TO %I', item.relname, item.conname, replace(item.conname, 'account', 'provider'));
    END LOOP;
    FOR item IN
        SELECT c.relname, c.relkind FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = 'public' AND c.relkind IN ('i', 'S') AND c.relname LIKE '%account%'
          AND ((c.relkind = 'i' AND EXISTS (SELECT 1 FROM pg_index i WHERE i.indexrelid = c.oid AND i.indrelid = ANY(target_tables)))
            OR (c.relkind = 'S' AND EXISTS (SELECT 1 FROM pg_depend d WHERE d.classid = 'pg_class'::regclass
                AND d.objid = c.oid AND d.refclassid = 'pg_class'::regclass AND d.refobjid = ANY(target_tables) AND d.deptype IN ('a', 'i'))))
        ORDER BY c.relname
    LOOP
        EXECUTE format('ALTER %s public.%I RENAME TO %I', CASE WHEN item.relkind = 'i' THEN 'INDEX' ELSE 'SEQUENCE' END, item.relname, replace(item.relname, 'account', 'provider'));
    END LOOP;
END $$;

-- 只迁移明确归本项目所有的配置路径；模型别名、请求头及第三方 JSON 保持原样。
CREATE OR REPLACE FUNCTION pg_temp.rename_provider_config_key(value JSONB, old_key TEXT, new_key TEXT)
RETURNS JSONB LANGUAGE plpgsql AS $$
BEGIN
    IF jsonb_typeof(value) <> 'object' OR NOT value ? old_key THEN
        RETURN value;
    END IF;
    IF value ? new_key THEN
        RAISE EXCEPTION 'provider configuration key conflict: %', new_key;
    END IF;
    RETURN (value - old_key) || jsonb_build_object(new_key, value -> old_key);
END $$;

CREATE OR REPLACE FUNCTION pg_temp.provider_credentials(value JSONB)
RETURNS JSONB LANGUAGE SQL AS $$
    SELECT pg_temp.rename_provider_config_key(
        pg_temp.rename_provider_config_key(value, 'account_mode', 'provider_mode'),
        'account_scheduling_threshold', 'provider_scheduling_threshold');
$$;

UPDATE providers SET credentials = pg_temp.provider_credentials(credentials)
WHERE credentials ?| ARRAY['account_mode', 'account_scheduling_threshold'];

-- 唯一约束检测新旧设置键冲突，不覆盖已有值；通知投递记录保持原样。
UPDATE settings SET key = replace(key, 'account', 'provider')
WHERE key IN ('account_quota_notify_enabled', 'account_quota_notify_emails', 'account_scheduling_thresholds')
    OR key IN ('notification_email_template:account.quota_alert:en', 'notification_email_template:account.quota_alert:zh');

DO $$
DECLARE
    item RECORD;
    payload JSONB;
    report JSONB;
BEGIN
    FOR item IN SELECT id, key, value FROM settings
        WHERE key IN ('openai_oauth_import_defaults', 'ops_email_notification_config', 'ops_advanced_settings')
            AND NULLIF(btrim(value), '') IS NOT NULL LOOP
        payload := item.value::jsonb;
        CASE item.key
        WHEN 'openai_oauth_import_defaults' THEN
            payload := pg_temp.rename_provider_config_key(payload, 'account', 'provider');
            IF jsonb_typeof(payload -> 'credentials') = 'object' THEN
                payload := jsonb_set(payload, '{credentials}', pg_temp.provider_credentials(payload -> 'credentials'), false);
            END IF;
        WHEN 'ops_email_notification_config' THEN
            report := payload -> 'report';
            IF jsonb_typeof(report) = 'object' THEN
                report := pg_temp.rename_provider_config_key(report, 'account_health_enabled', 'provider_health_enabled');
                report := pg_temp.rename_provider_config_key(report, 'account_health_schedule', 'provider_health_schedule');
                report := pg_temp.rename_provider_config_key(report, 'account_health_error_rate_threshold', 'provider_health_error_rate_threshold');
                payload := jsonb_set(payload, '{report}', report, false);
            END IF;
        WHEN 'ops_advanced_settings' THEN
            payload := pg_temp.rename_provider_config_key(payload, 'openai_account_quota_auto_pause', 'openai_provider_quota_auto_pause');
            payload := pg_temp.rename_provider_config_key(payload, 'ignore_no_available_accounts', 'ignore_no_available_providers');
        END CASE;
        UPDATE settings SET value = payload::text WHERE id = item.id;
    END LOOP;
END $$;

-- 仅额度告警使用提供商占位符；其它事件模板仍保留原文。
DO $$
DECLARE
    item RECORD;
    payload JSONB;
    field_name TEXT;
BEGIN
    FOR item IN SELECT id, value FROM settings
        WHERE key IN ('notification_email_template:provider.quota_alert:en', 'notification_email_template:provider.quota_alert:zh')
            AND value LIKE '%account_%' LOOP
        payload := item.value::jsonb;
        FOREACH field_name IN ARRAY ARRAY['subject', 'html'] LOOP
            IF jsonb_typeof(payload -> field_name) = 'string' THEN
                payload := jsonb_set(payload, ARRAY[field_name], to_jsonb(regexp_replace(
                    payload ->> field_name, '\{\{\s*account_(id|name)\s*\}\}', '{{provider_\1}}', 'g')), false);
            END IF;
        END LOOP;
        UPDATE settings SET value = payload::text WHERE id = item.id;
    END LOOP;
END $$;

-- 告警过滤字段没有本次改名项，只迁移明确的指标枚举。
UPDATE ops_alert_rules SET metric_type = replace(metric_type, 'account', 'provider')
WHERE metric_type IN ('group_available_accounts', 'account_rate_limited_count', 'account_error_count', 'account_error_ratio', 'overload_account_count');
