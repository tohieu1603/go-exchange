package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/cryptox/shared/types"
	"github.com/redis/go-redis/v9"
)

// IndexFeed maintains the canonical "index price" for each trading pair —
// the aggregate spot price from external sources, used by futures funding-rate
// calculations.
//
// Mark price (current order-book mid) is published by PriceFeed at "price:{pair}".
// Index price is published HERE at "index:{pair}".
//
// Source: CoinGecko aggregate (volume-weighted across multiple exchanges).
// Refreshed every 5 minutes — funding settlement runs every 8h, plenty of headroom.
type IndexFeed struct {
	rdb   *redis.Client
	coins []types.Coin
}

func NewIndexFeed(rdb *redis.Client) *IndexFeed {
	return &IndexFeed{rdb: rdb, coins: types.DefaultCoins}
}

// Start launches the periodic refresh goroutine. Returns immediately.
func (f *IndexFeed) Start(ctx context.Context) {
	go func() {
		// Initial fetch (warm cache before first funding settlement).
		f.refresh(ctx)

		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				f.refresh(ctx)
			}
		}
	}()
	log.Println("[index-feed] started (CoinGecko aggregate, 5m refresh)")
}

func (f *IndexFeed) refresh(ctx context.Context) {
	// Build comma-separated CoinGecko IDs. Skip coins without one.
	ids := make([]string, 0, len(f.coins))
	cgToPair := make(map[string]string, len(f.coins))
	for _, c := range f.coins {
		if c.CoinGeckoID == "" {
			continue
		}
		ids = append(ids, c.CoinGeckoID)
		cgToPair[c.CoinGeckoID] = c.Symbol + "_USDT"
	}
	if len(ids) == 0 {
		return
	}

	url := fmt.Sprintf(
		"https://api.coingecko.com/api/v3/simple/price?ids=%s&vs_currencies=usd",
		strings.Join(ids, ","),
	)
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		log.Printf("[index-feed] build request: %v", err)
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[index-feed] fetch: %v", err)
		return
	}
	defer resp.Body.Close()

	var data map[string]map[string]float64
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		log.Printf("[index-feed] decode: %v", err)
		return
	}

	count := 0
	for cgID, payload := range data {
		price := payload["usd"]
		if price <= 0 {
			continue
		}
		pair, ok := cgToPair[cgID]
		if !ok {
			continue
		}
		// TTL 9h: longer than the 8h funding interval so a transient CoinGecko
		// outage doesn't immediately strand the funding settler.
		f.rdb.Set(ctx, "index:"+pair, price, 9*time.Hour)
		count++
	}
	if count > 0 {
		log.Printf("[index-feed] refreshed %d index prices", count)
	}
}
