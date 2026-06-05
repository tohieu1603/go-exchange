package http

import (
	"github.com/cryptox/auth-service/internal/usecase"
	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/shared/response"
	"github.com/gin-gonic/gin"
)

type FeeTierHandler struct {
	svc *usecase.FeeTierService
}

func NewFeeTierHandler(svc *usecase.FeeTierService) *FeeTierHandler {
	return &FeeTierHandler{svc: svc}
}

// ListTiers — public endpoint, lists all VIP tiers for transparency.
func (h *FeeTierHandler) ListTiers(c *gin.Context) {
	tiers, err := h.svc.ListTiers()
	if err != nil {
		response.Internal(c, err)
		return
	}
	response.OK(c, tiers)
}

// MyTier — current user's tier + 30-day volume + progress to next.
func (h *FeeTierHandler) MyTier(c *gin.Context) {
	userID := middleware.GetUserID(c)
	view, err := h.svc.MyTier(userID)
	if err != nil {
		response.Internal(c, err)
		return
	}
	response.OK(c, view)
}
