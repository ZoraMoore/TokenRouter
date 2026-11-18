-- Key 请求上限默认为 0，存量 Key 在升级后按不限制处理。
ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS concurrency_limit integer NOT NULL DEFAULT 0 CHECK (concurrency_limit >= 0),
    ADD COLUMN IF NOT EXISTS rpm_limit integer NOT NULL DEFAULT 0 CHECK (rpm_limit >= 0);

-- 请求上限变化后通知各实例清理认证快照。
CREATE OR REPLACE FUNCTION enqueue_api_key_auth_cache_invalidation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        PERFORM enqueue_auth_cache_invalidation(OLD.key);
        RETURN OLD;
    END IF;

    IF OLD.key IS DISTINCT FROM NEW.key
       OR OLD.status IS DISTINCT FROM NEW.status
       OR OLD.deleted_at IS DISTINCT FROM NEW.deleted_at
       OR OLD.user_id IS DISTINCT FROM NEW.user_id
       OR OLD.group_id IS DISTINCT FROM NEW.group_id
       OR OLD.ip_whitelist IS DISTINCT FROM NEW.ip_whitelist
       OR OLD.ip_blacklist IS DISTINCT FROM NEW.ip_blacklist
       OR OLD.expires_at IS DISTINCT FROM NEW.expires_at
       OR OLD.concurrency_limit IS DISTINCT FROM NEW.concurrency_limit
       OR OLD.rpm_limit IS DISTINCT FROM NEW.rpm_limit
       OR OLD.billing_mode IS DISTINCT FROM NEW.billing_mode
       OR OLD.preferred_subscription_id IS DISTINCT FROM NEW.preferred_subscription_id THEN
        PERFORM enqueue_auth_cache_invalidation(OLD.key);
        IF NEW.deleted_at IS NULL AND NEW.key IS DISTINCT FROM OLD.key THEN
            PERFORM enqueue_auth_cache_invalidation(NEW.key);
        END IF;
    END IF;
    RETURN NEW;
END;
$$;
