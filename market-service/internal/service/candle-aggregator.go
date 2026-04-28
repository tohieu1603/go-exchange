package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/cryptox/market-service/internal/model"
	"github.com/cryptox/shared/ws"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Intervals supported by the aggregator
var Intervals = []string{"1m", "3m", "5m", "15m", "30m", "1h", "2h", "4h", "6h", "12h", "1D", "1W", "1M"}

type liveCandle struct {
	Open, High, Low, Close, Volume float64
	OpenTime                       time.Time
	mu                             sync.Mutex
}

// CandleAggregator consumes price ticks and builds OHLCV candles
type CandleAggregator struct {
	db      *gorm.DB
	hub     *ws.Hub
	rdb     *redis.Client
	candles sync.Map // key: "pair:interval" -> *liveCandle
}

func NewCandleAggregator(db *gorm.DB, hub *ws.Hub, rdb *redis.Client) *CandleAggregator {
	return &CandleAggregator{db: db, hub: hub, rdb: rdb}
}

// ProcessTick is called by PriceFeed on each price update
func (ca *CandleAggregator) ProcessTick(pair string, price, volume float64) {
	now := time.Now().UTC()
	for _, interval := range Intervals {
		boundary := truncateToInterval(now, interval)
		key := pair + ":" + interval

		val, _ := ca.candles.LoadOrStore(key, &liveCandle{
			Open: price, High: price, Low: price, Close: price,
			Volume: volume, OpenTime: boundary,
		})
		lc := val.(*liveCandle)

		lc.mu.Lock()
		if !lc.OpenTime.Equal(boundary) {
			completed := liveCandle{
				Open: lc.Open, High: lc.High, Low: lc.Low, Close: lc.Close,
				Volume: lc.Volume, OpenTime: lc.OpenTime,
			}
			lc.mu.Unlock()
			ca.flushCandle(pair, interval, &completed)

			lc.mu.Lock()
			lc.Open = price
			lc.High = price
			lc.Low = price
			lc.Close = price
			lc.Volume = volume
			lc.OpenTime = boundary
		} else {
			if price > lc.High {
				lc.High = price
			}
			if price < lc.Low {
				lc.Low = price
			}
			lc.Close = price
			lc.Volume += volume
		}
		snapshot := model.CandleWS{
			Time: lc.OpenTime.Unix(), Open: lc.Open, High: lc.High,
			Low: lc.Low, Close: lc.Close, Volume: lc.Volume,
		}
		lc.mu.Unlock()

		ca.hub.Broadcast("candle@"+interval+"@"+pair, snapshot)
	}
}

// flushCandle persists a completed candle to DB and invalidates Redis cache
func (ca *CandleAggregator) flushCandle(pair, interval string, lc *liveCandle) {
	candle := model.Candle{
		Pair:     pair,
		Interval: interval,
		OpenTime: lc.OpenTime,
		Open:     lc.Open,
		High:     lc.High,
		Low:      lc.Low,
		Close:    lc.Close,
		Volume:   lc.Volume,
	}
	if err := ca.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "pair"}, {Name: "interval"}, {Name: "open_time"}},
		DoUpdates: clause.AssignmentColumns([]string{"open", "high", "low", "close", "volume"}),
	}).Create(&candle).Error; err != nil {
		log.Printf("flushCandle error [%s %s]: %v", pair, interval, err)
	}

	ctx := context.Background()
	pattern := fmt.Sprintf("candles:%s:%s:*", pair, interval)
	keys, _ := ca.rdb.Keys(ctx, pattern).Result()
	if len(keys) > 0 {
		ca.rdb.Del(ctx, keys...)
	}
}

// GetCandles returns historical candles for REST API (Redis-cached)
func (ca *CandleAggregator) GetCandles(pair, interval string, limit int) ([]model.CandleWS, error) {
	ctx := context.Background()
	cacheKey := fmt.Sprintf("candles:%s:%s:%d", pair, interval, limit)

	if raw, err := ca.rdb.Get(ctx, cacheKey).Bytes(); err == nil {
		var result []model.CandleWS
		if json.Unmarshal(raw, &result) == nil {
			return result, nil
		}
	}

	var candles []model.Candle
	if err := ca.db.Where("pair = ? AND interval = ?", pair, interval).
		Order("open_time DESC").Limit(limit).Find(&candles).Error; err != nil {
		return nil, err
	}

	for i, j := 0, len(candles)-1; i < j; i, j = i+1, j-1 {
		candles[i], candles[j] = candles[j], candles[i]
	}

	result := make([]model.CandleWS, len(candles))
	for i, c := range candles {
		result[i] = c.ToWS()
	}

	if b, err := json.Marshal(result); err == nil {
		ca.rdb.Set(ctx, cacheKey, b, 30*time.Second)
	}
	return result, nil
}

func truncateToInterval(t time.Time, interval string) time.Time {
	switch interval {
	case "1m":
		return t.Truncate(time.Minute)
	case "3m":
		return t.Truncate(3 * time.Minute)
	case "5m":
		return t.Truncate(5 * time.Minute)
	case "15m":
		return t.Truncate(15 * time.Minute)
	case "30m":
		return t.Truncate(30 * time.Minute)
	case "1h":
		return t.Truncate(time.Hour)
	case "2h":
		return t.Truncate(2 * time.Hour)
	case "4h":
		return t.Truncate(4 * time.Hour)
	case "6h":
		return t.Truncate(6 * time.Hour)
	case "12h":
		return t.Truncate(12 * time.Hour)
	case "1D":
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	case "1W":
		weekday := int(t.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		day := t.Day() - (weekday - 1)
		return time.Date(t.Year(), t.Month(), day, 0, 0, 0, 0, time.UTC)
	case "1M":
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	default:
		return t.Truncate(time.Hour)
	}
}
