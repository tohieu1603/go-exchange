package handler

import (
	"fmt"
	"strconv"

	"github.com/cryptox/futures-service/internal/service"
	"github.com/cryptox/shared/eventbus"
	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/shared/response"
	"github.com/gin-gonic/gin"
)

// AdminHandler — admin-scoped read endpoints exposing per-user position data.
// JWT auth + AdminOnly middleware are applied at the route registration level.
type AdminHandler struct {
	futures *service.FuturesService
	bus     eventbus.EventPublisher
}

func NewAdminHandler(futures *service.FuturesService, bus eventbus.EventPublisher) *AdminHandler {
	return &AdminHandler{futures: futures, bus: bus}
}

// publishAudit fire-and-forgets an admin-action audit row to auth-service
// via the audit.request topic. Subject = the affected end-user.
func (h *AdminHandler) publishAudit(c *gin.Context, subjectUserID uint, action, outcome, detail string) {
	if h.bus == nil {
		return
	}
	adminID := middleware.GetUserID(c)
	full := fmt.Sprintf("admin=%d %s", adminID, detail)
	_ = h.bus.Publish(c.Request.Context(), eventbus.TopicAuditRequest, eventbus.AuditRequestEvent{
		UserID:  subjectUserID,
		Action:  action,
		Outcome: outcome,
		IP:      c.ClientIP(),
		Detail:  full,
	})
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

// CloseUserPosition force-closes a position at current mark price on
// behalf of the owning user. Settlement (unlock margin + apply net PnL)
// runs through the same FuturesService.ClosePosition path used by the
// user, so wallet/notification side-effects stay consistent.
// POST /api/admin/users/:id/positions/:positionId/close
func (h *AdminHandler) CloseUserPosition(c *gin.Context) {
	uid64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid user id")
		return
	}
	pid64, err := strconv.ParseUint(c.Param("positionId"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid position id")
		return
	}
	pos, err := h.futures.ClosePosition(uint(uid64), uint(pid64))
	if err != nil {
		h.publishAudit(c, uint(uid64), "admin.position.close", "failure",
			fmt.Sprintf("positionId=%d err=%s", pid64, err.Error()))
		response.Error(c, 400, err.Error())
		return
	}
	h.publishAudit(c, uint(uid64), "admin.position.close", "success",
		fmt.Sprintf("positionId=%d pair=%s side=%s pnl=%.4f", pid64, pos.Pair, pos.Side, pos.UnrealizedPnL))
	response.OK(c, pos)
}
