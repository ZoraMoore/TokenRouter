-- 初测时冻结两层模型映射，供定时测智核对实际作答账号。
ALTER TABLE pelican_candy_attempts
    ADD COLUMN request_model TEXT NOT NULL DEFAULT '',
    ADD COLUMN actual_model TEXT NOT NULL DEFAULT '';

ALTER TABLE pelican_test_results
    DROP CONSTRAINT pelican_test_results_schedule_action_check,
    ADD CHECK (schedule_action IN ('', 'disabled', 'already_disabled', 'account_changed', 'recheck_queued'));

-- 分组样本按画廊保留期清理。复检独立保存来源编号和初测回答。
ALTER TABLE provider_maintenance_runs
    ALTER COLUMN plan_id DROP NOT NULL,
    ADD COLUMN pelican_result_id BIGINT UNIQUE,
    DROP CONSTRAINT provider_maintenance_runs_trigger_check,
    ADD CHECK (
        (trigger IN ('manual', 'schedule') AND plan_id IS NOT NULL AND pelican_result_id IS NULL)
        OR (trigger = 'pelican' AND plan_id IS NULL AND pelican_result_id IS NOT NULL AND pelican_result_id > 0)
    );
