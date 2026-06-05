-- auth-service initial schema. IF NOT EXISTS keeps this a no-op against the
-- existing shared `exchange` database (gorm AutoMigrate already created these)
-- while building it from scratch in an integration-test database. Column and
-- table names match gorm's naming exactly (e.g. user_bonus, user_volume30ds,
-- is2_fa) so the same physical tables are reused.

CREATE TABLE IF NOT EXISTS users (
    id                bigserial   PRIMARY KEY,
    email             text        NOT NULL,
    password_hash     text        NOT NULL DEFAULT '',
    full_name         text        NOT NULL DEFAULT '',
    phone             text        NOT NULL DEFAULT '',
    kyc_status        text        NOT NULL DEFAULT 'NONE',
    is2_fa            boolean     NOT NULL DEFAULT false,
    two_fa_secret     text        NOT NULL DEFAULT '',
    role              text        NOT NULL DEFAULT 'USER',
    email_verified    boolean     NOT NULL DEFAULT false,
    kyc_step          integer     NOT NULL DEFAULT 0,
    is_locked         boolean     NOT NULL DEFAULT false,
    lock_reason       text        NOT NULL DEFAULT '',
    last_login_ip     text        NOT NULL DEFAULT '',
    register_ip       text        NOT NULL DEFAULT '',
    google_sub        text        NOT NULL DEFAULT '',
    avatar_url        text        NOT NULL DEFAULT '',
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email ON users (email);
CREATE INDEX IF NOT EXISTS idx_users_google_sub ON users (google_sub);

CREATE TABLE IF NOT EXISTS api_keys (
    id           bigserial   PRIMARY KEY,
    user_id      bigint      NOT NULL,
    label        text        NOT NULL DEFAULT '',
    key_id       text        NOT NULL,
    secret_hash  text        NOT NULL DEFAULT '',
    permissions  text        NOT NULL DEFAULT 'read,trade',
    ip_whitelist text        NOT NULL DEFAULT '',
    last_used_at timestamptz,
    last_used_ip text        NOT NULL DEFAULT '',
    expires_at   timestamptz,
    revoked_at   timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_api_keys_user ON api_keys (user_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_key_id ON api_keys (key_id);

CREATE TABLE IF NOT EXISTS audit_logs (
    id         bigserial   PRIMARY KEY,
    user_id    bigint      NOT NULL DEFAULT 0,
    email      text        NOT NULL DEFAULT '',
    action     text        NOT NULL,
    outcome    text        NOT NULL,
    ip         text        NOT NULL DEFAULT '',
    user_agent text        NOT NULL DEFAULT '',
    device_id  text        NOT NULL DEFAULT '',
    new_device boolean     NOT NULL DEFAULT false,
    detail     text        NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_audit_user_time ON audit_logs (user_id, created_at);
CREATE INDEX IF NOT EXISTS idx_audit_user_device ON audit_logs (user_id, device_id);
CREATE INDEX IF NOT EXISTS idx_audit_action ON audit_logs (action);

CREATE TABLE IF NOT EXISTS bonus_promotions (
    id               bigserial     PRIMARY KEY,
    name             text          NOT NULL,
    description      text          NOT NULL DEFAULT '',
    bonus_percent    numeric(5,2)  NOT NULL,
    max_bonus_amount numeric(20,2) NOT NULL DEFAULT 0,
    target_type      text          NOT NULL,
    target_user_ids  text          NOT NULL DEFAULT '',
    trigger_type     text          NOT NULL,
    min_deposit      numeric(20,2) NOT NULL DEFAULT 0,
    is_active        boolean       NOT NULL DEFAULT true,
    start_at         timestamptz   NOT NULL DEFAULT now(),
    end_at           timestamptz   NOT NULL DEFAULT now(),
    created_at       timestamptz   NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS user_bonus (
    id           bigserial     PRIMARY KEY,
    user_id      bigint        NOT NULL,
    promotion_id bigint        NOT NULL,
    deposit_id   bigint        NOT NULL DEFAULT 0,
    bonus_amount numeric(20,2) NOT NULL,
    used_amount  numeric(20,2) NOT NULL DEFAULT 0,
    status       text          NOT NULL DEFAULT 'ACTIVE',
    created_at   timestamptz   NOT NULL DEFAULT now(),
    expires_at   timestamptz
);
CREATE INDEX IF NOT EXISTS idx_user_bonus_user ON user_bonus (user_id);
CREATE INDEX IF NOT EXISTS idx_user_bonus_promo ON user_bonus (promotion_id);

CREATE TABLE IF NOT EXISTS fee_tiers (
    id          bigserial     PRIMARY KEY,
    level       integer       NOT NULL,
    name        text          NOT NULL,
    min_volume  numeric(30,2) NOT NULL,
    maker_fee   numeric(8,6)  NOT NULL,
    taker_fee   numeric(8,6)  NOT NULL,
    description text          NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_fee_tiers_level ON fee_tiers (level);

CREATE TABLE IF NOT EXISTS user_volume30ds (
    user_id    bigint        PRIMARY KEY,
    volume     numeric(30,2) NOT NULL DEFAULT 0,
    tier_level integer       NOT NULL DEFAULT 0,
    updated_at timestamptz   NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS fraud_logs (
    id          bigserial   PRIMARY KEY,
    user_ids    text        NOT NULL,
    fraud_type  text        NOT NULL,
    description text        NOT NULL DEFAULT '',
    evidence    text        NOT NULL DEFAULT '',
    action      text        NOT NULL DEFAULT 'FLAGGED',
    admin_note  text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS kyc_documents (
    id         bigserial   PRIMARY KEY,
    user_id    bigint      NOT NULL,
    doc_type   text        NOT NULL,
    file_path  text        NOT NULL,
    status     text        NOT NULL DEFAULT 'PENDING',
    admin_note text        NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_kyc_documents_user ON kyc_documents (user_id);

CREATE TABLE IF NOT EXISTS kyc_profiles (
    id            bigserial   PRIMARY KEY,
    user_id       bigint      NOT NULL,
    first_name    text        NOT NULL,
    last_name     text        NOT NULL,
    date_of_birth text        NOT NULL DEFAULT '',
    phone         text        NOT NULL DEFAULT '',
    address       text        NOT NULL DEFAULT '',
    ward          text        NOT NULL DEFAULT '',
    district      text        NOT NULL DEFAULT '',
    city          text        NOT NULL DEFAULT '',
    postal_code   text        NOT NULL DEFAULT '',
    country       text        NOT NULL DEFAULT 'VN',
    occupation    text        NOT NULL DEFAULT '',
    income        text        NOT NULL DEFAULT '',
    trading_exp   text        NOT NULL DEFAULT '',
    purpose       text        NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_kyc_profiles_user ON kyc_profiles (user_id);

CREATE TABLE IF NOT EXISTS referral_codes (
    id          bigserial   PRIMARY KEY,
    user_id     bigint      NOT NULL,
    code        text        NOT NULL,
    is_default  boolean     NOT NULL DEFAULT true,
    usage_count integer     NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_referral_codes_user ON referral_codes (user_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_referral_codes_code ON referral_codes (code);

CREATE TABLE IF NOT EXISTS referrals (
    id          bigserial   PRIMARY KEY,
    referrer_id bigint      NOT NULL,
    referee_id  bigint      NOT NULL,
    code        text        NOT NULL,
    tier        integer     NOT NULL DEFAULT 1,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_referrals_referrer ON referrals (referrer_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_referrals_referee ON referrals (referee_id);

CREATE TABLE IF NOT EXISTS referral_commissions (
    id          bigserial      PRIMARY KEY,
    referrer_id bigint         NOT NULL,
    referee_id  bigint         NOT NULL,
    trade_id    bigint         NOT NULL,
    currency    text           NOT NULL,
    fee_amount  numeric(30,10) NOT NULL,
    rate        numeric(6,4)   NOT NULL,
    commission  numeric(30,10) NOT NULL,
    created_at  timestamptz    NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_ref_comm_referrer ON referral_commissions (referrer_id);
CREATE INDEX IF NOT EXISTS idx_ref_comm_referee ON referral_commissions (referee_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_ref_trade ON referral_commissions (trade_id);

CREATE TABLE IF NOT EXISTS refresh_tokens (
    id             bigserial   PRIMARY KEY,
    user_id        bigint      NOT NULL,
    token_hash     text        NOT NULL,
    family_id      text        NOT NULL,
    parent_id      bigint,
    user_agent     text        NOT NULL DEFAULT '',
    ip             text        NOT NULL DEFAULT '',
    issued_at      timestamptz NOT NULL,
    expires_at     timestamptz NOT NULL,
    used_at        timestamptz,
    revoked_at     timestamptz,
    revoked_reason text        NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_rt_user ON refresh_tokens (user_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_rt_token_hash ON refresh_tokens (token_hash);
CREATE INDEX IF NOT EXISTS idx_rt_family ON refresh_tokens (family_id);
CREATE INDEX IF NOT EXISTS idx_rt_expires ON refresh_tokens (expires_at);

CREATE TABLE IF NOT EXISTS user_trade_pairs (
    id          bigserial     PRIMARY KEY,
    user1_id    bigint        NOT NULL,
    user2_id    bigint        NOT NULL,
    pair        text          NOT NULL,
    trade_count integer       NOT NULL DEFAULT 0,
    total_vol   numeric(30,2) NOT NULL DEFAULT 0,
    first_trade timestamptz   NOT NULL DEFAULT now(),
    last_trade  timestamptz   NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_utp_users_pair ON user_trade_pairs (user1_id, user2_id, pair);

CREATE TABLE IF NOT EXISTS platform_settings (
    id                   bigserial    PRIMARY KEY,
    deposit_fee_percent  numeric(5,2) NOT NULL DEFAULT 0,
    withdraw_fee_percent numeric(5,2) NOT NULL DEFAULT 0,
    min_deposit          numeric(20,2) NOT NULL DEFAULT 0,
    max_deposit          numeric(20,2) NOT NULL DEFAULT 0,
    min_withdraw         numeric(20,2) NOT NULL DEFAULT 0,
    max_withdraw         numeric(20,2) NOT NULL DEFAULT 0,
    trading_fee_percent  numeric(5,4) NOT NULL DEFAULT 0,
    kyc_required         boolean      NOT NULL DEFAULT false
);

CREATE TABLE IF NOT EXISTS coins (
    id            bigserial PRIMARY KEY,
    symbol        text      NOT NULL,
    name          text      NOT NULL DEFAULT '',
    coin_gecko_id text      NOT NULL DEFAULT '',
    bybit_symbol  text      NOT NULL DEFAULT '',
    icon_url      text      NOT NULL DEFAULT '',
    is_active     boolean   NOT NULL DEFAULT true,
    sort_order    integer   NOT NULL DEFAULT 0,
    asset_type    text      NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_coins_symbol ON coins (symbol);
