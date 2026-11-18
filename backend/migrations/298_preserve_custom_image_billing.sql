-- 图片独立倍率归属价格配置，图片价卡继续使用管理员填写的单价。
ALTER TABLE pricing_configs
 ADD COLUMN image_rate_independent BOOLEAN NOT NULL DEFAULT FALSE,
 ADD COLUMN image_rate_multiplier NUMERIC(10,4) NOT NULL DEFAULT 1 CHECK (image_rate_multiplier >= 0);

UPDATE pricing_configs pc SET
 image_rate_independent=COALESCE((archive.original_group->>'image_rate_independent')::BOOLEAN,FALSE),
 image_rate_multiplier=COALESCE((archive.original_group->>'image_rate_multiplier')::NUMERIC,1)
FROM pricing_config_groups binding
JOIN groups g ON g.id=binding.group_id AND g.deleted_at IS NULL
JOIN platform_independent_group_archive archive ON archive.group_id=g.id
WHERE pc.id=binding.pricing_config_id;
