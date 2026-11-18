-- 为出站 TLS 指纹模板增加来源和验证证据。
-- 历史及手工模板保持草稿状态，不能静默进入已验证运行时模板池。

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE tls_fingerprint_profiles
    ADD COLUMN IF NOT EXISTS signature_algorithms_cert JSONB,
    ADD COLUMN IF NOT EXISTS verification JSONB NOT NULL DEFAULT '{"source_kind":"legacy","validation_status":"draft","validator_version":0}'::jsonb;

COMMENT ON COLUMN tls_fingerprint_profiles.verification IS
    'Controlled-capture provenance and TLS replay/target-handshake validation evidence; never stores credentials or raw ClientHello secrets';

COMMENT ON COLUMN tls_fingerprint_profiles.signature_algorithms_cert IS
    'TLS extension 50 certificate signature algorithms, kept separate from extension 13';
