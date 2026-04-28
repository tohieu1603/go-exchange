package handler

import (
	"strconv"
	"strings"

	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/trading-service/internal/model"
	"github.com/cryptox/shared/redisutil"
	"github.com/cryptox/shared/response"
	grpcclient "github.com/cryptox/trading-service/internal/grpc"
	"github.com/cryptox/trading-service/internal/service"
	"github.com/gin-gonic/gin"
)

type TradingHandler struct {
	engine       *service.MatchingEngine
	balCache     *redisutil.BalanceCache
	locker       *service.BalanceLocker
	walletClient *grpcclient.WalletClient
	orders       *service.OrderService
}

func NewTradingHandler(engine *service.MatchingEngine, balCache *redisutil.BalanceCache, locker *service.BalanceLocker, walletClient *grpcclient.WalletClient, orders *service.OrderService) *TradingHandler {
	return &TradingHandler{engine: engine, balCache: balCache, locker: locker, walletClient: walletClient, orders: orders}
}

type PlaceOrderReq struct {
	Pair      string  `json:"pair" binding:"required"`
	Side      string  `json:"side" binding:"required,oneof=BUY SELL"`
	Type      string  `json:"type" binding:"required,oneof=MARKET LIMIT STOP_LIMIT"`
	Price     float64 `json:"price"`
	StopPrice float64 `json:"stopPrice"`
	Amount    float64 `json:"amount" binding:"required,gt=0"`
}

func (h *TradingHandler) PlaceOrder(c *gin.Context) {
	var req PlaceOrderReq
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

	if req.Type == "LIMIT" && req.Price <= 0 {
		response.Error(c, 400, "price required for LIMIT order")
		return
	}

	userID := middleware.GetUserID(c)
	ctx := c.Request.Context()

	if req.Type == "MARKET" {
		marketPrice := h.engine.GetCurrentPrice(req.Pair)
		if marketPrice <= 0 {
			response.Error(c, 400, "price unavailable for this pair")
			return
		}
		// Use gRPC CheckBalance → wallet-service ensures Redis cache is warm
		if req.Side == "BUY" {
			needed := marketPrice * req.Amount * 1.002
			if err := h.walletClient.CheckBalance(ctx, userID, quote, needed); err != nil {
				response.Error(c, 400, "insufficient "+quote+" balance")
				return
			}
		} else {
			if err := h.walletClient.CheckBalance(ctx, userID, base, req.Amount); err != nil {
				response.Error(c, 400, "insufficient "+base+" balance")
				return
			}
		}
	} else if req.Type == "LIMIT" {
		// Warm Redis cache via gRPC before locking
		if req.Side == "BUY" {
			lockAmt := req.Price * req.Amount * 1.001
			_ = h.walletClient.CheckBalance(ctx, userID, quote, 0) // warm cache
			if err := h.locker.Lock(ctx, userID, quote, lockAmt); err != nil {
				response.Error(c, 400, err.Error())
				return
			}
		} else {
			_ = h.walletClient.CheckBalance(ctx, userID, base, 0) // warm cache
			if err := h.locker.Lock(ctx, userID, base, req.Amount); err != nil {
				response.Error(c, 400, err.Error())
				return
			}
		}
	}

	order := model.Order{
		UserID: userID, Pair: req.Pair, Side: req.Side,
		Type: req.Type, Price: req.Price, StopPrice: req.StopPrice,
		Amount: req.Amount, Status: "OPEN",
	}

	if err := h.orders.CreateOrder(&order); err != nil {
		response.Error(c, 500, err.Error())
		return
	}

	if err := h.engine.ProcessOrder(&order); err != nil {
		order.Status = "CANCELLED"
		h.orders.UpdateOrderStatus(&order)
		response.Error(c, 400, err.Error())
		return
	}

	h.orders.SyncOrderStatus(&order)
	response.Created(c, order)
}

func (h *TradingHandler) CancelOrder(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid order id")
		return
	}

	userID := middleware.GetUserID(c)
	ctx := c.Request.Context()

	order, err := h.orders.CancelOrder(ctx, userID, uint(id))
	if err != nil {
		switch err.Error() {
		case "order not found":
			response.Error(c, 404, err.Error())
		case "forbidden":
			response.Error(c, 403, err.Error())
		default:
			response.Error(c, 400, err.Error())
		}
		return
	}

	h.engine.CancelOrder(order)
	response.OK(c, order)
}

func (h *TradingHandler) OrderHistory(c *gin.Context) {
	userID := middleware.GetUserID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	statusFilter := c.Query("status")
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}

	orders, total, err := h.orders.GetOrderHistory(userID, statusFilter, page, size)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Page(c, orders, total, page, size)
}

func (h *TradingHandler) OpenOrders(c *gin.Context) {
	userID := middleware.GetUserID(c)
	orders, err := h.orders.GetOpenOrders(userID)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, orders)
}
