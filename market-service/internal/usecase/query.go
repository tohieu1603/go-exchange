package usecase

import (
	"context"

	"github.com/cryptox/market-service/internal/domain"
	"github.com/cryptox/shared/types"
)

// CandleQuery serves historical candle series (cache-through to the repository).
type CandleQuery struct {
	repo  domain.CandleRepository
	cache CandleCache
}

func NewCandleQuery(repo domain.CandleRepository, cache CandleCache) *CandleQuery {
	return &CandleQuery{repo: repo, cache: cache}
}

func (q *CandleQuery) GetCandles(ctx context.Context, pair, interval string, limit int) ([]domain.CandleWS, error) {
	if cached, ok := q.cache.Get(ctx, pair, interval, limit); ok {
		return cached, nil
	}
	candles, err := q.repo.Query(ctx, pair, interval, limit)
	if err != nil {
		return nil, err
	}
	ws := make([]domain.CandleWS, len(candles))
	for i, c := range candles {
		ws[i] = c.ToWS()
	}
	q.cache.Set(ctx, pair, interval, limit, ws)
	return ws, nil
}

// MarketQuery serves ticker / depth / trade / price reads from the market store.
type MarketQuery struct {
	store MarketStore
	coins []types.Coin
}

func NewMarketQuery(store MarketStore) *MarketQuery {
	return &MarketQuery{store: store, coins: types.DefaultCoins}
}

// AllTickers returns one ticker per configured coin.
func (q *MarketQuery) AllTickers(ctx context.Context) []Ticker {
	tickers := make([]Ticker, 0, len(q.coins))
	for _, coin := range q.coins {
		pair := coin.Symbol + "_USDT"
		price, change, vol := q.store.Ticker(ctx, pair)
		assetType := coin.AssetType
		if assetType == "" {
			assetType = "crypto"
		}
		tickers = append(tickers, Ticker{
			Pair: pair, Symbol: coin.Symbol, Name: coin.Name, AssetType: assetType,
			Price: price, Change24h: change, Volume24h: vol,
		})
	}
	return tickers
}

func (q *MarketQuery) Price(ctx context.Context, pair string) float64 {
	return q.store.Price(ctx, pair)
}

func (q *MarketQuery) Depth(ctx context.Context, pair string, limit int) (Depth, error) {
	return q.store.Depth(ctx, pair, limit)
}

func (q *MarketQuery) RecentTrades(ctx context.Context, pair string, limit int) ([]map[string]interface{}, error) {
	return q.store.RecentTrades(ctx, pair, limit)
}
