-- 兑换码可要求领取者付过款，已有兑换码默认允许所有用户领取。
-- 所有服务实例升级后再开启此条件，旧二进制不会检查该字段。
ALTER TABLE redeem_codes ADD COLUMN IF NOT EXISTS requires_payment BOOLEAN NOT NULL DEFAULT FALSE;
