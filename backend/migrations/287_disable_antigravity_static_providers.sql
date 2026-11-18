-- Antigravity 静态凭据停止提供服务，历史记录通过原提供商 ID 查询。
WITH disabled AS (
    UPDATE providers
    SET status = 'inactive', schedulable = false, updated_at = NOW()
    WHERE platform = 'antigravity'
      AND type IN ('apikey', 'upstream')
      AND deleted_at IS NULL
      AND (status <> 'inactive' OR schedulable)
    RETURNING id
)
INSERT INTO scheduler_outbox (event_type, provider_id)
SELECT 'provider_changed', id FROM disabled;
