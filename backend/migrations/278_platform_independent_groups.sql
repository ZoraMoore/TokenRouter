-- 分组跨平台切换：执行前停止全部旧实例和 worker，普通迁移由 runner 在单一事务内提交。
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '30min';

-- 归档只保存首次迁移前的数据，重放不会覆盖；归档不能代替完整数据库备份。
CREATE TABLE IF NOT EXISTS platform_independent_group_archive (
    group_id BIGINT PRIMARY KEY,
    original_group JSONB NOT NULL,
    archived_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS removed_platform_quota_archive (
    source_table TEXT NOT NULL,
    source_id TEXT NOT NULL,
    original_record JSONB NOT NULL,
    archived_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (source_table, source_id)
);

-- 固定缺省值由 PostgreSQL 元数据保存，OpenAI 历史记录无需逐行重写。
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS platform VARCHAR(64) NOT NULL DEFAULT 'openai';
ALTER TABLE usage_logs ALTER COLUMN platform SET DEFAULT 'unknown';

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema() AND table_name = 'groups' AND column_name = 'platform') THEN
        INSERT INTO platform_independent_group_archive (group_id, original_group)
        SELECT id, to_jsonb(g) FROM groups g ON CONFLICT (group_id) DO NOTHING;

        -- 旧记录保存升级前的查询口径，资金明细和已存在的小时/日桶均不重算。
        UPDATE usage_logs ul SET platform = g.platform
        FROM groups g
        WHERE g.id = ul.group_id AND NULLIF(g.platform, '') IS NOT NULL
            AND ul.platform IS DISTINCT FROM g.platform;

        -- 缺少分组平台时查询账号；缺失关联和空平台归入 unknown。
        UPDATE usage_logs ul SET platform = COALESCE(
            (SELECT NULLIF(a.platform, '') FROM accounts a WHERE a.id = ul.account_id), 'unknown')
        WHERE NOT EXISTS (
            SELECT 1 FROM groups g WHERE g.id = ul.group_id AND NULLIF(g.platform, '') IS NOT NULL
        ) AND ul.platform IS DISTINCT FROM COALESCE(
            (SELECT NULLIF(a.platform, '') FROM accounts a WHERE a.id = ul.account_id), 'unknown');

        UPDATE ops_error_logs o SET platform = COALESCE(
            (SELECT NULLIF(g.platform, '') FROM groups g WHERE g.id = o.group_id),
            (SELECT NULLIF(a.platform, '') FROM accounts a WHERE a.id = o.account_id), 'unknown')
        WHERE COALESCE(o.platform, '') = '';

        -- 只启用原分组所属平台的策略分支，其余草稿留在归档中。
        UPDATE groups SET routing_policy = jsonb_set(jsonb_set(COALESCE(routing_policy, '{}'),
            '{model_mapping}', COALESCE(routing_policy->'model_mapping'->platform, '{}'::jsonb)),
            '{allowed_models}', COALESCE(routing_policy->'allowed_models'->platform, '[]'::jsonb));

        -- 未配置的旧入口仍保持仅原生；显式目标改为有序数组，不改变 allowed_protocols。
        UPDATE groups SET protocol_fallbacks = (
            SELECT jsonb_object_agg(protocol, CASE
                WHEN jsonb_typeof(protocol_fallbacks->protocol) = 'array' THEN protocol_fallbacks->protocol
                WHEN jsonb_typeof(protocol_fallbacks->protocol) = 'string'
                    AND protocol_fallbacks->>protocol <> '' THEN jsonb_build_array(protocol_fallbacks->>protocol)
                ELSE '[]'::jsonb END)
            FROM (SELECT unnest(ARRAY[
                'anthropic_messages', 'openai_responses', 'openai_chat_completions', 'gemini_generate_content',
                'openai_embeddings', 'openai_images_generations', 'openai_images_edits', 'image_batches',
                'grok_videos_generations', 'grok_videos_edits', 'grok_videos_extensions', 'grok_tts', 'grok_stt',
                'grok_custom_voices', 'grok_voice_realtime', 'openai_responses_websocket', 'openai_live',
                'openai_responses_compact', 'openai_alpha_search', 'grok_web_search', 'grok_x_search',
                'qoder_chat', 'gemini_batch_generate_content', 'vertex_batch_prediction'
            ]) AS protocol UNION SELECT jsonb_object_keys(protocol_fallbacks)) protocols
        );

        -- 仅首次升级转换旧全模型透传；重放不得改写升级后管理员设置的默认模型范围。
        UPDATE accounts SET credentials = jsonb_set(COALESCE(credentials, '{}'), '{model_whitelist}', '["*"]')
        WHERE platform = 'openai'
            AND (CASE WHEN jsonb_typeof(extra->'openai_passthrough') = 'boolean' THEN extra->'openai_passthrough'
                ELSE COALESCE(extra->'openai_oauth_passthrough', 'false') END) = 'true'::jsonb
            AND (NOT COALESCE(credentials, '{}') ? 'model_whitelist'
                OR credentials->'model_whitelist' IN ('null'::jsonb, '[]'::jsonb));

        ALTER TABLE groups DROP COLUMN platform;
        ALTER TABLE groups DROP COLUMN IF EXISTS is_default;
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema()
        AND table_name = 'api_keys' AND column_name = 'fallback_to_default_group_when_unavailable') THEN
        ALTER TABLE api_keys RENAME COLUMN fallback_to_default_group_when_unavailable TO fallback_when_group_unavailable;
    END IF;

    IF to_regclass('user_platform_quotas') IS NOT NULL THEN
        INSERT INTO removed_platform_quota_archive (source_table, source_id, original_record)
        SELECT 'user_platform_quotas', id::text, to_jsonb(q) FROM user_platform_quotas q
        ON CONFLICT (source_table, source_id) DO NOTHING;
        DROP TABLE user_platform_quotas;
    END IF;
END $$;

COMMENT ON COLUMN api_keys.fallback_when_group_unavailable IS '绑定分组不可用时允许回退到管理员明确配置的目标';

INSERT INTO removed_platform_quota_archive (source_table, source_id, original_record)
SELECT 'settings', key, to_jsonb(s) FROM settings s WHERE key IN (
    'default_platform_quotas', 'auth_source_default_email_platform_quotas',
    'auth_source_default_linuxdo_platform_quotas', 'auth_source_default_oidc_platform_quotas',
    'auth_source_default_wechat_platform_quotas', 'auth_source_default_github_platform_quotas',
    'auth_source_default_google_platform_quotas', 'auth_source_default_dingtalk_platform_quotas'
) ON CONFLICT (source_table, source_id) DO NOTHING;
DELETE FROM settings WHERE key IN (
    'default_platform_quotas', 'auth_source_default_email_platform_quotas',
    'auth_source_default_linuxdo_platform_quotas', 'auth_source_default_oidc_platform_quotas',
    'auth_source_default_wechat_platform_quotas', 'auth_source_default_github_platform_quotas',
    'auth_source_default_google_platform_quotas', 'auth_source_default_dingtalk_platform_quotas'
);

UPDATE accounts SET extra = extra - 'mixed_scheduling' WHERE extra ? 'mixed_scheduling';

-- 自动无分组调度入口随默认组下线；保留原设置便于升级核对。
CREATE TABLE IF NOT EXISTS platform_independent_setting_archive (
    key TEXT PRIMARY KEY,
    original_record JSONB NOT NULL,
    archived_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO platform_independent_setting_archive (key, original_record)
SELECT key, to_jsonb(s) FROM settings s WHERE key = 'allow_ungrouped_key_scheduling'
ON CONFLICT (key) DO NOTHING;
DELETE FROM settings WHERE key = 'allow_ungrouped_key_scheduling';
