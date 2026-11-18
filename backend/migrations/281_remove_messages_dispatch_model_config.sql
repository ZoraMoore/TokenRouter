-- 移除 Messages 专用模型覆盖，分组映射统一使用 routing_policy.model_mapping。
ALTER TABLE groups DROP COLUMN IF EXISTS messages_dispatch_model_config;
