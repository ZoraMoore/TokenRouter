-- 已有计划的站点继续提供网页展示。API Key 读取作品由管理员开启。
CREATE TABLE pelican_showcase_settings (
    singleton      BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    enabled        BOOLEAN NOT NULL DEFAULT FALSE,
    api_enabled    BOOLEAN NOT NULL DEFAULT FALSE,
    max_items      INTEGER NOT NULL DEFAULT 20 CHECK (max_items BETWEEN 1 AND 100),
    auto_cleanup   BOOLEAN NOT NULL DEFAULT TRUE,
    retention_days INTEGER NOT NULL DEFAULT 7 CHECK (retention_days BETWEEN 1 AND 90)
);

INSERT INTO pelican_showcase_settings (enabled)
SELECT EXISTS (SELECT 1 FROM pelican_test_plans);

-- 用户作品和管理员历史共用一份作答。作品退出画廊后仍可保留在计划历史中。
ALTER TABLE pelican_test_results ADD COLUMN published BOOLEAN NOT NULL DEFAULT FALSE;

WITH ranked AS (
    SELECT r.id, row_number() OVER (
        PARTITION BY p.group_id ORDER BY r.tested_at DESC, r.id DESC
    ) AS position
    FROM pelican_test_results r
    JOIN pelican_test_plans p ON p.id = r.plan_id
    WHERE r.plan_revision = p.plan_revision
      AND r.model_id = p.model_id AND r.question_kind = p.question_kind
      AND r.question_name = p.question_name
      AND r.target_provider_id IS NOT DISTINCT FROM p.provider_id
      AND r.question_kind IN ('pelican', 'custom_svg')
      AND r.status IN ('pending_review', 'passed', 'suspected') AND r.answer <> ''
      AND r.tested_at >= now() - interval '168 hours'
)
UPDATE pelican_test_results r SET published = TRUE
FROM ranked WHERE ranked.id = r.id AND ranked.position <= 20;

CREATE INDEX idx_pelican_published_plan_time
    ON pelican_test_results (plan_id, tested_at DESC, id DESC) WHERE published;
