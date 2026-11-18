-- 属性配置只提供展示和客户端导出元数据，与价格配置及网关准入独立。
CREATE TABLE IF NOT EXISTS model_attribute_configs (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    rules JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(rules) = 'array'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 唯一约束在并发保存时仍保证每个分组至多关联一份属性配置。
CREATE TABLE IF NOT EXISTS model_attribute_config_groups (
    config_id BIGINT NOT NULL REFERENCES model_attribute_configs(id) ON DELETE CASCADE,
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    PRIMARY KEY (config_id, group_id),
    UNIQUE (group_id)
);
