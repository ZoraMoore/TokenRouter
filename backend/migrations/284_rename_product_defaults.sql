-- 仅更新仍使用旧产品名的展示设置，保留自定义品牌和历史业务记录。
UPDATE settings
SET value = 'TokenRouter', updated_at = NOW()
WHERE key IN (
    'site_name', 'site_name_zh', 'site_name_en',
    'site_title_zh', 'site_title_en',
    'smtp_from_name', 'payment_product_name_prefix'
)
AND LOWER(BTRIM(value, E' \t\r\n\f\013')) = 'sub2api';
