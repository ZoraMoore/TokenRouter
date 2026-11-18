-- 用户最近使用时间与原始用量在同一事务内维护。
LOCK TABLE usage_logs IN SHARE ROW EXCLUSIVE MODE;

CREATE TABLE IF NOT EXISTS usage_user_activity (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    last_used_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_usage_user_activity_last_used
    ON usage_user_activity (last_used_at DESC, user_id DESC);

-- 专用锁覆盖尚未存在汇总行的用户，多用户批次按相同顺序获取锁。
CREATE OR REPLACE FUNCTION usage_lock_user_activity(ids BIGINT[])
RETURNS VOID LANGUAGE plpgsql VOLATILE AS $$
DECLARE
    target BIGINT;
BEGIN
    FOR target IN SELECT DISTINCT unnest(ids) ORDER BY 1 LOOP
        PERFORM pg_advisory_xact_lock(hashtextextended('usage_user_activity:' || target::text, 0));
    END LOOP;
END
$$;

-- 等待同用户写入提交后，再读取剩余记录；VOLATILE 函数中的查询取得当前快照。
CREATE OR REPLACE FUNCTION usage_refresh_user_activity(ids BIGINT[])
RETURNS VOID LANGUAGE plpgsql VOLATILE AS $$
BEGIN
    IF cardinality(ids) IS NULL OR cardinality(ids) = 0 THEN
        RETURN;
    END IF;
    PERFORM usage_lock_user_activity(ids);
    WITH latest AS MATERIALIZED (
        SELECT requested.user_id, entry.created_at
        FROM (SELECT DISTINCT unnest(ids) AS user_id) requested
        JOIN users u ON u.id = requested.user_id
        CROSS JOIN LATERAL (
            SELECT created_at FROM usage_logs
            WHERE user_id = requested.user_id
            ORDER BY created_at DESC LIMIT 1
        ) entry
    ), removed AS (
        DELETE FROM usage_user_activity a
        WHERE a.user_id = ANY(ids)
          AND NOT EXISTS (SELECT 1 FROM latest l WHERE l.user_id = a.user_id)
    )
    INSERT INTO usage_user_activity (user_id, last_used_at)
    SELECT user_id, created_at FROM latest ORDER BY user_id
    ON CONFLICT (user_id) DO UPDATE SET last_used_at = EXCLUDED.last_used_at
    WHERE usage_user_activity.last_used_at IS DISTINCT FROM EXCLUDED.last_used_at;
END
$$;

CREATE OR REPLACE FUNCTION usage_insert_user_activity()
RETURNS TRIGGER LANGUAGE plpgsql VOLATILE AS $$
BEGIN
    PERFORM usage_lock_user_activity(ARRAY(SELECT DISTINCT user_id FROM new_usage_rows ORDER BY user_id));
    INSERT INTO usage_user_activity (user_id, last_used_at)
    SELECT user_id, MAX(created_at) FROM new_usage_rows GROUP BY user_id ORDER BY user_id
    ON CONFLICT (user_id) DO UPDATE SET last_used_at = EXCLUDED.last_used_at
    WHERE usage_user_activity.last_used_at < EXCLUDED.last_used_at;
    RETURN NULL;
END
$$;

CREATE OR REPLACE FUNCTION usage_delete_user_activity()
RETURNS TRIGGER LANGUAGE plpgsql VOLATILE AS $$
BEGIN
    PERFORM usage_refresh_user_activity(ARRAY(SELECT DISTINCT user_id FROM old_usage_rows));
    RETURN NULL;
END
$$;

CREATE OR REPLACE FUNCTION usage_update_user_activity()
RETURNS TRIGGER LANGUAGE plpgsql VOLATILE AS $$
BEGIN
    -- 费用等字段更新无需重新查询活动时间；EXCEPT ALL 同时处理重复时间和归属变更。
    PERFORM usage_refresh_user_activity(ARRAY(
        SELECT DISTINCT user_id FROM (
            (SELECT user_id, created_at FROM old_usage_rows EXCEPT ALL SELECT user_id, created_at FROM new_usage_rows)
            UNION ALL
            (SELECT user_id, created_at FROM new_usage_rows EXCEPT ALL SELECT user_id, created_at FROM old_usage_rows)
        ) changed
    ));
    RETURN NULL;
END
$$;

CREATE OR REPLACE FUNCTION usage_truncate_user_activity()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    -- 用户表级联清空时，汇总表可能已在当前 TRUNCATE 的目标集合中。
    DELETE FROM usage_user_activity;
    RETURN NULL;
END
$$;

DROP TRIGGER IF EXISTS usage_activity_insert ON usage_logs;
CREATE TRIGGER usage_activity_insert AFTER INSERT ON usage_logs
    REFERENCING NEW TABLE AS new_usage_rows FOR EACH STATEMENT EXECUTE FUNCTION usage_insert_user_activity();
DROP TRIGGER IF EXISTS usage_activity_delete ON usage_logs;
CREATE TRIGGER usage_activity_delete AFTER DELETE ON usage_logs
    REFERENCING OLD TABLE AS old_usage_rows FOR EACH STATEMENT EXECUTE FUNCTION usage_delete_user_activity();
DROP TRIGGER IF EXISTS usage_activity_update ON usage_logs;
CREATE TRIGGER usage_activity_update AFTER UPDATE ON usage_logs
    REFERENCING OLD TABLE AS old_usage_rows NEW TABLE AS new_usage_rows
    FOR EACH STATEMENT EXECUTE FUNCTION usage_update_user_activity();
DROP TRIGGER IF EXISTS usage_activity_truncate ON usage_logs;
CREATE TRIGGER usage_activity_truncate AFTER TRUNCATE ON usage_logs
    FOR EACH STATEMENT EXECUTE FUNCTION usage_truncate_user_activity();

-- 回填、恢复校验和分区清理共用重建入口，调用期间用量写入等待表锁。
CREATE OR REPLACE FUNCTION usage_rebuild_user_activity()
RETURNS VOID LANGUAGE plpgsql VOLATILE AS $$
BEGIN
    LOCK TABLE usage_logs IN SHARE ROW EXCLUSIVE MODE;
    DELETE FROM usage_user_activity;
    INSERT INTO usage_user_activity (user_id, last_used_at)
    SELECT u.id, latest.created_at FROM users u
    CROSS JOIN LATERAL (
        SELECT created_at FROM usage_logs WHERE user_id = u.id
        ORDER BY created_at DESC LIMIT 1
    ) latest;
END
$$;

-- DROP 分区不会触发 DELETE，删除和重建需要共同提交。
CREATE OR REPLACE FUNCTION usage_drop_partition_with_activity(partition_name TEXT)
RETURNS VOID LANGUAGE plpgsql VOLATILE AS $$
DECLARE
    target REGCLASS;
BEGIN
    LOCK TABLE usage_logs IN ACCESS EXCLUSIVE MODE;
    target := to_regclass(partition_name);
    IF target IS NULL THEN
        RETURN;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_inherits WHERE inhrelid = target AND inhparent = 'usage_logs'::regclass) THEN
        RAISE EXCEPTION '目标表不是 usage_logs 的直属分区: %', partition_name;
    END IF;
    EXECUTE format('DROP TABLE %s', target);
    PERFORM usage_rebuild_user_activity();
END
$$;

SELECT usage_rebuild_user_activity();
