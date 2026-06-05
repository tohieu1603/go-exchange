// Package marketstore is the Redis adapter for usecase.MarketStore — the
// ticker/depth/trade reads populated by the price feed and the trading service.
package marketstore

import (
	"context"
	"encoding/json"

	"github.com/cryptox/market-service/internal/usecase"
	"github.com/redis/go-redis/v9"
)

type Store struct{ rdb *redis.Client }

func NewStore(rdb *redis.Client) *Store { return &Store{rdb: rdb} }

func (s *Store) Price(ctx context.Context, pair string) float64 {
	v, err := s.rdb.Get(ctx, "price:"+pair).Float64()
	if err != nil {
		return 0
	}
	return v
}

func (s *Store) Ticker(ctx context.Context, pair string) (price, change24h, vol24h float64) {
	price, _ = s.rdb.Get(ctx, "price:"+pair).Float64()
	change24h, _ = s.rdb.Get(ctx, "change24h:"+pair).Float64()
	vol24h, _ = s.rdb.Get(ctx, "vol24h:"+pair).Float64()
	return
}

func (s *Store) Depth(ctx context.Context, pair string, limit int) (usecase.Depth, error) {
	empty := usecase.Depth{Bids: []usecase.DepthLevel{}, Asks: []usecase.DepthLevel{}}
	raw, err := s.rdb.Get(ctx, "depth:"+pair).Bytes()
	if err != nil {
		return empty, nil // not yet published → empty book
	}
	var depth usecase.Depth
	if json.Unmarshal(raw, &depth) != nil {
		return empty, nil
	}
	if len(depth.Bids) > limit {
		depth.Bids = depth.Bids[:limit]
	}
	if len(depth.Asks) > limit {
		depth.Asks = depth.Asks[:limit]
	}
	return depth, nil
}

func (s *Store) RecentTrades(ctx context.Context, pair string, limit int) ([]map[string]interface{}, error) {
	cached, err := s.rdb.LRange(ctx, "recent_trades:"+pair, 0, int64(limit-1)).Result()
	if err != nil || len(cached) == 0 {
		return []map[string]interface{}{}, nil
	}
	trades := make([]map[string]interface{}, 0, len(cached))
	for _, raw := range cached {
		var t map[string]interface{}
		if json.Unmarshal([]byte(raw), &t) == nil {
			trades = append(trades, t)
		}
	}
	return trades, nil
}
