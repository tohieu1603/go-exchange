package handler

import (
	"strconv"

	"github.com/cryptox/futures-service/internal/service"
	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/shared/response"
	"github.com/gin-gonic/gin"
)

type FundingHandler struct {
	svc *service.FundingService
}

func NewFundingHandler(svc *service.FundingService) *FundingHandler {
	return &FundingHandler{svc: svc}
}

// Latest godoc
// @Summary      Latest funding rate for a pair
// @Tags         funding
// @Produce      json
// @Param        pair  path  string  true  "Perpetual pair e.g. BTC_USDT"
// @Success      200  {object}  map[string]interface{}
// @Failure      404  {object}  map[string]interface{}
// @Router       /futures/funding/{pair}/latest [get]
func (h *FundingHandler) Latest(c *gin.Context) {
	pair := c.Param("pair")
	rate, err := h.svc.LatestRate(pair)
	if err != nil {
		response.Error(c, 404, "no funding rate yet")
		return
	}
	response.OK(c, rate)
}

// History of funding rates for a pair (public).
func (h *FundingHandler) History(c *gin.Context) {
	pair := c.Param("pair")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "30"))
	rows, err := h.svc.RecentRates(pair, limit)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, rows)
}

// MyHistory — funding payments charged/credited to the current user.
func (h *FundingHandler) MyHistory(c *gin.Context) {
	userID := middleware.GetUserID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	rows, total, err := h.svc.UserHistory(userID, page, size)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Page(c, rows, total, page, size)
}
