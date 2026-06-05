package http

import (
	"fmt"
	"strconv"

	"github.com/cryptox/futures-service/internal/usecase"
	"github.com/cryptox/shared/eventbus"
	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/shared/response"
	"github.com/gin-gonic/gin"
)

// AdminHandler exposes admin-scoped position endpoints.
type AdminHandler struct {
	uc  *usecase.PositionService
	bus usecase.EventPublisher
}

func NewAdminHandler(uc *usecase.PositionService, bus usecase.EventPublisher) *AdminHandler {
	return &AdminHandler{uc: uc, bus: bus}
}

func (h *AdminHandler) publishAudit(c *gin.Context, subjectUserID uint, action, outcome, detail string) {
	if h.bus == nil {
		return
	}
	adminID := middleware.GetUserID(c)
	_ = h.bus.Publish(c.Request.Context(), eventbus.TopicAuditRequest, eventbus.AuditRequestEvent{
		UserID:  subjectUserID,
		Action:  action,
		Outcome: outcome,
		IP:      c.ClientIP(),
		Detail:  fmt.Sprintf("admin=%d %s", adminID, detail),
	})
}

// UserPositions GET /api/admin/users/:id/positions
func (h *AdminHandler) UserPositions(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid user id")
		return
	}
	positions, err := h.uc.GetPositions(c.Request.Context(), uint(id), c.Query("status"))
	if err != nil {
		response.Error(c, 500, "failed to load positions")
		return
	}
	response.OK(c, toPositionDTOs(positions))
}

// CloseUserPosition POST /api/admin/users/:id/positions/:positionId/close
func (h *AdminHandler) CloseUserPosition(c *gin.Context) {
	uid, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid user id")
		return
	}
	pid, err := strconv.ParseUint(c.Param("positionId"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid position id")
		return
	}
	pos, err := h.uc.ClosePosition(c.Request.Context(), uint(uid), uint(pid))
	if err != nil {
		h.publishAudit(c, uint(uid), "admin.position.close", "failure", fmt.Sprintf("positionId=%d err=%s", pid, err.Error()))
		fail(c, err)
		return
	}
	h.publishAudit(c, uint(uid), "admin.position.close", "success",
		fmt.Sprintf("positionId=%d pair=%s side=%s pnl=%.4f", pid, pos.Pair, pos.Side, pos.UnrealizedPnL))
	response.OK(c, toPositionDTO(*pos))
}
