// Package rate adapts a live USD→VND feed onto the application.RateProvider port.
package rate

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

const (
	defaultRate    = 25500
	refreshEvery   = 5 * time.Minute
	feedURL        = "https://open.er-api.com/v6/latest/USD"
	requestTimeout = 10 * time.Second
)

// Provider caches the VND-per-USDT rate and refreshes it on a ticker. A single
// http.Client is reused (no per-call allocation) and the refresh goroutine stops
// on context cancellation.
type Provider struct {
	mu     sync.RWMutex
	rate   float64
	client *http.Client
}

func NewProvider() *Provider {
	return &Provider{rate: defaultRate, client: &http.Client{Timeout: requestTimeout}}
}

// VNDRate returns the most recently fetched rate (or the default before the
// first successful fetch).
func (p *Provider) VNDRate() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.rate
}

// Start fetches once synchronously then refreshes every refreshEvery until ctx
// is cancelled.
func (p *Provider) Start(ctx context.Context) {
	p.fetch(ctx)
	go func() {
		t := time.NewTicker(refreshEvery)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				p.fetch(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (p *Provider) fetch(ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return
	}
	resp, err := p.client.Do(req)
	if err != nil {
		log.Printf("[wallet] exchange rate fetch error: %v", err)
		return
	}
	defer resp.Body.Close()

	var result struct {
		Rates map[string]float64 `json:"rates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("[wallet] exchange rate decode error: %v", err)
		return
	}
	if vnd, ok := result.Rates["VND"]; ok && vnd > 0 {
		p.mu.Lock()
		p.rate = vnd
		p.mu.Unlock()
		log.Printf("[wallet] exchange rate updated: 1 USDT = %.0f VND", vnd)
	}
}
