// Package feed holds the external market-data source adapters (Bybit WS,
// CoinGecko, forex). They write prices to Redis, broadcast tickers over the WS
// hub, and forward ticks to the candle aggregator (application layer).
package feed

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cryptox/market-service/internal/usecase"
	"github.com/cryptox/shared/types"
	"github.com/cryptox/shared/ws"
	gorillaws "github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

// PriceFeed streams live USD prices into Redis + the candle aggregator.
type PriceFeed struct {
	rdb         *redis.Client
	hub         *ws.Hub
	coins       []types.Coin
	aggregator  *usecase.CandleAggregator
	prevVol     sync.Map
	bybitToPair map[string]string
	bybitToSym  map[string]string
}

func NewPriceFeed(rdb *redis.Client, hub *ws.Hub, aggregator *usecase.CandleAggregator) *PriceFeed {
	b2p := make(map[string]string, len(types.DefaultCoins))
	b2s := make(map[string]string, len(types.DefaultCoins))
	for _, c := range types.DefaultCoins {
		bybit := c.GetBybitSymbol()
		b2p[bybit] = c.Symbol + "_USDT"
		b2s[bybit] = c.Symbol
	}
	return &PriceFeed{rdb: rdb, hub: hub, coins: types.DefaultCoins, aggregator: aggregator, bybitToPair: b2p, bybitToSym: b2s}
}

func (pf *PriceFeed) tick(pair string, price, volume float64) {
	if pf.aggregator != nil {
		pf.aggregator.ProcessTick(pair, price, volume)
	}
}

// Start launches the Bybit WS stream, REST warm-up and forex simulation.
func (pf *PriceFeed) Start() {
	log.Println("Price feed started (Bybit WebSocket + CoinGecko fallback)")
	go pf.fetchInitialPrices()
	go pf.streamBybitWS()
	go pf.pollForexRates()
}

func (pf *PriceFeed) pollForexRates() {
	time.Sleep(3 * time.Second)

	type fxPair struct {
		symbol    string
		apiKey    string
		pipSize   float64
		basePrice float64
		current   float64
		open24h   float64
	}
	pairs := []*fxPair{
		{symbol: "XAU", apiKey: "", pipSize: 1.20},
		{symbol: "PAXG", apiKey: "", pipSize: 1.20},
		{symbol: "EUR", apiKey: "eur", pipSize: 0.00015},
		{symbol: "GBP", apiKey: "gbp", pipSize: 0.00020},
		{symbol: "JPY", apiKey: "jpy", pipSize: 0.0000015},
	}

	client := &http.Client{Timeout: 10 * time.Second}
	if bybitResp, err := client.Get("https://api.bybit.com/v5/market/tickers?category=spot&symbol=XAUTUSDT"); err == nil {
		var bybitData struct {
			Result struct {
				List []struct {
					LastPrice string `json:"lastPrice"`
				} `json:"list"`
			} `json:"result"`
		}
		json.NewDecoder(bybitResp.Body).Decode(&bybitData)
		bybitResp.Body.Close()
		if len(bybitData.Result.List) > 0 {
			var p float64
			fmt.Sscanf(bybitData.Result.List[0].LastPrice, "%f", &p)
			pairs[0].basePrice, pairs[0].current, pairs[0].open24h = p, p, p
			pairs[1].basePrice, pairs[1].current, pairs[1].open24h = p, p, p
		}
	}

	resp, err := client.Get("https://cdn.jsdelivr.net/npm/@fawazahmed0/currency-api@latest/v1/currencies/usd.json")
	if err != nil {
		log.Printf("Forex API error: %v", err)
		return
	}
	var apiData struct {
		USD map[string]float64 `json:"usd"`
	}
	json.NewDecoder(resp.Body).Decode(&apiData)
	resp.Body.Close()

	for _, fx := range pairs {
		if fx.apiKey == "" {
			continue
		}
		rate := apiData.USD[fx.apiKey]
		if rate == 0 {
			continue
		}
		fx.basePrice, fx.current, fx.open24h = 1.0/rate, 1.0/rate, 1.0/rate
	}

	ctx := context.Background()
	for _, fx := range pairs {
		if fx.basePrice == 0 {
			continue
		}
		go func(fx *fxPair) {
			for {
				time.Sleep(time.Duration(300+rand.Intn(1700)) * time.Millisecond)
				move := (rand.Float64() - 0.5 + (rand.Float64()-0.5)*0.3) * 2.0 * fx.pipSize
				fx.current += move + (fx.basePrice-fx.current)*0.002
				maxDev := fx.basePrice * 0.01
				if fx.current > fx.basePrice+maxDev {
					fx.current = fx.basePrice + maxDev
				}
				if fx.current < fx.basePrice-maxDev {
					fx.current = fx.basePrice - maxDev
				}
				change24h := ((fx.current - fx.open24h) / fx.open24h) * 100
				pair := fx.symbol + "_USDT"
				pf.rdb.Set(ctx, "price:"+pair, fx.current, 0)
				pf.rdb.Set(ctx, "change24h:"+pair, change24h, 0)
				pf.hub.Broadcast("ticker@"+pair, map[string]interface{}{
					"pair": pair, "price": fx.current, "change24h": change24h, "volume24h": 0, "symbol": fx.symbol,
				})
				pf.tick(pair, fx.current, 0)
			}
		}(fx)
	}
	select {}
}

func (pf *PriceFeed) fetchInitialPrices() {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://api.bybit.com/v5/market/tickers?category=spot")
	if err != nil {
		log.Printf("Bybit REST fetch error: %v", err)
		return
	}
	defer resp.Body.Close()

	var result struct {
		Result struct {
			List []struct {
				Symbol       string `json:"symbol"`
				LastPrice    string `json:"lastPrice"`
				Price24hPcnt string `json:"price24hPcnt"`
				Turnover24h  string `json:"turnover24h"`
			} `json:"list"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("Bybit REST decode error: %v", err)
		return
	}

	bybitData := make(map[string]struct{ price, change, vol string })
	for _, t := range result.Result.List {
		bybitData[t.Symbol] = struct{ price, change, vol string }{t.LastPrice, t.Price24hPcnt, t.Turnover24h}
	}

	ctx := context.Background()
	for _, coin := range pf.coins {
		d, ok := bybitData[coin.GetBybitSymbol()]
		if !ok {
			continue
		}
		var priceUSD, changePct, volUSD float64
		fmt.Sscanf(d.price, "%f", &priceUSD)
		fmt.Sscanf(d.change, "%f", &changePct)
		fmt.Sscanf(d.vol, "%f", &volUSD)
		changePct *= 100
		pair := coin.Symbol + "_USDT"
		pf.rdb.Set(ctx, "price:"+pair, priceUSD, 5*time.Minute)
		pf.rdb.Set(ctx, "change24h:"+pair, changePct, 5*time.Minute)
		pf.rdb.Set(ctx, "vol24h:"+pair, volUSD, 5*time.Minute)
		pf.hub.Broadcast("ticker@"+pair, map[string]interface{}{
			"pair": pair, "price": priceUSD, "change24h": changePct, "volume24h": volUSD, "symbol": coin.Symbol,
		})
		pf.tick(pair, priceUSD, 0)
	}

	var missing []types.Coin
	for _, coin := range pf.coins {
		if _, ok := bybitData[coin.GetBybitSymbol()]; !ok && coin.CoinGeckoID != "" {
			missing = append(missing, coin)
		}
	}
	if len(missing) > 0 {
		go pf.fetchCoinGeckoFallback(missing)
	}
}

func (pf *PriceFeed) fetchCoinGeckoFallback(coins []types.Coin) {
	ids := make([]string, len(coins))
	for i, c := range coins {
		ids[i] = c.CoinGeckoID
	}
	url := fmt.Sprintf("https://api.coingecko.com/api/v3/simple/price?ids=%s&vs_currencies=usd&include_24hr_change=true&include_24hr_vol=true", strings.Join(ids, ","))
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		log.Printf("CoinGecko fallback error: %v", err)
		return
	}
	defer resp.Body.Close()

	var data map[string]map[string]float64
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return
	}
	ctx := context.Background()
	for _, coin := range coins {
		d, ok := data[coin.CoinGeckoID]
		if !ok || d["usd"] == 0 {
			continue
		}
		pair := coin.Symbol + "_USDT"
		pf.rdb.Set(ctx, "price:"+pair, d["usd"], 5*time.Minute)
		pf.rdb.Set(ctx, "change24h:"+pair, d["usd_24h_change"], 5*time.Minute)
		pf.rdb.Set(ctx, "vol24h:"+pair, d["usd_24h_vol"], 5*time.Minute)
		pf.hub.Broadcast("ticker@"+pair, map[string]interface{}{
			"pair": pair, "price": d["usd"], "change24h": d["usd_24h_change"], "volume24h": d["usd_24h_vol"], "symbol": coin.Symbol,
		})
		pf.tick(pair, d["usd"], 0)
	}
}

func (pf *PriceFeed) streamBybitWS() {
	for {
		pf.connectBybitWS()
		log.Println("Bybit WS disconnected, reconnecting in 3s...")
		time.Sleep(3 * time.Second)
	}
}

func (pf *PriceFeed) connectBybitWS() {
	dialer := gorillaws.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := dialer.Dial("wss://stream.bybit.com/v5/public/spot", nil)
	if err != nil {
		log.Printf("Bybit WS connect error: %v", err)
		return
	}
	defer conn.Close()
	log.Println("Bybit WebSocket connected - streaming real-time USD prices")

	symbols := make([]string, 0, len(pf.coins))
	for _, coin := range pf.coins {
		symbols = append(symbols, fmt.Sprintf("tickers.%s", coin.GetBybitSymbol()))
	}
	for i := 0; i < len(symbols); i += 10 {
		end := i + 10
		if end > len(symbols) {
			end = len(symbols)
		}
		if err := conn.WriteJSON(map[string]interface{}{"op": "subscribe", "args": symbols[i:end]}); err != nil {
			log.Printf("Bybit WS subscribe error: %v", err)
			return
		}
	}

	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if err := conn.WriteJSON(map[string]string{"op": "ping"}); err != nil {
				return
			}
		}
	}()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Bybit WS read error: %v", err)
			return
		}
		pf.handleBybitMessage(msg)
	}
}

type bybitTickerMsg struct {
	Topic string `json:"topic"`
	Data  struct {
		Symbol       string `json:"symbol"`
		LastPrice    string `json:"lastPrice"`
		Price24hPcnt string `json:"price24hPcnt"`
		Turnover24h  string `json:"turnover24h"`
	} `json:"data"`
}

func (pf *PriceFeed) handleBybitMessage(msg []byte) {
	var ticker bybitTickerMsg
	if err := json.Unmarshal(msg, &ticker); err != nil || !strings.HasPrefix(ticker.Topic, "tickers.") {
		return
	}
	bybitSym := strings.TrimPrefix(ticker.Topic, "tickers.")
	pair, ok := pf.bybitToPair[bybitSym]
	if !ok {
		return
	}
	symbol := pf.bybitToSym[bybitSym]

	var priceUSD, changePct, volUSD float64
	fmt.Sscanf(ticker.Data.LastPrice, "%f", &priceUSD)
	fmt.Sscanf(ticker.Data.Price24hPcnt, "%f", &changePct)
	fmt.Sscanf(ticker.Data.Turnover24h, "%f", &volUSD)
	if priceUSD == 0 {
		return
	}
	changePct *= 100

	ctx := context.Background()
	pf.rdb.Set(ctx, "price:"+pair, priceUSD, 5*time.Minute)
	pf.rdb.Set(ctx, "change24h:"+pair, changePct, 5*time.Minute)
	pf.rdb.Set(ctx, "vol24h:"+pair, volUSD, 5*time.Minute)
	pf.hub.Broadcast("ticker@"+pair, map[string]interface{}{
		"pair": pair, "price": priceUSD, "change24h": changePct, "volume24h": volUSD, "symbol": symbol,
	})

	prev, _ := pf.prevVol.LoadOrStore(pair, volUSD)
	volDelta := volUSD - prev.(float64)
	if volDelta < 0 {
		volDelta = 0
	}
	pf.prevVol.Store(pair, volUSD)
	pf.tick(pair, priceUSD, volDelta)
}
