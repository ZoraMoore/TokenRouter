-- 创作任务保存执行平台，避免混合分组或账号后续调整改变供应商语义。
ALTER TABLE creative_runs ADD COLUMN IF NOT EXISTS provider VARCHAR(32) NOT NULL DEFAULT '';
UPDATE creative_runs r SET provider = a.platform
FROM accounts a WHERE r.account_id = a.id AND r.provider = '';
