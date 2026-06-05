// Package domain holds the market data domain: the Candle (OHLCV) aggregate and
// the live-candle aggregation rules. Pure Go — no gorm/redis/ws.
package domain

import "time"

// Intervals are the candle timeframes the aggregator maintains.
var Intervals = []string{"1m", "3m", "5m", "15m", "30m", "1h", "2h", "4h", "6h", "12h", "1D", "1W", "1M"}

// Candle is a completed OHLCV bar for a (pair, interval, openTime).
type Candle struct {
	Pair     string
	Interval string
	OpenTime time.Time
	Open     float64
	High     float64
	Low      float64
	Close    float64
	Volume   float64
}

// CandleWS is the lightweight-charts wire shape.
type CandleWS struct {
	Time   int64   `json:"time"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

func (c Candle) ToWS() CandleWS {
	return CandleWS{Time: c.OpenTime.Unix(), Open: c.Open, High: c.High, Low: c.Low, Close: c.Close, Volume: c.Volume}
}

// LiveCandle accumulates ticks into the currently-forming bar for one
// (pair, interval). The OHLCV folding rules live here so they are unit-testable
// without any infrastructure.
type LiveCandle struct {
	Open     float64
	High     float64
	Low      float64
	Close    float64
	Volume   float64
	OpenTime time.Time
}

// NewLiveCandle starts a bar from the first tick.
func NewLiveCandle(price, volume float64, openTime time.Time) *LiveCandle {
	return &LiveCandle{Open: price, High: price, Low: price, Close: price, Volume: volume, OpenTime: openTime}
}

// Apply folds a tick into the bar. If boundary differs from the bar's open time
// the bar has rolled: it returns a copy of the just-completed bar and resets the
// receiver to the new period. Otherwise it updates high/low/close/volume and
// returns (nil, false).
func (lc *LiveCandle) Apply(price, volume float64, boundary time.Time) (completed *LiveCandle, rolled bool) {
	if !lc.OpenTime.Equal(boundary) {
		done := *lc
		lc.Open, lc.High, lc.Low, lc.Close, lc.Volume, lc.OpenTime = price, price, price, price, volume, boundary
		return &done, true
	}
	if price > lc.High {
		lc.High = price
	}
	if price < lc.Low {
		lc.Low = price
	}
	lc.Close = price
	lc.Volume += volume
	return nil, false
}

// ToCandle materialises the bar as a persistable Candle.
func (lc *LiveCandle) ToCandle(pair, interval string) Candle {
	return Candle{Pair: pair, Interval: interval, OpenTime: lc.OpenTime, Open: lc.Open, High: lc.High, Low: lc.Low, Close: lc.Close, Volume: lc.Volume}
}

// Snapshot is the live bar in wire form for WebSocket streaming.
func (lc *LiveCandle) Snapshot() CandleWS {
	return CandleWS{Time: lc.OpenTime.Unix(), Open: lc.Open, High: lc.High, Low: lc.Low, Close: lc.Close, Volume: lc.Volume}
}

// TruncateToInterval rounds t down to the start of its bar for the interval.
func TruncateToInterval(t time.Time, interval string) time.Time {
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
