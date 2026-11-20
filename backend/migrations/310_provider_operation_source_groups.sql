-- 单组任务继续读取 group_id，多组任务在 group_ids 保存完整来源列表。
ALTER TABLE provider_operations
    ADD COLUMN IF NOT EXISTS group_ids BIGINT[] NOT NULL DEFAULT ARRAY[]::BIGINT[];
