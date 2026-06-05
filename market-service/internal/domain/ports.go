package domain

import "context"

// CandleRepository persists and queries completed candles.
type CandleRepository interface {
	// Upsert writes a completed candle, overwriting an existing bar with the
	// same (pair, interval, open_time).
	Upsert(ctx context.Context, c Candle) error
	// UpsertBatch writes many candles (used by historical backfill).
	UpsertBatch(ctx context.Context, candles []Candle) error
	// Query returns up to limit candles for (pair, interval) in ASCENDING time.
	Query(ctx context.Context, pair, interval string, limit int) ([]Candle, error)
}
