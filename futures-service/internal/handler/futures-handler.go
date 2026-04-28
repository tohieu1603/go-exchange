package handler

import (
	"strconv"
	"strings"

	"github.com/cryptox/futures-service/internal/service"
	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/shared/response"
	"github.com/gin-gonic/gin"
)

type FuturesHandler struct {
	futures *service.FuturesService
}

func NewFuturesHandler(futures *service.FuturesService) *FuturesHandler {
	return &FuturesHandler{futures: futures}
}

// OpenPosition godoc
// @Summary      Open a futures position
// @Description  Side must be LONG or SHORT; leverage 1-125
// @Tags         futures
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        body  body  service.OpenPositionReq  true  "Position payload"
// @Success      201  {object}  map[string]interface{}
// @Failure      400  {object}  map[string]interface{}
// @Router       /futures/order [post]
func (h *FuturesHandler) OpenPosition(c *gin.Context) {
	var req service.OpenPositionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}

	if req.Side != "LONG" && req.Side != "SHORT" {
		response.Error(c, 400, "side must be LONG or SHORT")
		return
	}
	if req.Leverage < 1 || req.Leverage > 125 {
		response.Error(c, 400, "leverage must be between 1 and 125")
		return
	}

	userID := middleware.GetUserID(c)
	pos, err := h.futures.OpenPosition(userID, req)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.Created(c, pos)
}

// ClosePosition godoc
// @Summary      Close a futures position
// @Tags         futures
// @Produce      json
// @Security     CookieAuth
// @Param        id   path  int  true  "Position ID"
// @Success      200  {object}  map[string]interface{}
// @Failure      404  {object}  map[string]interface{}
// @Router       /futures/close/{id} [post]
func (h *FuturesHandler) ClosePosition(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid position id")
		return
	}

	userID := middleware.GetUserID(c)
	pos, err := h.futures.ClosePosition(userID, uint(id))
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "not found") || strings.Contains(msg, "already closed") {
			response.Error(c, 404, msg)
		} else {
			response.Error(c, 400, msg)
		}
		return
	}
	response.OK(c, pos)
}

// Positions godoc
// @Summary      List all positions (optionally filtered by status)
// @Tags         futures
// @Produce      json
// @Security     CookieAuth
// @Param        status  query  string  false  "Filter: OPEN CLOSED LIQUIDATED"
// @Success      200  {array}   map[string]interface{}
// @Router       /futures/positions [get]
func (h *FuturesHandler) Positions(c *gin.Context) {
	userID := middleware.GetUserID(c)
	status := c.Query("status")
	positions, err := h.futures.GetPositions(userID, status)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, positions)
}

// UpdateTPSL godoc
// @Summary      Update take-profit / stop-loss for a position
// @Tags         futures
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        id    path  int                                         true  "Position ID"
// @Param        body  body  object{takeProfit=number,stopLoss=number}   true  "TP/SL values"
// @Success      200  {object}  map[string]interface{}
// @Failure      400  {object}  map[string]interface{}
// @Router       /futures/positions/{id}/tpsl [put]
func (h *FuturesHandler) UpdateTPSL(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid position id")
		return
	}
	var req struct {
		TakeProfit *float64 `json:"takeProfit"`
		StopLoss   *float64 `json:"stopLoss"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}

	userID := middleware.GetUserID(c)
	pos, err := h.futures.UpdateTPSL(userID, uint(id), req.TakeProfit, req.StopLoss)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, pos)
}

// OpenPositions GET /api/futures/positions/open
func (h *FuturesHandler) OpenPositions(c *gin.Context) {
	userID := middleware.GetUserID(c)
	positions, err := h.futures.GetOpenPositions(userID)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, positions)
}
