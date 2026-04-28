package model

import "time"

type Candle struct {
	ID       uint      `gorm:"primaryKey" json:"-"`
	Pair     string    `gorm:"not null;index:idx_candle,unique" json:"pair"`
	Interval string    `gorm:"not null;index:idx_candle,unique" json:"interval"` // 1m, 5m, 15m, 1h, 4h, 1d, 1w
	OpenTime time.Time `gorm:"not null;index:idx_candle,unique" json:"openTime"`
	Open     float64   `gorm:"type:decimal(30,10)" json:"open"`
	High     float64   `gorm:"type:decimal(30,10)" json:"high"`
	Low      float64   `gorm:"type:decimal(30,10)" json:"low"`
	Close    float64   `gorm:"type:decimal(30,10)" json:"close"`
	Volume   float64   `gorm:"type:decimal(30,10)" json:"volume"`
}

// CandleWS is the WebSocket format for TradingView lightweight-charts.
type CandleWS struct {
	Time   int64   `json:"time"`   // Unix timestamp seconds
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

func (c *Candle) ToWS() CandleWS {
	return CandleWS{
		Time: c.OpenTime.Unix(), Open: c.Open, High: c.High,
		Low: c.Low, Close: c.Close, Volume: c.Volume,
	}
}
