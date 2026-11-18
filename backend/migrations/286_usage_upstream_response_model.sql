-- 保存上游原始模型声明；历史记录不从请求模型推测回填，兼容旧版本写入。
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS upstream_response_model VARCHAR(200);
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS upstream_model_mismatch BOOLEAN;
