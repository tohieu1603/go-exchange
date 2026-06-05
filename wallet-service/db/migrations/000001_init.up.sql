-- wallet-service initial schema.
-- IF NOT EXISTS keeps this a no-op against the existing shared `exchange`
-- database (whose tables gorm AutoMigrate already created), while still building
-- the schema from scratch in an isolated integration-test database. Column names
-- match gorm's snake_case so the same physical tables are used.

CREATE TABLE IF NOT EXISTS wallets (
    id             bigserial      PRIMARY KEY,
    user_id        bigint         NOT NULL,
    currency       text           NOT NULL,
    balance        numeric(30,10) NOT NULL DEFAULT 0,
    locked_balance numeric(30,10) NOT NULL DEFAULT 0,
    updated_at     timestamptz    NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_currency ON wallets (user_id, currency);

CREATE TABLE IF NOT EXISTS deposits (
    id            bigserial      PRIMARY KEY,
    user_id       bigint         NOT NULL,
    amount        numeric(30,2)  NOT NULL,
    amount_usdt   numeric(30,8)  NOT NULL DEFAULT 0,
    exchange_rate numeric(20,2)  NOT NULL DEFAULT 0,
    currency      text           NOT NULL DEFAULT 'VND',
    method        text           NOT NULL DEFAULT 'BANK_TRANSFER',
    status        text           NOT NULL DEFAULT 'PENDING',
    order_code    text           NOT NULL DEFAULT '',
    qr_code_url   text           NOT NULL DEFAULT '',
    sepay_ref     text           NOT NULL DEFAULT '',
    created_at    timestamptz    NOT NULL DEFAULT now(),
    updated_at    timestamptz    NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_deposits_user ON deposits (user_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_deposits_order_code ON deposits (order_code);

CREATE TABLE IF NOT EXISTS withdrawals (
    id           bigserial     PRIMARY KEY,
    user_id      bigint        NOT NULL,
    amount       numeric(30,2) NOT NULL,
    currency     text          NOT NULL DEFAULT 'VND',
    bank_code    text          NOT NULL,
    bank_account text          NOT NULL,
    account_name text          NOT NULL,
    status       text          NOT NULL DEFAULT 'PENDING',
    admin_note   text          NOT NULL DEFAULT '',
    created_at   timestamptz   NOT NULL DEFAULT now(),
    updated_at   timestamptz   NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_withdrawals_user ON withdrawals (user_id);
CREATE INDEX IF NOT EXISTS idx_withdrawals_status ON withdrawals (status);
