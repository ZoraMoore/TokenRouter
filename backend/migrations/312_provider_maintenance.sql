CREATE TABLE provider_maintenance_plans (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    draft JSONB NOT NULL,
    enabled BOOLEAN NOT NULL,
    revision BIGINT NOT NULL DEFAULT 1,
    next_run_at TIMESTAMPTZ,
    CHECK (enabled = (next_run_at IS NOT NULL))
);
CREATE INDEX provider_maintenance_plans_due ON provider_maintenance_plans (next_run_at, id) WHERE enabled;

CREATE TABLE provider_maintenance_runs (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    plan_id BIGINT NOT NULL REFERENCES provider_maintenance_plans(id),
    plan_revision BIGINT NOT NULL,
    draft JSONB NOT NULL,
    trigger TEXT NOT NULL CHECK (trigger IN ('manual', 'schedule')),
    status TEXT NOT NULL CHECK (status IN ('queued', 'running', 'finished', 'cancelled')),
    scheduled_at TIMESTAMPTZ NOT NULL,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX provider_maintenance_run_schedule
    ON provider_maintenance_runs (plan_id, scheduled_at) WHERE trigger = 'schedule';
CREATE UNIQUE INDEX provider_maintenance_run_active
    ON provider_maintenance_runs (plan_id) WHERE status IN ('queued', 'running');
CREATE INDEX provider_maintenance_run_history ON provider_maintenance_runs (plan_id, id DESC);

CREATE TABLE provider_maintenance_items (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    run_id BIGINT NOT NULL REFERENCES provider_maintenance_runs(id) ON DELETE CASCADE,
    provider_id BIGINT NOT NULL,
    target JSONB NOT NULL,
    stage TEXT NOT NULL CHECK (stage IN ('auth', 'relogin', 'verify', 'probe', 'candy')),
    state TEXT NOT NULL CHECK (state IN ('queued', 'running', 'finished', 'cancelled')),
    result JSONB NOT NULL DEFAULT '{}',
    observed JSONB NOT NULL DEFAULT '{}',
    operation_id UUID REFERENCES provider_operations(id),
    replacement_id BIGINT,
    next_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    lease UUID,
    lease_until TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    UNIQUE (run_id, provider_id)
);
CREATE INDEX provider_maintenance_item_queue
    ON provider_maintenance_items (next_at, id) WHERE state IN ('queued', 'running');
CREATE INDEX provider_maintenance_item_lease
    ON provider_maintenance_items (lease_until) WHERE lease IS NOT NULL;
CREATE INDEX provider_maintenance_item_account
    ON provider_maintenance_items (provider_id, id DESC) WHERE operation_id IS NOT NULL;
CREATE INDEX provider_maintenance_item_replacement
    ON provider_maintenance_items (replacement_id, id DESC) WHERE replacement_id IS NOT NULL;

-- 自动任务资格基于当次观察到的凭据，执行前会重新核对。
ALTER TABLE provider_operations ADD COLUMN expected_credential_version TEXT NOT NULL DEFAULT '';
