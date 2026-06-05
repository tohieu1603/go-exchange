// Package http is the inbound gin adapter for the futures service. Transport
// only: decode → use case → map domain errors → present DTOs.
package http

import (
	"errors"
	"net/http"
	"time"

	"github.com/cryptox/futures-service/internal/domain"
	"github.com/cryptox/shared/response"
	"github.com/gin-gonic/gin"
)

type positionDTO struct {
	ID               uint       `json:"id"`
	UserID           uint       `json:"userId"`
	Pair             string     `json:"pair"`
	Side             string     `json:"side"`
	Leverage         int        `json:"leverage"`
	EntryPrice       float64    `json:"entryPrice"`
	MarkPrice        float64    `json:"markPrice"`
	Size             float64    `json:"size"`
	Margin           float64    `json:"margin"`
	UnrealizedPnL    float64    `json:"unrealizedPnl"`
	LiquidationPrice float64    `json:"liquidationPrice"`
	TakeProfit       float64    `json:"takeProfit"`
	StopLoss         float64    `json:"stopLoss"`
	Status           string     `json:"status"`
	CreatedAt        time.Time  `json:"createdAt"`
	ClosedAt         *time.Time `json:"closedAt,omitempty"`
}

func toPositionDTO(p domain.Position) positionDTO {
	return positionDTO{
		ID: p.ID, UserID: p.UserID, Pair: p.Pair, Side: p.Side, Leverage: p.Leverage,
		EntryPrice: p.EntryPrice, MarkPrice: p.MarkPrice, Size: p.Size, Margin: p.Margin,
		UnrealizedPnL: p.UnrealizedPnL, LiquidationPrice: p.LiquidationPrice,
		TakeProfit: p.TakeProfit, StopLoss: p.StopLoss, Status: p.Status,
		CreatedAt: p.CreatedAt, ClosedAt: p.ClosedAt,
	}
}

func toPositionDTOs(ps []domain.Position) []positionDTO {
	out := make([]positionDTO, len(ps))
	for i, p := range ps {
		out[i] = toPositionDTO(p)
	}
	return out
}

type fundingRateDTO struct {
	ID         uint      `json:"id"`
	Pair       string    `json:"pair"`
	Rate       float64   `json:"rate"`
	IndexPrice float64   `json:"indexPrice"`
	MarkPrice  float64   `json:"markPrice"`
	Interval   string    `json:"interval"`
	SettledAt  time.Time `json:"settledAt"`
}

func toFundingRateDTO(r domain.FundingRate) fundingRateDTO {
	return fundingRateDTO{ID: r.ID, Pair: r.Pair, Rate: r.Rate, IndexPrice: r.IndexPrice, MarkPrice: r.MarkPrice, Interval: r.Interval, SettledAt: r.SettledAt}
}

func toFundingRateDTOs(rs []domain.FundingRate) []fundingRateDTO {
	out := make([]fundingRateDTO, len(rs))
	for i, r := range rs {
		out[i] = toFundingRateDTO(r)
	}
	return out
}

type fundingPaymentDTO struct {
	ID            uint      `json:"id"`
	PositionID    uint      `json:"positionId"`
	UserID        uint      `json:"userId"`
	FundingRateID uint      `json:"fundingRateId"`
	Pair          string    `json:"pair"`
	Side          string    `json:"side"`
	Notional      float64   `json:"notional"`
	Rate          float64   `json:"rate"`
	Amount        float64   `json:"amount"`
	CreatedAt     time.Time `json:"createdAt"`
}

func toFundingPaymentDTOs(ps []domain.FundingPayment) []fundingPaymentDTO {
	out := make([]fundingPaymentDTO, len(ps))
	for i, p := range ps {
		out[i] = fundingPaymentDTO{
			ID: p.ID, PositionID: p.PositionID, UserID: p.UserID, FundingRateID: p.FundingRateID,
			Pair: p.Pair, Side: p.Side, Notional: p.Notional, Rate: p.Rate, Amount: p.Amount, CreatedAt: p.CreatedAt,
		}
	}
	return out
}

// fail maps a domain error to the right HTTP status. It routes through
// response.FailClassified so an unexpected (apperr.Internal / 5xx) error is
// reduced to a generic "internal error" instead of leaking its detail.
func fail(c *gin.Context, err error) {
	response.FailClassified(c, err, func(e error) int {
		if errors.Is(e, domain.ErrPositionNotFound) {
			return http.StatusNotFound
		}
		return http.StatusBadRequest
	})
}
