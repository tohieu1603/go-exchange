// Package usecase holds market-data application logic: candle aggregation,
// candle queries, and ticker/depth/trade reads. Depends on domain + the outbound
// ports declared here.
package usecase

import (
	"context"

	"github.com/cryptox/market-service/internal/domain"
)

// Broadcaster pushes real-time payloads to a WebSocket channel.
type Broadcaster interface {
	Broadcast(channel string, data interface{})
}

// CandleCache is a short-TTL cache for rendered candle series.
type CandleCache interface {
	Get(ctx context.Context, pair, interval string, limit int) ([]domain.CandleWS, bool)
	Set(ctx context.Context, pair, interval string, limit int, candles []domain.CandleWS)
	Invalidate(ctx context.Context, pair, interval string)
}

// Ticker is a presentation-neutral market ticker.
type Ticker struct {
	Pair      string
	Symbol    string
	Name      string
	AssetType string
	Price     float64
	Change24h float64
	Volume24h float64
}

// DepthLevel / Depth model an order-book snapshot.
type DepthLevel struct {
	Price  float64 `json:"price"`
	Amount float64 `json:"amount"`
}
type Depth struct {
	Bids []DepthLevel `json:"bids"`
	Asks []DepthLevel `json:"asks"`
}

// MarketStore exposes the Redis-cached market reads written by the price feed
// and the trading service.
type MarketStore interface {
	Price(ctx context.Context, pair string) float64
	Ticker(ctx context.Context, pair string) (price, change24h, vol24h float64)
	Depth(ctx context.Context, pair string, limit int) (Depth, error)
	RecentTrades(ctx context.Context, pair string, limit int) ([]map[string]interface{}, error)
}

// RateProvider supplies the VND-per-USDT exchange rate.
type RateProvider interface {
	VNDRate() float64
}
