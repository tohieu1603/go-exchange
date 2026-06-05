package usecase

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/cryptox/market-service/internal/domain"
)

// guarded pairs a forming bar with its mutex; ProcessTick is called from
// multiple feed goroutines so each bar must be updated under its own lock.
type guarded struct {
	mu sync.Mutex
	lc *domain.LiveCandle
}

// CandleAggregator folds incoming price ticks into OHLCV bars across all
// intervals, persisting completed bars and streaming live updates.
type CandleAggregator struct {
	repo        domain.CandleRepository
	cache       CandleCache
	broadcaster Broadcaster
	live        sync.Map // "pair:interval" -> *guarded
}

func NewCandleAggregator(repo domain.CandleRepository, cache CandleCache, b Broadcaster) *CandleAggregator {
	return &CandleAggregator{repo: repo, cache: cache, broadcaster: b}
}

// ProcessTick updates every interval's forming bar with one price tick.
func (a *CandleAggregator) ProcessTick(pair string, price, volume float64) {
	now := time.Now().UTC()
	for _, interval := range domain.Intervals {
		boundary := domain.TruncateToInterval(now, interval)
		v, _ := a.live.LoadOrStore(pair+":"+interval, &guarded{lc: domain.NewLiveCandle(price, volume, boundary)})
		g := v.(*guarded)

		g.mu.Lock()
		completed, rolled := g.lc.Apply(price, volume, boundary)
		snapshot := g.lc.Snapshot()
		g.mu.Unlock()

		if rolled && completed != nil {
			c := completed.ToCandle(pair, interval)
			if err := a.repo.Upsert(context.Background(), c); err != nil {
				log.Printf("[aggregator] flush %s %s: %v", pair, interval, err)
			}
			a.cache.Invalidate(context.Background(), pair, interval)
		}
		a.broadcaster.Broadcast("candle@"+interval+"@"+pair, snapshot)
	}
}
