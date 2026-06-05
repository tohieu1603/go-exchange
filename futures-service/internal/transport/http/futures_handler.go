package http

import (
	"strconv"

	"github.com/cryptox/futures-service/internal/usecase"
	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/shared/response"
	"github.com/gin-gonic/gin"
)

// FuturesHandler exposes the user-facing position endpoints.
type FuturesHandler struct {
	uc *usecase.PositionService
}

func NewFuturesHandler(uc *usecase.PositionService) *FuturesHandler { return &FuturesHandler{uc: uc} }

type openPositionReq struct {
	Pair       string  `json:"pair" binding:"required"`
	Side       string  `json:"side" binding:"required"`
	Leverage   int     `json:"leverage" binding:"required,min=1,max=125"`
	Size       float64 `json:"size" binding:"required,gt=0"`
	TakeProfit float64 `json:"takeProfit"`
	StopLoss   float64 `json:"stopLoss"`
}

// OpenPosition godoc
// @Summary Open a futures position
// @Tags futures
// @Accept json
// @Produce json
// @Security CookieAuth
// @Success 201 {object} map[string]interface{}
// @Router /futures/order [post]
func (h *FuturesHandler) OpenPosition(c *gin.Context) {
	var req openPositionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	pos, err := h.uc.OpenPosition(c.Request.Context(), middleware.GetUserID(c), usecase.OpenCommand{
		Pair: req.Pair, Side: req.Side, Leverage: req.Leverage, Size: req.Size,
		TakeProfit: req.TakeProfit, StopLoss: req.StopLoss,
	})
	if err != nil {
		fail(c, err)
		return
	}
	response.Created(c, toPositionDTO(*pos))
}

// ClosePosition godoc
// @Summary Close a futures position
// @Tags futures
// @Produce json
// @Security CookieAuth
// @Param id path int true "Position ID"
// @Success 200 {object} map[string]interface{}
// @Router /futures/close/{id} [post]
func (h *FuturesHandler) ClosePosition(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid position id")
		return
	}
	pos, err := h.uc.ClosePosition(c.Request.Context(), middleware.GetUserID(c), uint(id))
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, toPositionDTO(*pos))
}

// Positions godoc
// @Summary List positions (optional status filter)
// @Tags futures
// @Produce json
// @Security CookieAuth
// @Success 200 {array} map[string]interface{}
// @Router /futures/positions [get]
func (h *FuturesHandler) Positions(c *gin.Context) {
	positions, err := h.uc.GetPositions(c.Request.Context(), middleware.GetUserID(c), c.Query("status"))
	if err != nil {
		response.Error(c, 500, "failed to load positions")
		return
	}
	response.OK(c, toPositionDTOs(positions))
}

// OpenPositions GET /api/futures/positions/open
func (h *FuturesHandler) OpenPositions(c *gin.Context) {
	positions, err := h.uc.GetOpenPositions(c.Request.Context(), middleware.GetUserID(c))
	if err != nil {
		response.Error(c, 500, "failed to load positions")
		return
	}
	response.OK(c, toPositionDTOs(positions))
}

// UpdateTPSL godoc
// @Summary Update take-profit / stop-loss
// @Tags futures
// @Accept json
// @Produce json
// @Security CookieAuth
// @Param id path int true "Position ID"
// @Success 200 {object} map[string]interface{}
// @Router /futures/positions/{id}/tpsl [put]
func (h *FuturesHandler) UpdateTPSL(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
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
	pos, err := h.uc.UpdateTPSL(c.Request.Context(), middleware.GetUserID(c), uint(id), req.TakeProfit, req.StopLoss)
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, toPositionDTO(*pos))
}
