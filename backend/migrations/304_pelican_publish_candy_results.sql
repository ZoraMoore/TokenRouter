-- 糖果题按当前计划收录，绘图和糖果各自使用分组上限。
WITH ranked AS (
    SELECT r.id, row_number() OVER (
        PARTITION BY p.group_id ORDER BY r.tested_at DESC, r.id DESC
    ) AS position
    FROM pelican_test_results r
    JOIN pelican_test_plans p ON p.id = r.plan_id
    CROSS JOIN pelican_showcase_settings s
    WHERE s.singleton AND r.question_kind = 'candy' AND p.question_kind = 'candy'
      AND r.plan_revision = p.plan_revision AND r.model_id = p.model_id AND r.question_name = p.question_name
      AND r.target_provider_id IS NOT DISTINCT FROM p.provider_id
      AND r.status IN ('passed', 'suspected') AND r.answer ~ '[^[:space:]]'
      AND (NOT s.auto_cleanup OR r.tested_at >= statement_timestamp() - s.retention_days * interval '24 hours')
)
UPDATE pelican_test_results r SET published = true
FROM ranked, pelican_showcase_settings s
WHERE r.id = ranked.id AND s.singleton AND ranked.position <= s.max_items;
