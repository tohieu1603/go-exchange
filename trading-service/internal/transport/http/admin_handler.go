package http

import (
	"fmt"
	"strconv"

	"github.com/cryptox/shared/eventbus"
	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/shared/response"
	"github.com/cryptox/trading-service/internal/usecase"
	"github.com/gin-gonic/gin"
)

// AdminHandler exposes admin-scoped order endpoints.
type AdminHandler struct {
	orders *usecase.OrderService
	bus    eventbus.EventPublisher
}

func NewAdminHandler(orders *usecase.OrderService, bus eventbus.EventPublisher) *AdminHandler {
	return &AdminHandler{orders: orders, bus: bus}
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

// UserOrders GET /api/admin/users/:id/orders
func (h *AdminHandler) UserOrders(c *gin.Context) {
	uid, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid user id")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 10
	}
	orders, total, err := h.orders.GetOrderHistory(c.Request.Context(), uint(uid), c.Query("status"), page, size)
	if err != nil {
		response.Error(c, 500, "failed to load orders")
		return
	}
	response.Page(c, toOrderDTOs(orders), total, page, size)
}

// CancelUserOrder POST /api/admin/users/:id/orders/:orderId/cancel
func (h *AdminHandler) CancelUserOrder(c *gin.Context) {
	uid, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid user id")
		return
	}
	oid, err := strconv.ParseUint(c.Param("orderId"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid order id")
		return
	}
	order, err := h.orders.CancelOrder(c.Request.Context(), uint(uid), uint(oid))
	if err != nil {
		h.publishAudit(c, uint(uid), "admin.order.cancel", "failure", fmt.Sprintf("orderId=%d err=%s", oid, err.Error()))
		failCancel(c, err)
		return
	}
	h.publishAudit(c, uint(uid), "admin.order.cancel", "success", fmt.Sprintf("orderId=%d pair=%s side=%s", oid, order.Pair, order.Side))
	response.OK(c, toOrderDTO(*order))
}
