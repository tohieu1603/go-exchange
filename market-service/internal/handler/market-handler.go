package handler

import (
	"encoding/json"
	"strconv"

	"github.com/cryptox/market-service/internal/service"
	"github.com/cryptox/shared/response"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type MarketHandler struct {
	feed       *service.PriceFeed
	aggregator *service.CandleAggregator
	rdb        *redis.Client
}

func NewMarketHandler(feed *service.PriceFeed, aggregator *service.CandleAggregator, rdb *redis.Client) *MarketHandler {
	return &MarketHandler{feed: feed, aggregator: aggregator, rdb: rdb}
}

// Tickers godoc
// @Summary      Get all market tickers
// @Tags         market
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Router       /market/tickers [get]
func (h *MarketHandler) Tickers(c *gin.Context) {
	tickers := h.feed.GetAllTickers()
	response.OK(c, tickers)
}

// Depth godoc
// @Summary      Get order book depth for a pair
// @Tags         market
// @Produce      json
// @Param        pair   path   string  true  "Trading pair e.g. BTC_USDT"
// @Param        limit  query  int     false "Depth levels (default 20, max 100)"
// @Success      200  {object}  map[string]interface{}
// @Router       /market/depth/{pair} [get]
func (h *MarketHandler) Depth(c *gin.Context) {
	pair := c.Param("pair")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit < 1 || limit > 100 {
		limit = 20
	}

	ctx := c.Request.Context()
	key := "depth:" + pair

	type depthLevel struct {
		Price  float64 `json:"price"`
		Amount float64 `json:"amount"`
	}
	type depthResponse struct {
		Bids []depthLevel `json:"bids"`
		Asks []depthLevel `json:"asks"`
	}

	// Try to read from Redis; trading-service publishes depth snapshots here
	raw, err := h.rdb.Get(ctx, key).Bytes()
	if err != nil {
		// depth not yet published by trading-service - return empty
		response.OK(c, depthResponse{Bids: []depthLevel{}, Asks: []depthLevel{}})
		return
	}

	var depth depthResponse
	if err := json.Unmarshal(raw, &depth); err != nil {
		response.OK(c, depthResponse{Bids: []depthLevel{}, Asks: []depthLevel{}})
		return
	}

	// Trim to requested limit
	if len(depth.Bids) > limit {
		depth.Bids = depth.Bids[:limit]
	}
	if len(depth.Asks) > limit {
		depth.Asks = depth.Asks[:limit]
	}

	response.OK(c, depth)
}

// Trades godoc
// @Summary      Get recent trades for a pair
// @Tags         market
// @Produce      json
// @Param        pair   path   string  true  "Trading pair e.g. BTC_USDT"
// @Param        limit  query  int     false "Max results (default 50, max 200)"
// @Success      200  {array}   map[string]interface{}
// @Router       /market/trades/{pair} [get]
func (h *MarketHandler) Trades(c *gin.Context) {
	pair := c.Param("pair")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit < 1 || limit > 200 {
		limit = 50
	}
	// In microservices, trades are owned by trading-service.
	// Read recent trades from Redis cache (populated by trading-service WS broadcasts)
	ctx := c.Request.Context()
	cached, err := h.rdb.LRange(ctx, "recent_trades:"+pair, 0, int64(limit-1)).Result()
	if err != nil || len(cached) == 0 {
		response.OK(c, []interface{}{})
		return
	}
	var trades []map[string]interface{}
	for _, raw := range cached {
		var t map[string]interface{}
		if json.Unmarshal([]byte(raw), &t) == nil {
			trades = append(trades, t)
		}
	}
	response.OK(c, trades)
}

// Candles godoc
// @Summary      Get OHLCV candles for a pair
// @Tags         market
// @Produce      json
// @Param        pair      path   string  true  "Trading pair e.g. BTC_USDT"
// @Param        interval  query  string  false "Candle interval: 1m 5m 15m 1h 4h 1d (default 1h)"
// @Param        limit     query  int     false "Max candles (default 500, max 1500)"
// @Success      200  {array}   map[string]interface{}
// @Router       /market/candles/{pair} [get]
func (h *MarketHandler) Candles(c *gin.Context) {
	if h.aggregator == nil {
		response.OK(c, []interface{}{})
		return
	}
	pair := c.Param("pair")
	interval := c.DefaultQuery("interval", "1h")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "500"))
	if limit < 1 || limit > 1500 {
		limit = 500
	}
	candles, err := h.aggregator.GetCandles(pair, interval, limit)
	if err != nil {
		response.OK(c, []interface{}{})
		return
	}
	response.OK(c, candles)
}

// ExchangeRate godoc
// @Summary      Get VND/USDT exchange rate
// @Tags         market
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Router       /market/rate [get]
func (h *MarketHandler) ExchangeRate(c *gin.Context) {
	rate := service.GetVNDRate()
	response.OK(c, gin.H{"vndPerUsdt": rate})
}
