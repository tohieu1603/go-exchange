// Package http is the inbound gin adapter for the trading service.
package http

import (
	"errors"
	"net/http"
	"time"

	"github.com/cryptox/shared/response"
	"github.com/cryptox/trading-service/internal/domain"
	"github.com/gin-gonic/gin"
)

type orderDTO struct {
	ID           uint      `json:"id"`
	UserID       uint      `json:"userId"`
	Pair         string    `json:"pair"`
	Side         string    `json:"side"`
	Type         string    `json:"type"`
	Price        float64   `json:"price"`
	StopPrice    float64   `json:"stopPrice"`
	Amount       float64   `json:"amount"`
	FilledAmount float64   `json:"filledAmount"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"createdAt"`
}

func toOrderDTO(o domain.Order) orderDTO {
	return orderDTO{
		ID: o.ID, UserID: o.UserID, Pair: o.Pair, Side: o.Side, Type: o.Type,
		Price: o.Price, StopPrice: o.StopPrice, Amount: o.Amount, FilledAmount: o.FilledAmount,
		Status: o.Status, CreatedAt: o.CreatedAt,
	}
}

func toOrderDTOs(os []domain.Order) []orderDTO {
	out := make([]orderDTO, len(os))
	for i, o := range os {
		out[i] = toOrderDTO(o)
	}
	return out
}

// failCancel maps the cancel use-case errors to HTTP statuses. It routes through
// response.FailClassified so an unexpected (apperr.Internal / 5xx) error is
// reduced to a generic "internal error" instead of leaking its detail.
func failCancel(c *gin.Context, err error) {
	response.FailClassified(c, err, func(e error) int {
		switch {
		case errors.Is(e, domain.ErrOrderNotFound):
			return http.StatusNotFound
		case errors.Is(e, domain.ErrForbidden):
			return http.StatusForbidden
		default:
			return http.StatusBadRequest
		}
	})
}
