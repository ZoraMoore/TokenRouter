-- 探针固定在一个账号上续接请求，不能使用自动调度或答题参数。
ALTER TABLE pelican_test_plans
    DROP CONSTRAINT pelican_test_plans_question_kind_check,
    ADD CHECK (question_kind IN ('pelican', 'candy', 'custom_svg', 'custom_text', 'state_probe')),
    ADD CONSTRAINT pelican_test_plans_state_probe_target_check CHECK (
        question_kind <> 'state_probe' OR (
            provider_id IS NOT NULL AND question_prompt = ''
            AND expected_answer = '' AND reasoning_effort = ''
        )
    );

ALTER TABLE pelican_test_results
    DROP CONSTRAINT pelican_test_results_question_kind_check,
    ADD CHECK (question_kind IN ('pelican', 'candy', 'custom_svg', 'custom_text', 'state_probe'));
