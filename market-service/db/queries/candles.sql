-- name: UpsertCandle :exec
-- Write a completed candle, overwriting an existing bar with the same
-- (pair, interval, open_time).
INSERT INTO candles (pair, interval, open_time, open, high, low, close, volume)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (pair, interval, open_time)
DO UPDATE SET open = EXCLUDED.open, high = EXCLUDED.high, low = EXCLUDED.low,
              close = EXCLUDED.close, volume = EXCLUDED.volume;

-- name: QueryCandles :many
-- The most-recent `limit` bars for (pair, interval), returned in ASCENDING
-- open_time order (the inner query takes the newest rows, the outer re-sorts).
SELECT pair, interval, open_time, open, high, low, close, volume
FROM (
    SELECT pair, interval, open_time, open, high, low, close, volume
    FROM candles
    WHERE pair = $1 AND interval = $2
    ORDER BY open_time DESC
    LIMIT $3
) sub
ORDER BY open_time ASC;
