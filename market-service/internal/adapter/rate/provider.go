// Package rate adapts a USD→VND feed onto usecase.RateProvider.
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
	defaultRate    = 25000
	refreshEvery   = 5 * time.Minute
	feedURL        = "https://open.er-api.com/v6/latest/USD"
	requestTimeout = 10 * time.Second
)

// Provider caches and refreshes the VND-per-USDT rate (single reused client,
// context-cancellable refresh loop).
type Provider struct {
	mu     sync.RWMutex
	rate   float64
	client *http.Client
}

func NewProvider() *Provider {
	return &Provider{rate: defaultRate, client: &http.Client{Timeout: requestTimeout}}
}

func (p *Provider) VNDRate() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.rate
}

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
		log.Printf("[market] exchange rate fetch error: %v", err)
		return
	}
	defer resp.Body.Close()
	var result struct {
		Rates map[string]float64 `json:"rates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return
	}
	if vnd, ok := result.Rates["VND"]; ok && vnd > 0 {
		p.mu.Lock()
		p.rate = vnd
		p.mu.Unlock()
		log.Printf("[market] exchange rate updated: 1 USDT = %.0f VND", vnd)
	}
}
