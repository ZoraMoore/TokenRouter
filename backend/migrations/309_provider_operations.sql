CREATE TABLE IF NOT EXISTS provider_operations (
    id UUID PRIMARY KEY,
    provider_id BIGINT NOT NULL,
    provider_name TEXT NOT NULL,
    group_id BIGINT NOT NULL DEFAULT 0,
    kind TEXT NOT NULL CHECK (kind IN ('relogin', 'traffic_push')),
    state TEXT NOT NULL CHECK (state IN ('queued', 'running', 'succeeded', 'failed', 'needs_action')),
    stage TEXT NOT NULL DEFAULT '',
    message TEXT NOT NULL DEFAULT '',
    checkpoint JSONB NOT NULL DEFAULT '{}',
    result JSONB NOT NULL DEFAULT '{}',
    next_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 重登可能替换账号 ID，历史任务仍需保留原 ID 和检查点。
CREATE UNIQUE INDEX IF NOT EXISTS provider_operations_active_provider_idx
    ON provider_operations (provider_id) WHERE state IN ('queued', 'running');

CREATE INDEX IF NOT EXISTS provider_operations_queue_idx
    ON provider_operations (kind, next_at, created_at) WHERE state IN ('queued', 'running');

CREATE INDEX IF NOT EXISTS provider_operations_recent_idx ON provider_operations (created_at DESC, id);
CREATE INDEX IF NOT EXISTS provider_operations_provider_recent_idx ON provider_operations (provider_id, created_at DESC, id);
