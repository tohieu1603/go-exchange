-- market-service initial schema (completed OHLCV candles).
-- IF NOT EXISTS keeps this a no-op against the existing shared `exchange`
-- database while building it from scratch in an integration-test database.

CREATE TABLE IF NOT EXISTS candles (
    id        bigserial      PRIMARY KEY,
    pair      text           NOT NULL,
    interval  text           NOT NULL,
    open_time timestamptz    NOT NULL,
    open      numeric(30,10) NOT NULL DEFAULT 0,
    high      numeric(30,10) NOT NULL DEFAULT 0,
    low       numeric(30,10) NOT NULL DEFAULT 0,
    close     numeric(30,10) NOT NULL DEFAULT 0,
    volume    numeric(30,10) NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_candle ON candles (pair, interval, open_time);
