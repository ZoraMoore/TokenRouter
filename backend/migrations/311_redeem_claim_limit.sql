CREATE TABLE IF NOT EXISTS redeem_claim_settings (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    starts_at TIMESTAMPTZ,
    ends_at TIMESTAMPTZ,
    tracking_since TIMESTAMPTZ NOT NULL DEFAULT statement_timestamp(),
    CHECK (
        (starts_at IS NULL AND ends_at IS NULL AND NOT enabled)
        OR (starts_at IS NOT NULL AND ends_at IS NOT NULL AND starts_at < ends_at)
    )
);

INSERT INTO redeem_claim_settings (singleton) VALUES (TRUE)
ON CONFLICT (singleton) DO NOTHING;

-- 用户主动兑换的记录供管理员调整限领时段后重新核对。
CREATE TABLE IF NOT EXISTS redeem_user_claims (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    claimed_at TIMESTAMPTZ NOT NULL,
    redeem_code_id BIGINT NOT NULL,
    PRIMARY KEY (user_id, claimed_at, redeem_code_id)
);
