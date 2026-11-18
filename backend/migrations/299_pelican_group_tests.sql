-- 每个分组可为同一模型分别设置动画题和糖果题。账号为空时由分组调度选号。
CREATE TABLE IF NOT EXISTS pelican_test_plans (
    id              BIGSERIAL PRIMARY KEY,
    group_id        BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    provider_id     BIGINT REFERENCES providers(id) ON DELETE CASCADE,
    model_id        TEXT NOT NULL CHECK (octet_length(model_id) BETWEEN 1 AND 128),
    question_kind   TEXT NOT NULL CHECK (question_kind IN ('pelican', 'candy')),
    cron_expression TEXT NOT NULL CHECK (octet_length(cron_expression) BETWEEN 1 AND 128),
    enabled         BOOLEAN NOT NULL,
    next_run_at     TIMESTAMPTZ,
    last_run_at     TIMESTAMPTZ,
    queued_at       TIMESTAMPTZ,
    lease_token     TEXT,
    lease_until     TIMESTAMPTZ,
    UNIQUE (group_id, model_id, question_kind)
);

CREATE INDEX IF NOT EXISTS idx_pelican_plans_due ON pelican_test_plans(next_run_at) WHERE enabled;
CREATE INDEX IF NOT EXISTS idx_pelican_plans_queued ON pelican_test_plans(queued_at) WHERE queued_at IS NOT NULL;

-- target_provider_id 记录该轮配置的账号，provider_id 记录实际作答账号。
-- 管理员切换选号方式后，用户页按当轮配置匹配历史样本。
CREATE TABLE IF NOT EXISTS pelican_test_results (
    id                 BIGSERIAL PRIMARY KEY,
    plan_id            BIGINT NOT NULL REFERENCES pelican_test_plans(id) ON DELETE CASCADE,
    target_provider_id BIGINT,
    provider_id        BIGINT REFERENCES providers(id) ON DELETE SET NULL,
    model_id           TEXT NOT NULL,
    question_kind      TEXT NOT NULL CHECK (question_kind IN ('pelican', 'candy')),
    status             TEXT NOT NULL CHECK (status IN ('pending_review', 'passed', 'suspected', 'inconclusive')),
    detail_code        TEXT NOT NULL DEFAULT '',
    prompt             TEXT NOT NULL,
    criteria           JSONB NOT NULL,
    answer             TEXT NOT NULL,
    tested_at          TIMESTAMPTZ NOT NULL,
    latency_ms         BIGINT NOT NULL CHECK (latency_ms >= 0),
    CHECK (target_provider_id IS NULL OR provider_id IS NULL OR target_provider_id = provider_id)
);

CREATE INDEX IF NOT EXISTS idx_pelican_results_plan_time ON pelican_test_results(plan_id, tested_at DESC, id DESC);
