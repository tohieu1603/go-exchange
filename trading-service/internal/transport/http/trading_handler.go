package http

import (
	"strconv"
	"strings"

	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/shared/response"
	"github.com/cryptox/trading-service/internal/usecase"
	"github.com/cryptox/trading-service/internal/domain"
	"github.com/gin-gonic/gin"
)

// TradingHandler exposes the user-facing spot order endpoints.
type TradingHandler struct {
	engine *usecase.MatchingEngine
	locker *usecase.BalanceLocker
	wallet usecase.WalletGateway
	orders *usecase.OrderService
}

func NewTradingHandler(engine *usecase.MatchingEngine, locker *usecase.BalanceLocker, wallet usecase.WalletGateway, orders *usecase.OrderService) *TradingHandler {
	return &TradingHandler{engine: engine, locker: locker, wallet: wallet, orders: orders}
}

type placeOrderReq struct {
	Pair      string  `json:"pair" binding:"required"`
	Side      string  `json:"side" binding:"required,oneof=BUY SELL"`
	Type      string  `json:"type" binding:"required,oneof=MARKET LIMIT STOP_LIMIT"`
	Price     float64 `json:"price"`
	StopPrice float64 `json:"stopPrice"`
	Amount    float64 `json:"amount" binding:"required,gt=0"`
}

// PlaceOrder godoc
// @Summary Place a spot order
// @Tags trading
// @Accept json
// @Produce json
// @Security CookieAuth
// @Success 201 {object} map[string]interface{}
// @Router /trading/orders [post]
func (h *TradingHandler) PlaceOrder(c *gin.Context) {
	var req placeOrderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	parts := strings.Split(req.Pair, "_")
	if len(parts) != 2 {
		response.Error(c, 400, "invalid pair format, expected BASE_QUOTE")
		return
	}
	base, quote := parts[0], parts[1]
	if req.Type == domain.TypeLimit && req.Price <= 0 {
		response.Error(c, 400, "price required for LIMIT order")
		return
	}

	userID := middleware.GetUserID(c)
	ctx := c.Request.Context()

	switch req.Type {
	case domain.TypeMarket:
		marketPrice := h.engine.GetCurrentPrice(req.Pair)
		if marketPrice <= 0 {
			response.Error(c, 400, "price unavailable for this pair")
			return
		}
		if req.Side == domain.SideBuy {
			if err := h.wallet.CheckBalance(ctx, userID, quote, marketPrice*req.Amount*1.002); err != nil {
				response.Error(c, 400, "insufficient "+quote+" balance")
				return
			}
		} else if err := h.wallet.CheckBalance(ctx, userID, base, req.Amount); err != nil {
			response.Error(c, 400, "insufficient "+base+" balance")
			return
		}
	case domain.TypeLimit:
		if req.Side == domain.SideBuy {
			_ = h.wallet.CheckBalance(ctx, userID, quote, 0) // warm cache
			if err := h.locker.Lock(ctx, userID, quote, req.Price*req.Amount*1.001); err != nil {
				response.Error(c, 400, err.Error())
				return
			}
		} else {
			_ = h.wallet.CheckBalance(ctx, userID, base, 0) // warm cache
			if err := h.locker.Lock(ctx, userID, base, req.Amount); err != nil {
				response.Error(c, 400, err.Error())
				return
			}
		}
	}

	order := &domain.Order{
		UserID: userID, Pair: req.Pair, Side: req.Side, Type: req.Type,
		Price: req.Price, StopPrice: req.StopPrice, Amount: req.Amount, Status: domain.StatusOpen,
	}
	if err := h.orders.CreateOrder(ctx, order); err != nil {
		response.Error(c, 500, "failed to create order")
		return
	}
	if err := h.engine.ProcessOrder(order); err != nil {
		order.Status = domain.StatusCancelled
		h.orders.UpdateOrderStatus(ctx, order)
		response.Error(c, 400, err.Error())
		return
	}
	h.orders.SyncOrderStatus(ctx, order)
	response.Created(c, toOrderDTO(*order))
}

// CancelOrder godoc
// @Summary Cancel an open order
// @Tags trading
// @Produce json
// @Security CookieAuth
// @Param id path int true "Order ID"
// @Success 200 {object} map[string]interface{}
// @Router /trading/orders/{id} [delete]
func (h *TradingHandler) CancelOrder(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid order id")
		return
	}
	order, err := h.orders.CancelOrder(c.Request.Context(), middleware.GetUserID(c), uint(id))
	if err != nil {
		failCancel(c, err)
		return
	}
	h.engine.CancelOrder(order)
	response.OK(c, toOrderDTO(*order))
}

// OrderHistory GET /api/trading/orders
func (h *TradingHandler) OrderHistory(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	orders, total, err := h.orders.GetOrderHistory(c.Request.Context(), middleware.GetUserID(c), c.Query("status"), page, size)
	if err != nil {
		response.Error(c, 500, "failed to load orders")
		return
	}
	response.Page(c, toOrderDTOs(orders), total, page, size)
}

// OpenOrders GET /api/trading/orders/open
func (h *TradingHandler) OpenOrders(c *gin.Context) {
	orders, err := h.orders.GetOpenOrders(c.Request.Context(), middleware.GetUserID(c))
	if err != nil {
		response.Error(c, 500, "failed to load orders")
		return
	}
	response.OK(c, toOrderDTOs(orders))
}
