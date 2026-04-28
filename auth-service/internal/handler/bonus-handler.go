package handler

import (
	"strconv"

	"github.com/cryptox/auth-service/internal/service"
	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/shared/response"
	"github.com/gin-gonic/gin"
)

type BonusHandler struct {
	bonusSvc *service.BonusService
}

func NewBonusHandler(bonusSvc *service.BonusService) *BonusHandler {
	return &BonusHandler{bonusSvc: bonusSvc}
}

// POST /api/admin/bonus/promotions
func (h *BonusHandler) CreatePromotion(c *gin.Context) {
	var req service.CreatePromotionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	promo, err := h.bonusSvc.CreatePromotion(req)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.Created(c, promo)
}

// GET /api/admin/bonus/promotions
func (h *BonusHandler) ListPromotions(c *gin.Context) {
	promos, err := h.bonusSvc.ListPromotions()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, promos)
}

// PUT /api/admin/bonus/promotions/:id/toggle
func (h *BonusHandler) TogglePromotion(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid promotion id")
		return
	}
	var body struct {
		Active bool `json:"active"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := h.bonusSvc.TogglePromotion(uint(id), body.Active); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "promotion updated"})
}

// GET /api/admin/bonus/users/:userId
func (h *BonusHandler) UserBonus(c *gin.Context) {
	uid, err := strconv.ParseUint(c.Param("userId"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid user id")
		return
	}
	total, bonuses, err := h.bonusSvc.GetUserBonus(uint(uid))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"totalActive": total, "bonuses": bonuses})
}

// GET /api/bonus/my
func (h *BonusHandler) MyBonus(c *gin.Context) {
	userID := middleware.GetUserID(c)
	total, bonuses, err := h.bonusSvc.GetUserBonus(userID)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"totalActive": total, "bonuses": bonuses})
}
