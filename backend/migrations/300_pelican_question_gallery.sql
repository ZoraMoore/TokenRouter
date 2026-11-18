-- 保留旧计划和历史样本。旧样本缺少请求强度记录，展示时使用“未记录”。
ALTER TABLE pelican_test_plans
    ADD COLUMN question_name text NOT NULL DEFAULT '' CHECK (octet_length(question_name) <= 96),
    ADD COLUMN question_prompt text NOT NULL DEFAULT '' CHECK (octet_length(question_prompt) <= 8192),
    ADD COLUMN expected_answer text NOT NULL DEFAULT '' CHECK (octet_length(expected_answer) <= 1024),
    ADD COLUMN reasoning_effort text NOT NULL DEFAULT '' CHECK (reasoning_effort IN ('','low','medium','high','xhigh')),
    ADD COLUMN plan_revision bigint NOT NULL DEFAULT 1,
    DROP CONSTRAINT pelican_test_plans_question_kind_check,
    DROP CONSTRAINT pelican_test_plans_group_id_model_id_question_kind_key,
    ADD CHECK (question_kind IN ('pelican','candy','custom_svg','custom_text')),
    ADD UNIQUE (group_id, model_id, question_kind, question_name, reasoning_effort);

ALTER TABLE pelican_test_results
    ADD COLUMN question_name text NOT NULL DEFAULT '',
    ADD COLUMN reasoning_effort text NOT NULL DEFAULT '',
    ADD COLUMN plan_revision bigint NOT NULL DEFAULT 1,
    DROP CONSTRAINT pelican_test_results_question_kind_check,
    ADD CHECK (question_kind IN ('pelican','candy','custom_svg','custom_text'));
