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

// OpenPosition POST /api/futures/order
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

// ClosePosition POST /api/futures/close/:id
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

// Positions GET /api/futures/positions
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

// UpdateTPSL PUT /api/futures/positions/:id/tpsl
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
