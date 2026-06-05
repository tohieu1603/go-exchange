package http

import (
	"strconv"

	"github.com/cryptox/futures-service/internal/usecase"
	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/shared/response"
	"github.com/gin-gonic/gin"
)

// FundingHandler exposes funding-rate reads.
type FundingHandler struct {
	uc *usecase.FundingService
}

func NewFundingHandler(uc *usecase.FundingService) *FundingHandler { return &FundingHandler{uc: uc} }

// Latest godoc
// @Summary Latest funding rate for a pair
// @Tags funding
// @Produce json
// @Param pair path string true "Perpetual pair"
// @Success 200 {object} map[string]interface{}
// @Router /futures/funding/{pair}/latest [get]
func (h *FundingHandler) Latest(c *gin.Context) {
	rate, err := h.uc.LatestRate(c.Request.Context(), c.Param("pair"))
	if err != nil {
		response.Error(c, 404, "no funding rate yet")
		return
	}
	response.OK(c, toFundingRateDTO(*rate))
}

// History of funding rates for a pair (public).
func (h *FundingHandler) History(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "30"))
	rows, err := h.uc.RecentRates(c.Request.Context(), c.Param("pair"), limit)
	if err != nil {
		response.Error(c, 500, "failed to load funding history")
		return
	}
	response.OK(c, toFundingRateDTOs(rows))
}

// MyHistory — funding payments for the current user.
func (h *FundingHandler) MyHistory(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	rows, total, err := h.uc.UserHistory(c.Request.Context(), middleware.GetUserID(c), page, size)
	if err != nil {
		response.Error(c, 500, "failed to load funding payments")
		return
	}
	response.Page(c, toFundingPaymentDTOs(rows), total, page, size)
}
