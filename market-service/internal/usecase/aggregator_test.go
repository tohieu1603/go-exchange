package usecase

import (
	"context"
	"sync"
	"testing"

	"github.com/cryptox/market-service/internal/domain"
)

type fakeCandleRepo struct {
	mu       sync.Mutex
	upserted []domain.Candle
}

func (r *fakeCandleRepo) Upsert(_ context.Context, c domain.Candle) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.upserted = append(r.upserted, c)
	return nil
}
func (r *fakeCandleRepo) UpsertBatch(_ context.Context, cs []domain.Candle) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.upserted = append(r.upserted, cs...)
	return nil
}
func (r *fakeCandleRepo) Query(context.Context, string, string, int) ([]domain.Candle, error) {
	return nil, nil
}

type fakeCandleCache struct {
	mu          sync.Mutex
	invalidated int
}

func (c *fakeCandleCache) Get(context.Context, string, string, int) ([]domain.CandleWS, bool) {
	return nil, false
}
func (c *fakeCandleCache) Set(context.Context, string, string, int, []domain.CandleWS) {}
func (c *fakeCandleCache) Invalidate(context.Context, string, string) {
	c.mu.Lock()
	c.invalidated++
	c.mu.Unlock()
}

type countingBroadcaster struct {
	mu    sync.Mutex
	calls int
}

func (b *countingBroadcaster) Broadcast(string, interface{}) {
	b.mu.Lock()
	b.calls++
	b.mu.Unlock()
}

func TestAggregator_ProcessTickBroadcastsEveryInterval(t *testing.T) {
	repo := &fakeCandleRepo{}
	bc := &countingBroadcaster{}
	agg := NewCandleAggregator(repo, &fakeCandleCache{}, bc)

	agg.ProcessTick("BTC_USDT", 50000, 1)
	// One broadcast per supported interval; no flush yet (first tick).
	if bc.calls != len(domain.Intervals) {
		t.Fatalf("expected %d broadcasts, got %d", len(domain.Intervals), bc.calls)
	}
	if len(repo.upserted) != 0 {
		t.Fatalf("first tick must not flush any completed candle, got %d", len(repo.upserted))
	}
}

func TestAggregator_FoldsRepeatedTicks(t *testing.T) {
	repo := &fakeCandleRepo{}
	agg := NewCandleAggregator(repo, &fakeCandleCache{}, &countingBroadcaster{})
	// Multiple ticks within the same minute should not flush (no roll).
	for i := 0; i < 5; i++ {
		agg.ProcessTick("ETH_USDT", 3000+float64(i), 1)
	}
	if len(repo.upserted) != 0 {
		t.Fatalf("ticks within one bar must not flush, got %d", len(repo.upserted))
	}
}
