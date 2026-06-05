-- trading-service initial schema (spot orders + executed trades).
-- IF NOT EXISTS keeps this a no-op against the existing shared `exchange`
-- database while building it from scratch in an integration-test database.
-- Column names match gorm's snake_case so the same physical tables are used.

CREATE TABLE IF NOT EXISTS orders (
    id            bigserial      PRIMARY KEY,
    user_id       bigint         NOT NULL,
    pair          text           NOT NULL,
    side          text           NOT NULL,
    type          text           NOT NULL,
    price         numeric(30,10) NOT NULL DEFAULT 0,
    stop_price    numeric(30,10) NOT NULL DEFAULT 0,
    amount        numeric(30,10) NOT NULL,
    filled_amount numeric(30,10) NOT NULL DEFAULT 0,
    status        text           NOT NULL DEFAULT 'OPEN',
    created_at    timestamptz    NOT NULL DEFAULT now(),
    updated_at    timestamptz    NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_order_user_status ON orders (user_id, status);
CREATE INDEX IF NOT EXISTS idx_order_pair_status ON orders (pair, status);
CREATE INDEX IF NOT EXISTS idx_order_status_type ON orders (status, type);

CREATE TABLE IF NOT EXISTS trades (
    id            bigserial      PRIMARY KEY,
    pair          text           NOT NULL,
    buy_order_id  bigint         NOT NULL,
    sell_order_id bigint         NOT NULL,
    buyer_id      bigint         NOT NULL,
    seller_id     bigint         NOT NULL,
    price         numeric(30,10) NOT NULL,
    amount        numeric(30,10) NOT NULL,
    total         numeric(30,2)  NOT NULL,
    buyer_fee     numeric(30,10) NOT NULL DEFAULT 0,
    seller_fee    numeric(30,10) NOT NULL DEFAULT 0,
    created_at    timestamptz    NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_trades_pair ON trades (pair);
