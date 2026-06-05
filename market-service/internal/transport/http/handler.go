// Package http is the inbound gin adapter for market data (all public reads).
package http

import (
	"strconv"

	"github.com/cryptox/market-service/internal/usecase"
	"github.com/cryptox/shared/response"
	"github.com/gin-gonic/gin"
)

type tickerDTO struct {
	Pair      string  `json:"pair"`
	Symbol    string  `json:"symbol"`
	Name      string  `json:"name"`
	AssetType string  `json:"assetType"`
	Price     float64 `json:"price"`
	Change24h float64 `json:"change24h"`
	Volume24h float64 `json:"volume24h"`
}

// Handler serves tickers, depth, trades, candles and the exchange rate.
type Handler struct {
	candles *usecase.CandleQuery
	market  *usecase.MarketQuery
	rate    usecase.RateProvider
}

func NewHandler(candles *usecase.CandleQuery, market *usecase.MarketQuery, rate usecase.RateProvider) *Handler {
	return &Handler{candles: candles, market: market, rate: rate}
}

func clampLimit(c *gin.Context, def, max int) int {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", strconv.Itoa(def)))
	if limit < 1 || limit > max {
		return def
	}
	return limit
}

// Tickers godoc
// @Summary Get all market tickers
// @Tags market
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /market/tickers [get]
func (h *Handler) Tickers(c *gin.Context) {
	ts := h.market.AllTickers(c.Request.Context())
	out := make([]tickerDTO, len(ts))
	for i, t := range ts {
		out[i] = tickerDTO{Pair: t.Pair, Symbol: t.Symbol, Name: t.Name, AssetType: t.AssetType, Price: t.Price, Change24h: t.Change24h, Volume24h: t.Volume24h}
	}
	response.OK(c, out)
}

// Depth godoc
// @Summary Get order book depth
// @Tags market
// @Produce json
// @Param pair path string true "Pair e.g. BTC_USDT"
// @Success 200 {object} map[string]interface{}
// @Router /market/depth/{pair} [get]
func (h *Handler) Depth(c *gin.Context) {
	depth, err := h.market.Depth(c.Request.Context(), c.Param("pair"), clampLimit(c, 20, 100))
	if err != nil {
		response.Error(c, 500, "failed to load depth")
		return
	}
	response.OK(c, depth)
}

// Trades godoc
// @Summary Get recent trades
// @Tags market
// @Produce json
// @Param pair path string true "Pair e.g. BTC_USDT"
// @Success 200 {array} map[string]interface{}
// @Router /market/trades/{pair} [get]
func (h *Handler) Trades(c *gin.Context) {
	trades, err := h.market.RecentTrades(c.Request.Context(), c.Param("pair"), clampLimit(c, 50, 200))
	if err != nil {
		response.Error(c, 500, "failed to load trades")
		return
	}
	response.OK(c, trades)
}

// Candles godoc
// @Summary Get OHLCV candles
// @Tags market
// @Produce json
// @Param pair path string true "Pair e.g. BTC_USDT"
// @Param interval query string false "1m 5m 15m 1h 4h 1D (default 1h)"
// @Success 200 {array} map[string]interface{}
// @Router /market/candles/{pair} [get]
func (h *Handler) Candles(c *gin.Context) {
	candles, err := h.candles.GetCandles(c.Request.Context(), c.Param("pair"), c.DefaultQuery("interval", "1h"), clampLimit(c, 500, 1500))
	if err != nil {
		response.OK(c, []interface{}{})
		return
	}
	response.OK(c, candles)
}

// ExchangeRate godoc
// @Summary Get VND/USDT rate
// @Tags market
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /market/rate [get]
func (h *Handler) ExchangeRate(c *gin.Context) {
	response.OK(c, gin.H{"vndPerUsdt": h.rate.VNDRate()})
}
