package handler

import (
	"strconv"

	"github.com/cryptox/shared/response"
	"github.com/cryptox/trading-service/internal/service"
	"github.com/gin-gonic/gin"
)

// AdminHandler — admin-scoped read endpoints exposing per-user order history.
// JWT auth + admin-role middleware are applied at the route registration level
// (see cmd/main.go). The handler itself only validates the URL parameter.
type AdminHandler struct {
	orders *service.OrderService
}

func NewAdminHandler(orders *service.OrderService) *AdminHandler {
	return &AdminHandler{orders: orders}
}

// UserOrders returns paginated order history for an arbitrary user.
// GET /api/admin/users/:id/orders?page=1&size=20&status=FILLED
func (h *AdminHandler) UserOrders(c *gin.Context) {
	uid64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid user id")
		return
	}
	userID := uint(uid64)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	statusFilter := c.Query("status")
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 10
	}

	orders, total, err := h.orders.GetOrderHistory(userID, statusFilter, page, size)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Page(c, orders, total, page, size)
}

// CancelUserOrder force-cancels an order on behalf of a user. The order
// must belong to that user; the underlying service still scopes by userID
// so an admin can't cancel someone else's order via id-collision.
// POST /api/admin/users/:id/orders/:orderId/cancel
func (h *AdminHandler) CancelUserOrder(c *gin.Context) {
	uid64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid user id")
		return
	}
	oid64, err := strconv.ParseUint(c.Param("orderId"), 10, 64)
	if err != nil {
		response.Error(c, 400, "invalid order id")
		return
	}
	order, err := h.orders.CancelOrder(c.Request.Context(), uint(uid64), uint(oid64))
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, order)
}
