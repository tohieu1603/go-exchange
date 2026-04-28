package service

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/cryptox/market-service/internal/model"
	"github.com/cryptox/shared/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// bybitIntervals maps our interval strings to Bybit interval strings
var bybitIntervals = map[string]string{
	"1m":  "1",
	"3m":  "3",
	"5m":  "5",
	"15m": "15",
	"30m": "30",
	"1h":  "60",
	"2h":  "120",
	"4h":  "240",
	"6h":  "360",
	"12h": "720",
	"1D":  "D",
	"1W":  "W",
}

// CandleBackfill fetches historical OHLCV from Bybit on startup
type CandleBackfill struct {
	db *gorm.DB
}

func NewCandleBackfill(db *gorm.DB, _ string) *CandleBackfill {
	return &CandleBackfill{db: db}
}

// Run fetches historical candles for top coins across multiple timeframes
func (b *CandleBackfill) Run() {
	log.Println("Candle backfill started (Bybit multi-timeframe)")
	client := &http.Client{Timeout: 15 * time.Second}

	intervals := []struct {
		interval string
		limit    int
	}{
		{"1m", 200},
		{"5m", 200},
		{"15m", 200},
		{"1h", 200},
		{"4h", 200},
		{"1D", 200},
		{"1W", 100},
	}

	coins := types.DefaultCoins
	if len(coins) > 20 {
		coins = coins[:20]
	}

	for _, iv := range intervals {
		for _, coin := range coins {
			if err := b.backfillCoin(client, coin, iv.interval, iv.limit); err != nil {
				log.Printf("Backfill [%s %s]: %v", coin.Symbol, iv.interval, err)
			}
			time.Sleep(200 * time.Millisecond)
		}
		log.Printf("Backfill interval %s complete", iv.interval)
	}
	log.Println("Candle backfill complete")
}

func (b *CandleBackfill) backfillCoin(client *http.Client, coin types.Coin, interval string, limit int) error {
	bybitIv, ok := bybitIntervals[interval]
	if !ok {
		return fmt.Errorf("unsupported interval: %s", interval)
	}

	symbol := coin.Symbol + "USDT"
	url := fmt.Sprintf(
		"https://api.bybit.com/v5/market/kline?category=spot&symbol=%s&interval=%s&limit=%d",
		symbol, bybitIv, limit,
	)
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		RetCode int `json:"retCode"`
		Result  struct {
			List [][]string `json:"list"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	if result.RetCode != 0 {
		return fmt.Errorf("bybit code: %d", result.RetCode)
	}

	pair := coin.Symbol + "_USDT"
	candles := make([]model.Candle, 0, len(result.Result.List))
	for _, row := range result.Result.List {
		if len(row) < 6 {
			continue
		}
		ts, _ := strconv.ParseInt(row[0], 10, 64)
		open, _ := strconv.ParseFloat(row[1], 64)
		high, _ := strconv.ParseFloat(row[2], 64)
		low, _ := strconv.ParseFloat(row[3], 64)
		cl, _ := strconv.ParseFloat(row[4], 64)
		vol, _ := strconv.ParseFloat(row[5], 64)

		candles = append(candles, model.Candle{
			Pair:     pair,
			Interval: interval,
			OpenTime: time.UnixMilli(ts).UTC(),
			Open:     open,
			High:     high,
			Low:      low,
			Close:    cl,
			Volume:   vol,
		})
	}

	if len(candles) == 0 {
		return nil
	}

	if err := b.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "pair"}, {Name: "interval"}, {Name: "open_time"}},
		DoUpdates: clause.AssignmentColumns([]string{"open", "high", "low", "close", "volume"}),
	}).CreateInBatches(candles, 100).Error; err != nil {
		return fmt.Errorf("upsert: %w", err)
	}

	log.Printf("Backfilled %d %s candles for %s", len(candles), interval, pair)
	return nil
}
