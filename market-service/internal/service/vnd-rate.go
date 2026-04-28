package service

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

var (
	cachedVNDRate float64 = 25000
	vndRateMu     sync.RWMutex
)

// GetVNDRate returns the cached VND/USDT exchange rate
func GetVNDRate() float64 {
	vndRateMu.RLock()
	defer vndRateMu.RUnlock()
	return cachedVNDRate
}

// StartVNDRateRefresh fetches and refreshes VND rate every 5 minutes
func StartVNDRateRefresh() {
	fetchVNDRate()
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			fetchVNDRate()
		}
	}()
}

func fetchVNDRate() {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://open.er-api.com/v6/latest/USD")
	if err != nil {
		log.Printf("Exchange rate fetch error: %v", err)
		return
	}
	defer resp.Body.Close()

	var result struct {
		Rates map[string]float64 `json:"rates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("Exchange rate decode error: %v", err)
		return
	}
	if vnd, ok := result.Rates["VND"]; ok && vnd > 0 {
		vndRateMu.Lock()
		cachedVNDRate = vnd
		vndRateMu.Unlock()
		log.Printf("Exchange rate updated: 1 USDT = %.0f VND", vnd)
	}
}
