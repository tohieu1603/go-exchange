package handler

import (
	"strconv"

	"github.com/cryptox/futures-service/internal/service"
	"github.com/cryptox/shared/response"
	"github.com/gin-gonic/gin"
)

// AdminHandler — admin-scoped read endpoints exposing per-user position data.
// JWT auth + AdminOnly middleware are applied at the route registration level.
type AdminHandler struct {
	futures *service.FuturesService
}

func NewAdminHandler(futures *service.FuturesService) *AdminHandler {
	return &AdminHandler{futures: futures}
}

// UserPositions returns positions for an arbitrary user.
// Optional ?status=OPEN|CLOSED|LIQUIDATED filters; default is all.
// GET /api/admin/users/:id/positions
func (h *AdminHandler) UserPositions(c *gin.Context) {
	uid64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid user id")
		return
	}
	userID := uint(uid64)
	status := c.Query("status")

	positions, err := h.futures.GetPositions(userID, status)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, positions)
}
