-- 系列默认映射下线，只清理对应 JSON 字段，保留精确模型覆盖和其它分组配置。
UPDATE groups
SET messages_dispatch_model_config = messages_dispatch_model_config
    - ARRAY['opus_mapped_model', 'sonnet_mapped_model', 'haiku_mapped_model']
WHERE messages_dispatch_model_config ?| ARRAY['opus_mapped_model', 'sonnet_mapped_model', 'haiku_mapped_model'];

COMMENT ON COLUMN groups.messages_dispatch_model_config IS 'Messages 精确模型映射配置';
