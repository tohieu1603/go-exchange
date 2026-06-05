-- futures-service initial schema (perpetual positions, funding rates/payments).
-- IF NOT EXISTS keeps this a no-op against the existing shared `exchange`
-- database while building it from scratch in an integration-test database.
-- Column names match gorm's snake_case (note: unrealized_pn_l for UnrealizedPnL).

CREATE TABLE IF NOT EXISTS futures_positions (
    id                bigserial      PRIMARY KEY,
    user_id           bigint         NOT NULL,
    pair              text           NOT NULL,
    side              text           NOT NULL,
    leverage          integer        NOT NULL,
    entry_price       numeric(30,10) NOT NULL DEFAULT 0,
    mark_price        numeric(30,10) NOT NULL DEFAULT 0,
    size              numeric(30,10) NOT NULL DEFAULT 0,
    margin            numeric(30,2)  NOT NULL DEFAULT 0,
    unrealized_pn_l   numeric(30,2)  NOT NULL DEFAULT 0,
    liquidation_price numeric(30,10) NOT NULL DEFAULT 0,
    take_profit       numeric(30,10) NOT NULL DEFAULT 0,
    stop_loss         numeric(30,10) NOT NULL DEFAULT 0,
    status            text           NOT NULL DEFAULT 'OPEN',
    created_at        timestamptz    NOT NULL DEFAULT now(),
    closed_at         timestamptz
);
CREATE INDEX IF NOT EXISTS idx_pos_user ON futures_positions (user_id);
CREATE INDEX IF NOT EXISTS idx_pos_pair ON futures_positions (pair);
CREATE INDEX IF NOT EXISTS idx_pos_status ON futures_positions (status);

CREATE TABLE IF NOT EXISTS funding_rates (
    id          bigserial      PRIMARY KEY,
    pair        text           NOT NULL,
    rate        numeric(10,8)  NOT NULL,
    index_price numeric(30,10) NOT NULL DEFAULT 0,
    mark_price  numeric(30,10) NOT NULL DEFAULT 0,
    interval    text           NOT NULL DEFAULT '8h',
    settled_at  timestamptz    NOT NULL,
    created_at  timestamptz    NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_fr_pair_time ON funding_rates (pair, settled_at);

CREATE TABLE IF NOT EXISTS funding_payments (
    id              bigserial      PRIMARY KEY,
    position_id     bigint         NOT NULL,
    user_id         bigint         NOT NULL,
    funding_rate_id bigint         NOT NULL,
    pair            text           NOT NULL,
    side            text           NOT NULL,
    notional        numeric(30,10) NOT NULL,
    rate            numeric(10,8)  NOT NULL,
    amount          numeric(30,10) NOT NULL,
    created_at      timestamptz    NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_fp_pos_rate ON funding_payments (position_id, funding_rate_id);
CREATE INDEX IF NOT EXISTS idx_fpay_user ON funding_payments (user_id);
