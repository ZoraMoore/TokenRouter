-- 自动停调按计划启用。计划属于固定分组。
ALTER TABLE pelican_test_plans
    ADD COLUMN suspend_on_wrong_answer BOOLEAN NOT NULL DEFAULT FALSE,
    ADD CONSTRAINT pelican_candy_auto_target CHECK (
        NOT suspend_on_wrong_answer OR (
            question_kind = 'candy' AND provider_id IS NULL AND expected_answer IN ('', '21')
        )
    );

ALTER TABLE pelican_test_results
    ADD COLUMN round_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN schedule_action TEXT NOT NULL DEFAULT ''
        CHECK (schedule_action IN ('', 'disabled', 'already_disabled', 'account_changed'));

-- 同一轮只有一个候选快照。每次尝试完成后，下一个 worker 从库中继续。
CREATE TABLE pelican_candy_rounds (
    plan_id BIGINT PRIMARY KEY REFERENCES pelican_test_plans(id) ON DELETE CASCADE,
    id TEXT UNIQUE NOT NULL,
    candidate_ids BIGINT[] NOT NULL,
    retry_at TIMESTAMPTZ,
    busy_since TIMESTAMPTZ
);

-- 保留实际作答账号和处置，供修改计划或删除样本后审计。
CREATE TABLE pelican_candy_attempts (
    round_id TEXT NOT NULL,
    provider_id BIGINT NOT NULL,
    plan_id BIGINT NOT NULL,
    group_id BIGINT NOT NULL,
    provider_revision BIGINT NOT NULL,
    group_updated_at TIMESTAMPTZ NOT NULL,
    membership_created_at TIMESTAMPTZ NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT statement_timestamp(),
    finished_at TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT '',
    detail_code TEXT NOT NULL DEFAULT '',
    schedule_action TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (round_id, provider_id)
);

-- 热路径的用量和活跃时间更新可以发生在测试过程中。版本记录账号配置和人工调度的变动。
ALTER TABLE providers ADD COLUMN pelican_candy_revision BIGINT NOT NULL DEFAULT 1;

CREATE FUNCTION bump_pelican_candy_provider_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.pelican_candy_revision := OLD.pelican_candy_revision + 1;
    RETURN NEW;
END;
$$;

CREATE TRIGGER pelican_candy_provider_revision
BEFORE UPDATE OF credentials, proxy_id, parent_provider_id, platform, type, status, schedulable, deleted_at ON providers
FOR EACH ROW WHEN (
    (OLD.credentials, OLD.proxy_id, OLD.parent_provider_id, OLD.platform, OLD.type, OLD.status, OLD.schedulable, OLD.deleted_at)
    IS DISTINCT FROM
    (NEW.credentials, NEW.proxy_id, NEW.parent_provider_id, NEW.platform, NEW.type, NEW.status, NEW.schedulable, NEW.deleted_at)
)
EXECUTE FUNCTION bump_pelican_candy_provider_revision();
